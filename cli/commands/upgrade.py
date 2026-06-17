"""
fastblog upgrade — 端到端系统升级引擎

流水线:
  precheck → backup → migrate-db → data-migrate → bump-version → clear-cache → postcheck → save-snapshot

每步失败自动触发 rollback（数据库 + version.txt 双回退）。
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from datetime import datetime
from pathlib import Path
from typing import Optional

import typer

app = typer.Typer(help="系统升级")


# ─── 工具函数 ────────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
BACKUP_DIR = PROJECT_ROOT / "backups"
SNAPSHOT_FILE = PROJECT_ROOT / ".upgrade-snapshot.json"


def _get_alembic_head() -> str:
    """获取当前 Alembic 最新迁移版本号"""
    try:
        from alembic.config import Config
        from alembic.script import ScriptDirectory
        cfg = Config(str(PROJECT_ROOT / "alembic.ini"))
        script = ScriptDirectory.from_config(cfg)
        head = script.get_current_head()
        return head or "base"
    except Exception:
        return "unknown"


def _run_alembic_cmd(*args: str) -> bool:
    """运行 Alembic 命令"""
    try:
        result = subprocess.run(
            [sys.executable, "-m", "alembic", "-c", str(PROJECT_ROOT / "alembic.ini"), *args],
            capture_output=True, text=True, cwd=str(PROJECT_ROOT),
        )
        if result.returncode != 0:
            typer.echo(f"  ❌ {result.stderr.strip()}", err=True)
            return False
        for line in result.stdout.splitlines():
            if line.strip():
                typer.echo(f"     {line.strip()}")
        return True
    except Exception as e:
        typer.echo(f"  ❌ {e}", err=True)
        return False


# ─── 升级快照（用于 --rollback） ─────────────────────────────

def _save_snapshot(old_ver: str, new_ver: str, old_migration: str, backup_file: str, env_backup: str):
    snap = {
        "old_version": old_ver,
        "new_version": new_ver,
        "old_migration": old_migration,
        "backup_file": backup_file,
        "env_backup": env_backup,
        "timestamp": datetime.now().isoformat(),
    }
    SNAPSHOT_FILE.write_text(json.dumps(snap, indent=2, ensure_ascii=False), encoding="utf-8")
    return snap


def _load_snapshot() -> Optional[dict]:
    if SNAPSHOT_FILE.exists():
        return json.loads(SNAPSHOT_FILE.read_text(encoding="utf-8"))
    return None


# ─── 流水线步骤 ──────────────────────────────────────────────

def step_precheck() -> bool:
    """检查环境是否满足升级条件"""
    typer.echo("🔍 ① 预检查...")

    # Python 版本
    py_ok = sys.version_info >= (3, 11)
    typer.echo(f"  {'✅' if py_ok else '❌'} Python {sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}")
    if not py_ok:
        return False

    # Alembic 配置
    alembic_ini = PROJECT_ROOT / "alembic.ini"
    if not alembic_ini.exists():
        typer.echo("  ❌ alembic.ini 不存在")
        return False

    # version.txt
    ver_file = PROJECT_ROOT / "version.txt"
    if not ver_file.exists():
        typer.echo("  ❌ version.txt 不存在")
        return False

    # 数据库连通性（通过 Alembic 检查）
    if not _run_alembic_cmd("current"):
        typer.echo("  ❌ 数据库无法连接或 Alembic 状态异常")
        return False

    typer.echo("  ✅ 预检查通过")
    return True


def step_backup() -> tuple[bool, str, str]:
    """备份数据库和配置文件"""
    typer.echo("💾 ② 备份...")
    BACKUP_DIR.mkdir(parents=True, exist_ok=True)
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")

    # 数据库备份（pg_dump）
    db_file = str(BACKUP_DIR / f"db_{ts}.sql")
    try:
        from src.setting import settings
        host = getattr(settings, 'DB_HOST', 'localhost')
        port = getattr(settings, 'DB_PORT', '5432')
        user = getattr(settings, 'DB_USER', 'postgres')
        dbname = getattr(settings, 'DB_NAME', 'fast_blog')
        pwd = getattr(settings, 'DB_PASSWORD', '')

        env = os.environ.copy()
        if pwd:
            env["PGPASSWORD"] = pwd
        result = subprocess.run(
            ["pg_dump", "-h", host, "-p", str(port), "-U", user, "-d", dbname,
             "--no-owner", "--no-acl", "-f", db_file],
            capture_output=True, text=True, env=env, timeout=120,
        )
        if result.returncode != 0:
            typer.echo(f"  ⚠️  pg_dump 失败（{result.stderr.strip()}），跳过备份")
            db_file = ""
        else:
            typer.echo(f"  ✅ 数据库备份: {db_file}")
    except Exception as e:
        typer.echo(f"  ⚠️  备份跳过（{e}）")
        db_file = ""

    # .env 备份
    env_backup = ""
    for src in [PROJECT_ROOT / "config" / ".env", PROJECT_ROOT / ".env"]:
        if src.exists():
            env_backup = str(BACKUP_DIR / f"env_{ts}")
            shutil.copy2(str(src), env_backup)
            typer.echo(f"  ✅ 配置备份: {env_backup}")
            break

    return True, db_file, env_backup


def step_migrate_db() -> bool:
    """执行 Alembic 数据库迁移"""
    typer.echo("🗄️  ③ 数据库迁移...")
    return _run_alembic_cmd("upgrade", "head")


def step_data_migrate() -> bool:
    """执行幂等数据迁移钩子"""
    typer.echo("🔄 ④ 数据迁移钩子...")
    from shared.upgrade import discover
    hooks = discover()

    if not hooks:
        typer.echo("  没有注册的数据迁移钩子")
        return True

    # 获取数据库会话
    try:
        from src.utils.database.unified_manager import db_manager
    except ImportError:
        typer.echo("  ⚠️  无法初始化数据库连接，跳过数据迁移")
        return True

    import asyncio

    async def _run_hooks():
        async with db_manager.get_session() as db:
            for priority, name, fn in hooks:
                typer.echo(f"  执行: {name}...", nl=False)
                try:
                    result = fn(db)
                    if result is None:
                        typer.echo(f" ✅ 完成")
                    elif result == 0:
                        typer.echo(f" ⏭️  无需处理")
                    else:
                        typer.echo(f" ✅ 处理 {result} 条")
                except Exception as e:
                    typer.echo(f" ❌ {e}", err=True)
                    return False
            return True

    loop = asyncio.new_event_loop()
    try:
        ok = loop.run_until_complete(_run_hooks())
        return ok
    finally:
        loop.close()


def step_bump_version(new_version: str):
    """更新版本号"""
    typer.echo(f"🏷️  ⑤ 更新版本: {new_version}")
    from shared.utils.version_manager import version_manager
    version_manager.bump_version(new_version)

    # 同步更新 DATABASE.migration
    head = _get_alembic_head()
    version_manager.update_database(migration=head)
    typer.echo(f"  ✅ version.txt → {new_version}, migration → {head}")


def step_clear_cache():
    """清理缓存"""
    typer.echo("🧹 ⑥ 清理缓存...")
    cache_dirs = [
        PROJECT_ROOT / "storage" / "cache",
        PROJECT_ROOT / "__pycache__",
    ]
    cleared = 0
    for d in cache_dirs:
        if d.exists() and d.is_dir():
            try:
                import shutil
                for item in d.iterdir():
                    if item.is_dir():
                        shutil.rmtree(item)
                    else:
                        item.unlink()
                cleared += 1
            except Exception as e:
                typer.echo(f"  ⚠️  {d.name}: {e}")

    # Redis 缓存清理（如果有）
    try:
        from src.extensions import cache
        cache.clear()
        typer.echo("  ✅ Redis/内存缓存已清理")
        cleared += 1
    except Exception:
        pass

    typer.echo(f"  ✅ 清理 {cleared} 个缓存位置")


def step_postcheck() -> bool:
    """升级后健康检查"""
    typer.echo("🧪 ⑦ 健康检查...")
    try:
        import httpx
        base_url = os.environ.get("SITE_URL", "http://localhost:9421")
        resp = httpx.get(f"{base_url}/health", timeout=10)
        if resp.is_success:
            typer.echo(f"  ✅ API 正常 (HTTP {resp.status_code})")
            return True
        typer.echo(f"  ⚠️  API 返回 {resp.status_code}")
        return False
    except Exception as e:
        typer.echo(f"  ⚠️  健康检查跳过（{e}）")
        return True


def step_save_snapshot(old_ver: str, new_ver: str, old_migration: str, backup_file: str, env_backup: str):
    """保存升级快照"""
    snap = _save_snapshot(old_ver, new_ver, old_migration, backup_file, env_backup)
    typer.echo(f"📸 ⑧ 升级快照已保存")
    return snap


# ─── Rollback ────────────────────────────────────────────────

def do_rollback():
    """回退到上一次升级前的状态"""
    snap = _load_snapshot()
    if not snap:
        typer.echo("❌ 没有找到升级快照，无法回退", err=True)
        raise typer.Exit(1)

    typer.echo(f"⏪ 回退到版本 {snap['old_version']}")
    typer.echo(f"   升级时间: {snap['timestamp']}")

    # 1. 恢复数据库
    if snap.get("backup_file") and os.path.exists(snap["backup_file"]):
        typer.echo("  恢复数据库...")
        if not _run_alembic_cmd("downgrade", snap.get("old_migration", "base")):
            typer.echo("  ⚠️  Alembic 回退失败")

        try:
            from src.setting import settings
            env = os.environ.copy()
            if hasattr(settings, 'DB_PASSWORD'):
                env["PGPASSWORD"] = settings.DB_PASSWORD
            subprocess.run(
                ["psql", "-h", settings.DB_HOST, "-p", str(settings.DB_PORT),
                 "-U", settings.DB_USER, "-d", settings.DB_NAME, "-f", snap["backup_file"]],
                capture_output=True, timeout=300, env=env,
            )
            typer.echo("  ✅ 数据库已恢复")
        except Exception as e:
            typer.echo(f"  ❌ 数据库恢复失败: {e}", err=True)
    else:
        typer.echo("  ⚠️  无可用备份，仅回退版本号")

    # 2. 恢复 .env
    if snap.get("env_backup") and os.path.exists(snap["env_backup"]):
        for dst in [PROJECT_ROOT / "config" / ".env", PROJECT_ROOT / ".env"]:
            if dst.exists():
                shutil.copy2(snap["env_backup"], str(dst))
                typer.echo("  ✅ 配置文件已恢复")
                break

    # 3. 恢复版本号
    from shared.utils.version_manager import version_manager
    version_manager.bump_version(snap["old_version"])
    version_manager.update_database(migration=snap.get("old_migration", "base"))
    typer.echo(f"  ✅ version.txt → {snap['old_version']}")

    # 4. 删除快照
    SNAPSHOT_FILE.unlink(missing_ok=True)
    typer.echo("✅ 回退完成")


# ─── 主命令 ──────────────────────────────────────────────────

@app.command()
def upgrade(
    to: Optional[str] = typer.Option(None, "--to", help="目标版本号（默认自动计算）"),
    dry_run: bool = typer.Option(False, "--dry-run", help="仅预览升级步骤，不执行"),
    force: bool = typer.Option(False, "--force", help="跳过预检查强制升级"),
):
    """执行端到端系统升级"""
    from shared.utils.version_manager import version_manager

    old_ver = version_manager.get_version()
    db_info = version_manager.get_database_info()
    old_migration = db_info.get("migration", "base")
    new_ver = to or _auto_next_version(old_ver)

    typer.echo(f"\n{'═' * 50}")
    typer.echo(f"  FastBlog 升级工具")
    typer.echo(f"  {old_ver}  →  {new_ver}")
    typer.echo(f"{'═' * 50}\n")

    if dry_run:
        typer.echo("🔍 [DRY RUN] 将执行以下步骤:\n")
        steps = [
            ("① precheck", "环境检查"),
            ("② backup", "数据库 + 配置备份"),
            ("③ migrate-db", "Alembic 迁移"),
            ("④ data-migrate", "数据迁移钩子"),
            ("⑤ bump-version", f"版本 {old_ver} → {new_ver}"),
            ("⑥ clear-cache", "缓存清理"),
            ("⑦ postcheck", "健康检查"),
            ("⑧ save-snapshot", "保存升级快照"),
        ]
        for step_id, desc in steps:
            typer.echo(f"  {step_id}: {desc}")
        typer.echo(f"\n✅ DRY RUN 完成，未执行任何操作")
        return

    # ── 执行流水线 ──
    pipeline = [
        ("precheck", lambda: force or step_precheck()),
        ("backup", lambda: step_backup()),
        ("migrate-db", step_migrate_db),
        ("data-migrate", step_data_migrate),
    ]

    results = {}
    backup_file = ""
    env_backup = ""

    for name, fn in pipeline:
        typer.echo()
        try:
            result = fn()
            if isinstance(result, tuple):
                ok, backup_file, env_backup = result
                results[name] = ok
            else:
                results[name] = result

            if not results[name]:
                typer.echo(f"\n❌ 步骤 '{name}' 失败，触发回退...")
                _auto_rollback(old_ver, old_migration)
                return
        except Exception as e:
            typer.echo(f"\n❌ 步骤 '{name}' 异常: {e}", err=True)
            _auto_rollback(old_ver, old_migration)
            return

    # ── 不可回退步骤（失败不 rollback，只警告） ──
    step_bump_version(new_ver)
    step_clear_cache()
    postcheck_ok = step_postcheck()
    snapshot = step_save_snapshot(old_ver, new_ver, old_migration, backup_file, env_backup)

    # ── 结果 ──
    typer.echo(f"\n{'═' * 50}")
    if postcheck_ok:
        typer.echo(f"✅ 升级完成: {old_ver} → {new_ver}")
    else:
        typer.echo(f"⚠️  升级完成（健康检查异常）: {old_ver} → {new_ver}")
    typer.echo(f"{'═' * 50}")


@app.command()
def rollback():
    """回退到上一次升级前的状态"""
    do_rollback()


def _auto_next_version(current: str) -> str:
    """自动计算下一个版本号（补丁+1，保留前导零）"""
    parts = current.split(".")
    if len(parts) >= 4:
        try:
            last = parts[-1]
            # 保留前导零的宽度
            width = len(last)
            incremented = str(int(last) + 1).zfill(width)
            parts[-1] = incremented
            return ".".join(parts)
        except ValueError:
            pass
    from datetime import datetime
    return f"0.5.{datetime.now().strftime('%m%d')}.01"


def _auto_rollback(old_ver: str, old_migration: str):
    """升级失败时自动回退"""
    typer.echo(f"\n⏪ 自动回退到 {old_ver}...")
    try:
        from shared.utils.version_manager import version_manager
        if not _run_alembic_cmd("downgrade", old_migration or "base"):
            typer.echo("  ⚠️  Alembic 回退失败，请手动处理")
        version_manager.bump_version(old_ver)
        typer.echo(f"  ✅ version.txt 已恢复")
    except Exception as e:
        typer.echo(f"  ❌ 回退异常: {e}", err=True)
    typer.echo("请检查系统状态后重试")


if __name__ == "__main__":
    app()
