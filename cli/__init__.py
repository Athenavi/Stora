"""
FastBlog CLI — 命令行管理工具

用法:
    fastblog --help
    fastblog create-user --help
    fastblog backup --help
    fastblog migrate --help
"""
import typer

app = typer.Typer(
    name="fastblog",
    help="FastBlog 命令行管理工具",
    add_completion=False,
    rich_markup_mode="rich",
)

# 注册子命令
from cli.commands import users, backup, cache, migrate, health, shell, upgrade

app.add_typer(users.app, name="user", help="用户管理")
app.add_typer(backup.app, name="backup", help="数据库备份与恢复")
app.add_typer(cache.app, name="cache", help="缓存管理")
app.add_typer(migrate.app, name="migrate", help="数据库迁移")
app.add_typer(health.app, name="health", help="健康检查")
app.add_typer(shell.app, name="shell", help="交互式 Shell")
app.add_typer(upgrade.app, name="upgrade", help="系统升级")


def main():
    app()


if __name__ == "__main__":
    main()
