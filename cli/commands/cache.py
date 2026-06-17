"""
缓存管理命令
"""
import typer

app = typer.Typer(help="缓存管理")


@app.command("clear")
def clear_cache(
    cache_type: str = typer.Option("all", "--type", "-t", help="缓存类型: all, redis, page, object"),
):
    """清理缓存"""
    typer.echo(f"清理缓存: {cache_type}")
    try:
        from src.extensions import redis_client
        if cache_type in ("all", "redis"):
            redis_client.flushdb()
            typer.echo("  ✅ Redis 缓存已清空")
        typer.echo("✅ 缓存清理完成")
    except Exception as e:
        typer.echo(f"❌ 清理失败: {e}", err=True)


@app.command("status")
def cache_status():
    """查看缓存状态"""
    typer.echo("缓存状态:")
    try:
        from src.extensions import redis_client
        info = redis_client.info()
        typer.echo(f"  Redis 版本: {info.get('redis_version', 'unknown')}")
        typer.echo(f"  已用内存: {info.get('used_memory_human', 'unknown')}")
        typer.echo(f"  连接数: {info.get('connected_clients', 'unknown')}")
        dbsize = redis_client.dbsize()
        typer.echo(f"  键数量: {dbsize}")
    except Exception as e:
        typer.echo(f"  ❌ 连接失败: {e}")
