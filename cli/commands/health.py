"""
健康检查命令
"""
import typer
import httpx

app = typer.Typer(help="健康检查")


@app.command("check")
def health_check(
    url: str = typer.Option("http://localhost:9421", "--url", "-u", help="服务地址"),
    verbose: bool = typer.Option(False, "--verbose", "-v", help="详细输出"),
):
    """检查服务运行状态"""
    typer.echo(f"健康检查: {url}")
    checks = {
        "API": f"{url}/health",
        "数据库": f"{url}/health/db",
        "Redis": f"{url}/health/redis",
    }
    all_ok = True
    for name, endpoint in checks.items():
        try:
            resp = httpx.get(endpoint, timeout=5)
            if resp.is_success:
                typer.echo(f"  ✅ {name}: OK")
                if verbose:
                    typer.echo(f"     {resp.json()}")
            else:
                typer.echo(f"  ❌ {name}: {resp.status_code}")
                all_ok = False
        except Exception as e:
            typer.echo(f"  ❌ {name}: {e}")
            all_ok = False

    if all_ok:
        typer.echo("✅ 所有服务正常运行")
    else:
        typer.echo("⚠️  部分服务异常", err=True)
        raise typer.Exit(code=1)
