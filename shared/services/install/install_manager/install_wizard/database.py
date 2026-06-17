"""
安装向导 — 数据库配置与迁移模块
"""
import os, sys, json, subprocess
from pathlib import Path
from typing import Dict, Any
from datetime import datetime

from src.unified_logger import default_logger as logger


def _update_env_file(env_path: Path, updates: Dict[str, str]):
    """更新 .env 文件中的键值对"""
    if env_path.exists():
        content = env_path.read_text(encoding="utf-8")
    else:
        content = ""
    lines = content.split("\n")
    existing_keys = set()
    for i, line in enumerate(lines):
        for k, v in updates.items():
            if line.startswith(f"{k}="):
                lines[i] = f"{k}={v}"
                existing_keys.add(k)
    for k, v in updates.items():
        if k not in existing_keys:
            lines.append(f"{k}={v}")
    env_path.write_text("\n".join(lines), encoding="utf-8")


def configure_database(project_root: Path, config: Dict[str, str]) -> Dict[str, Any]:
    """配置数据库连接并写入 .env"""
    from shared.services.install.install_manager.install_wizard.prerequisites import test_postgresql_connection
    host = config.get("DB_HOST", "localhost")
    port = config.get("DB_PORT", "5432")
    user = config.get("DB_USER", "postgres")
    password = config.get("DB_PASSWORD", "")
    db_name = config.get("DB_NAME", "fast_blog")

    conn_test = test_postgresql_connection(config)
    if not conn_test["success"]:
        return conn_test

    updates = {
        "DB_HOST": host,
        "DB_PORT": str(port),
        "DB_USER": user,
        "DB_PASSWORD": password,
        "DB_NAME": db_name,
        "DATABASE_URL": f"postgresql+psycopg2://{user}:{password}@{host}:{port}/{db_name}",
    }
    env_files = [project_root / ".env", project_root / "config" / ".env"]
    written = False
    for env_path in env_files:
        try:
            env_path.parent.mkdir(parents=True, exist_ok=True)
            _update_env_file(env_path, updates)
            written = True
        except Exception:
            continue
    if not written:
        return {"success": False, "message": "无法写入 .env 文件"}
    return {"success": True, "message": "数据库配置已保存"}


def run_migration(project_root: Path) -> Dict[str, Any]:
    """执行 Alembic 数据库迁移"""
    try:
        result = subprocess.run(
            [sys.executable, "-m", "alembic", "upgrade", "head"],
            capture_output=True, text=True, cwd=str(project_root), timeout=120,
        )
        if result.returncode != 0:
            return {"success": False, "message": f"迁移失败: {result.stderr[:500]}"}
        return {"success": True, "message": "数据库迁移完成"}
    except subprocess.TimeoutExpired:
        return {"success": False, "message": "迁移超时（120秒）"}
    except Exception as e:
        return {"success": False, "message": f"迁移错误: {str(e)}"}


def confirm_database_and_migrate(project_root: Path) -> Dict[str, Any]:
    """确认数据库配置并执行迁移"""
    from shared.services.install.install_manager.install_wizard.prerequisites import check_database_connection
    env_path = project_root / ".env"
    if not env_path.exists():
        return {"success": False, "message": "请先配置数据库"}
    config = {}
    for line in env_path.read_text(encoding="utf-8").split("\n"):
        if "=" in line:
            k, v = line.strip().split("=", 1)
            config[k] = v
    conn = check_database_connection(config)
    if not conn["success"]:
        return conn
    return run_migration(project_root)
