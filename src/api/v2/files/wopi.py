"""
Stora WOPI Protocol — Online document editing (Collabora Online / OnlyOffice)

WOPI endpoints called by the document server:
  GET  /api/v2/wopi/files/{file_id}           → CheckFileInfo (metadata)
  GET  /api/v2/wopi/files/{file_id}/contents   → GetFile (binary)
  POST /api/v2/wopi/files/{file_id}/contents   → PutFile (save)

Frontend access URL (to embed in iframe):
  /api/v2/wopi/access/{file_id}  → Redirect to Collabora/OnlyOffice with access token
"""
import os
from datetime import datetime, timedelta
from typing import Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query, Request, Response
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import FileItem, User
from src.auth import create_access_token, decode_token, jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/wopi", tags=["wopi"])

# WOPI access token (short-lived, for document server)
WOPI_TOKEN_EXPIRY = 3600


@router.get("/files/{file_id}")
async def wopi_check_file_info(
    file_id: int,
    access_token: str = Query(...),
    db: AsyncSession = Depends(get_async_db),
):
    """WOPI CheckFileInfo — return file metadata for the document server"""
    try:
        payload = decode_token(access_token)
    except Exception:
        raise HTTPException(status_code=401, detail="Invalid access token")

    user_id = int(payload.get("sub", 0))
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != user_id:
        raise HTTPException(status_code=404, detail="File not found")

    user = await db.get(User, user_id)

    return {
        "BaseFileName": file.filename or "untitled",
        "OwnerId": str(user_id),
        "Size": file.file_size or 0,
        "UserId": str(user_id),
        "UserFriendlyName": user.username if user else "User",
        "UserCanWrite": True,
        "Version": str(file.updated_at or file.created_at or ""),
        "LastModifiedTime": (file.updated_at or file.created_at or datetime.utcnow()).strftime(
            "%Y-%m-%dT%H:%M:%S.000Z"
        ),
        "SupportsLocks": False,
        "SupportsGetLock": False,
        "SupportsUpdate": True,
        "BreadcrumbDocName": file.filename or "",
        "DisablePrint": False,
        "DisableExport": False,
        "DisableCopy": False,
        "UserCanNotWriteRelative": False,
    }


@router.get("/files/{file_id}/contents")
async def wopi_get_file(
    file_id: int,
    access_token: str = Query(...),
    db: AsyncSession = Depends(get_async_db),
):
    """WOPI GetFile — return file binary content"""
    try:
        payload = decode_token(access_token)
    except Exception:
        raise HTTPException(status_code=401)

    user_id = int(payload.get("sub", 0))
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != user_id:
        raise HTTPException(status_code=404)

    storage_path = getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        raise HTTPException(status_code=404)

    from fastapi.responses import FileResponse
    return FileResponse(storage_path, media_type="application/octet-stream")


@router.post("/files/{file_id}/contents")
async def wopi_put_file(
    file_id: int,
    request: Request,
    access_token: str = Query(...),
    db: AsyncSession = Depends(get_async_db),
):
    """WOPI PutFile — save file content from document server"""
    try:
        payload = decode_token(access_token)
    except Exception:
        raise HTTPException(status_code=401)

    user_id = int(payload.get("sub", 0))
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != user_id:
        raise HTTPException(status_code=404)

    # Read content from request body
    content = await request.body()
    storage_path = getattr(file, "storage_path", None)

    if storage_path:
        with open(storage_path, "wb") as f:
            f.write(content)
        file.file_size = len(content)
        file.updated_at = datetime.utcnow()
        await db.commit()

    return Response(status_code=200)


@router.get("/access/{file_id}")
async def wopi_access_page(
    file_id: int,
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """生成 WOPI 访问 URL (嵌入 iframe)"""
    user_id = int(current_user.get("sub", 0))
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != user_id:
        raise HTTPException(status_code=404)

    # Create short-lived WOPI token
    wopi_token = create_access_token(
        {"sub": str(user_id), "file_id": file_id},
        expires_delta=WOPI_TOKEN_EXPIRY,
    )

    # WOPI source URL
    base = str(request.base_url).rstrip("/")
    wopi_src = f"{base}/api/v2/wopi/files/{file_id}?access_token={wopi_token}"
    collabora_url = os.environ.get("COLLABORA_URL", "https://collabora.example.com")
    editor_url = f"{collabora_url}/browser/{file_id}/ws?WOPISrc={wopi_src}"

    return {"editor_url": editor_url, "wopi_src": wopi_src}
