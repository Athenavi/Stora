"""
数据库备份与恢复命令（实现版）
"""
import typer
import os
import json
from pathlib import Path
from typing import Optional
from datetime import datetime

app = typer.Typer(help="数据库备份与恢复")


@app.command("create")
def create_backup(
    output: Optional[Path] = typer.Option(None, "--output", "-o", help="备份文件路径"),
    compress: bool = typer.Option(True, "--compress/--no-compress", help="是否压缩"),
):
    """创建数据库备份"""
    typer.echo("创建数据库备份...")
    try:
        from src.setting import settings
        db_url = getattr(settings, 'SQLALCHEMY_DATABASE_URI', None) or "postgresql://localhost/db"
        typer.echo(f"  数据库: {db_url}")

        output_path = output or f"backup_{datetime.now().strftime('%Y%m%d_%H%M%S')}.dump"
        if compress and not str(output_path).endswith('.gz'):
            output_path = str(output_path) + '.gz'

        # 解析数据库 URL
        import re
        match = re.match(r'postgresql(?:\+psycopg2)?://(?:([^:]+):([^@]+)@)?([^:/]+):?(\d+)?/(.+)', db_url)
        if not match:
            typer.echo("  ❌ 无法解析数据库 URL", err=True)
            raise typer.Exit(1)

        user, password, host, port, dbname = match.groups()
        port = port or '5432'

        env = os.environ.copy()
        if password:
            env['PGPASSWORD'] = password

        import subprocess
        cmd = ['pg_dump', '-h', host, '-p', port, '-U', user or 'postgres', '-F', 'c', '-f', str(output_path), dbname]
        result = subprocess.run(cmd, env=env, capture_output=True, text=True)
        if result.returncode != 0:
            typer.echo(f"  ❌ 备份失败: {result.stderr}", err=True)
            raise typer.Exit(1)

        typer.echo(f"  ✅ 备份成功: {output_path}")
    except Exception as e:
        typer.echo(f"  ❌ {e}", err=True)
        raise typer.Exit(1)


@app.command("restore")
def restore_backup(
    backup_file: Path = typer.Argument(..., help="备份文件路径", exists=True),
    force: bool = typer.Option(False, "--force", "-f", help="强制恢复（跳过确认）"),
):
    """从备份文件恢复数据库"""
    if not force:
        typer.confirm("⚠️  恢复将覆盖现有数据，确定继续?", abort=True)
    typer.echo(f"从 {backup_file} 恢复...")
    try:
        from src.setting import settings
        db_url = getattr(settings, 'SQLALCHEMY_DATABASE_URI', None) or "postgresql://localhost/db"

        import re
        match = re.match(r'postgresql(?:\+psycopg2)?://(?:([^:]+):([^@]+)@)?([^:/]+):?(\d+)?/(.+)', db_url)
        if not match:
            typer.echo("  ❌ 无法解析数据库 URL", err=True)
            raise typer.Exit(1)

        user, password, host, port, dbname = match.groups()
        port = port or '5432'

        env = os.environ.copy()
        if password:
            env['PGPASSWORD'] = password

        import subprocess

        # dropdb
        typer.echo("  - 删除旧数据库...")
        subprocess.run(['dropdb', '-h', host, '-p', port, '-U', user or 'postgres', '--if-exists', dbname],
                       env=env, capture_output=True)

        # createdb
        typer.echo("  - 创建新数据库...")
        subprocess.run(['createdb', '-h', host, '-p', port, '-U', user or 'postgres', dbname],
                       env=env, capture_output=True)

        # pg_restore
        typer.echo("  - 恢复数据...")
        result = subprocess.run(
            ['pg_restore', '-h', host, '-p', port, '-U', user or 'postgres', '-d', dbname,
             '--no-owner', '--no-privileges', str(backup_file)],
            env=env, capture_output=True, text=True
        )
        if result.returncode != 0:
            typer.echo(f"  ❌ 恢复失败: {result.stderr}", err=True)
            raise typer.Exit(1)

        typer.echo("  ✅ 恢复完成")
    except Exception as e:
        typer.echo(f"  ❌ {e}", err=True)
        raise typer.Exit(1)


@app.command("list")
def list_backups(
    backup_dir: str = typer.Option("./backups", "--dir", "-d", help="备份目录"),
):
    """列出可用备份"""
    typer.echo("可用备份:")
    backup_path = Path(backup_dir)
    if not backup_path.exists():
        typer.echo("  (备份目录不存在)")
        return

    found = False
    for f in sorted(backup_path.rglob("*.sql*"), key=lambda p: p.stat().st_mtime, reverse=True):
        size = f.stat().st_size
        size_str = f"{size / 1024 / 1024:.1f}MB" if size > 1024 * 1024 else f"{size / 1024:.1f}KB"
        mtime = datetime.fromtimestamp(f.stat().st_mtime).strftime('%Y-%m-%d %H:%M:%S')
        typer.echo(f"  {f.name} ({size_str}, {mtime})")
        found = True

    if not found:
        typer.echo("  (暂无备份)")
