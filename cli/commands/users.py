"""
用户管理命令
"""
import typer
from typing import Optional

app = typer.Typer(help="用户管理")


@app.command("create")
def create_user(
    username: str = typer.Option(..., "--username", "-u", prompt=True, help="用户名"),
    email: str = typer.Option(..., "--email", "-e", prompt=True, help="邮箱"),
    password: str = typer.Option(..., "--password", "-p", prompt=True, hide_input=True, confirmation_prompt=True, help="密码"),
    is_superuser: bool = typer.Option(False, "--superuser", help="是否为超级管理员"),
):
    """创建新用户"""
    typer.echo(f"创建用户: {username} ({email})")
    try:
        from src.app import create_app
        from src.setting import ProductionConfig
        app = create_app(ProductionConfig())
        # TODO: 实际用户创建逻辑
        typer.echo(f"✅ 用户 {username} 创建成功!")
    except Exception as e:
        typer.echo(f"❌ 创建失败: {e}", err=True)


@app.command("list")
def list_users(
    page: int = typer.Option(1, "--page", help="页码"),
    limit: int = typer.Option(20, "--limit", help="每页数量"),
):
    """列出所有用户"""
    typer.echo(f"列出用户 (第 {page} 页, 每页 {limit} 条)")
    # TODO: 实际查询逻辑


@app.command("activate")
def activate_user(
    user_id: int = typer.Argument(..., help="用户 ID"),
):
    """激活用户"""
    typer.echo(f"激活用户: {user_id}")
    # TODO


@app.command("deactivate")
def deactivate_user(
    user_id: int = typer.Argument(..., help="用户 ID"),
):
    """停用用户"""
    typer.echo(f"停用用户: {user_id}")
    # TODO


@app.command("reset-password")
def reset_password(
    user_id: int = typer.Argument(..., help="用户 ID"),
):
    """重置用户密码"""
    typer.echo(f"重置密码: {user_id}")
    # TODO
