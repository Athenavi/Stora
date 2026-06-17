"""
安装向导 — 前置检查模块
"""
import os, sys
from pathlib import Path
from typing import Dict, Any, List, Optional

from src.unified_logger import default_logger as logger


def check_python_version() -> Dict[str, Any]:
    version = sys.version_info
    return {
        "success": True,
        "python_version": f"{version.major}.{version.minor}.{version.micro}",
        "min_version": "3.11",
        "meets_requirement": version.major >= 3 and version.minor >= 11,
    }


def test_postgresql_connection(config: Dict[str, str]) -> Dict[str, Any]:
    import psycopg2
    try:
        conn = psycopg2.connect(
            host=config.get("DB_HOST", "localhost"),
            port=int(config.get("DB_PORT", 5432)),
            user=config.get("DB_USER", "postgres"),
            password=config.get("DB_PASSWORD", ""),
            dbname=config.get("DB_NAME", "fast_blog"),
            connect_timeout=5,
        )
        conn.close()
        return {"success": True, "message": "数据库连接成功"}
    except Exception as e:
        logger.warning(f"数据库连接测试失败: {e}")
        return {"success": False, "message": f"连接失败: {str(e)}"}


def check_database_connection(config: Dict[str, str]) -> Dict[str, Any]:
    return test_postgresql_connection(config)


def check_writable_directories(project_root: Path) -> List[Dict[str, Any]]:
    dirs = ["storage", "storage/thumbnails", "config", "static"]
    results = []
    for d in dirs:
        p = project_root / d
        p.mkdir(parents=True, exist_ok=True)
        writable = os.access(str(p), os.W_OK)
        results.append({"path": d, "exists": p.exists(), "writable": writable})
    return results


def check_required_packages() -> List[Dict[str, Any]]:
    required = ["fastapi", "sqlalchemy", "alembic", "psycopg2-binary"]
    results = []
    for pkg in required:
        try:
            __import__(pkg.replace("-", "_"))
            results.append({"name": pkg, "installed": True})
        except ImportError:
            results.append({"name": pkg, "installed": False})
    return results


def check_prerequisites(project_root: Path) -> Dict[str, Any]:
    return {
        "python": check_python_version(),
        "directories": check_writable_directories(project_root),
        "packages": check_required_packages(),
    }
