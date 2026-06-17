"""
数据库迁移命令
"""
import typer
from typing import Optional

app = typer.Typer(help="数据库迁移")


@app.command("run")
def run_migrations(
    message: Optional[str] = typer.Option(None, "--message", "-m", help="迁移说明"),
    autogenerate: bool = typer.Option(True, "--autogenerate/--no-autogenerate", help="自动生成迁移脚本"),
):
    """执行数据库迁移"""
    typer.echo("执行数据库迁移...")
    try:
        from alembic.config import Config
        from alembic import command
        alembic_cfg = Config("alembic.ini")
        if autogenerate:
            command.revision(alembic_cfg, autogenerate=True, message=message or "auto migration")
        command.upgrade(alembic_cfg, "head")
        typer.echo("✅ 迁移完成")
    except Exception as e:
        typer.echo(f"❌ 迁移失败: {e}", err=True)


@app.command("history")
def migration_history():
    """查看迁移历史"""
    typer.echo("迁移历史:")
    try:
        from alembic.config import Config
        from alembic import command
        alembic_cfg = Config("alembic.ini")
        command.history(alembic_cfg)
    except Exception as e:
        typer.echo(f"❌ 获取失败: {e}", err=True)


@app.command("downgrade")
def downgrade(
    revision: str = typer.Argument("base", help="目标版本"),
):
    """回滚迁移"""
    typer.echo(f"回滚到: {revision}")
    try:
        from alembic.config import Config
        from alembic import command
        alembic_cfg = Config("alembic.ini")
        command.downgrade(alembic_cfg, revision)
        typer.echo("✅ 回滚完成")
    except Exception as e:
        typer.echo(f"❌ 回滚失败: {e}", err=True)
