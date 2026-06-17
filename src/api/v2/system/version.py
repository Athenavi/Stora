"""
版本信息 API
提供版本查询、构建信息、更新检查等功能
"""
from functools import wraps
from pathlib import Path

from fastapi import APIRouter, Depends, HTTPException

from src.api.v2._helpers import ok
from src.auth.auth_deps import admin_required as admin_required_api

router = APIRouter()


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            from src.api.v2._helpers import fail
            return fail(str(e))
    return wrapper


def _read_version_txt() -> dict:
    """读取项目根目录的 version.txt"""
    version_file = Path(__file__).resolve().parent.parent.parent.parent.parent / "version.txt"
    info = {
        "version": "",
        "build_time": "",
        "migration": "",
        "migration_status": "",
        "maintainer": "",
        "repository": "",
    }
    if not version_file.exists():
        return info
    try:
        text = version_file.read_text(encoding="utf-8")
        section = None
        for line in text.splitlines():
            line = line.strip()
            if line == "[RELEASE]":
                section = "release"
            elif line == "[DATABASE]":
                section = "database"
            elif line == "[AUTHOR]":
                section = "author"
            elif "=" in line and section:
                k, v = line.split("=", 1)
                k, v = k.strip(), v.strip()
                if section == "release" and k == "version":
                    info["version"] = v
                elif section == "release" and k == "build_time":
                    info["build_time"] = v
                elif section == "database" and k == "migration":
                    info["migration"] = v
                elif section == "database" and k == "status":
                    info["migration_status"] = v
                elif section == "author" and k == "maintainer":
                    info["maintainer"] = v
                elif section == "author" and k == "repository":
                    info["repository"] = v
    except Exception:
        pass
    return info


@router.get("/version/full", summary="完整版本信息")
@_catch
async def get_version_full(
    current_user=Depends(admin_required_api),
):
    """获取完整的版本和构建信息"""
    return ok(data=_read_version_txt())


@router.get("/version/frontend", summary="前端版本")
@_catch
async def get_version_frontend(
    current_user=Depends(admin_required_api),
):
    """获取前端版本号"""
    info = _read_version_txt()
    return ok(data={"version": info["version"], "build_time": info["build_time"]})


@router.get("/version/backend", summary="后端版本")
@_catch
async def get_version_backend(
    current_user=Depends(admin_required_api),
):
    """获取后端版本号"""
    info = _read_version_txt()
    return ok(data={"version": info["version"], "build_time": info["build_time"]})
