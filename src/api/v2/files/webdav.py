"""
Stora WebDAV Server — mount as a network drive

Maps Stora file system to WebDAV protocol:
  PROPFIND  → list files/folders
  GET       → download file
  PUT       → upload file
  DELETE    → move to trash
  MKCOL     → create folder
  MOVE      → rename/move

Mount URL: http://localhost:9421/api/v2/webdav/
Auth: HTTP Basic Auth (username=stora_username, password=stora_password)
"""
import os
import xml.etree.ElementTree as ET
from datetime import datetime
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Header, Request, Response
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, and_

from shared.models import FileItem, Folder, User
from src.auth import verify_password, create_access_token
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="", tags=["webdav"])


async def webdav_auth(authorization: Optional[str] = Header(None), db: AsyncSession = Depends(get_async_db)):
    """WebDAV HTTP Basic Auth"""
    if not authorization or not authorization.startswith("Basic "):
        raise HTTPException(status_code=401, detail="Unauthorized",
                            headers={"WWW-Authenticate": 'Basic realm="Stora WebDAV"'})

    import base64
    decoded = base64.b64decode(authorization[6:]).decode("utf-8")
    username, password = decoded.split(":", 1)
    user = (await db.execute(select(User).where(User.username == username))).scalar_one_or_none()
    if not user or not verify_password(password, user.password):
        raise HTTPException(status_code=401, detail="Unauthorized",
                            headers={"WWW-Authenticate": 'Basic realm="Stora WebDAV"'})
    return user


def _parse_path(path: str) -> tuple:
    """Parse WebDAV path into user_id and folder_id"""
    parts = path.strip("/").split("/")
    if not parts or parts[0] == "":
        return None, None
    return parts[0], "/".join(parts[1:]) if len(parts) > 1 else ""


def _to_xml_response(href: str, is_folder: bool, name: str, size: int = 0, modified: str = ""):
    """Generate WebDAV XML response"""
    ET.register_namespace("D", "DAV:")
    ET.register_namespace("", "DAV:")

    response = ET.Element("{DAV:}response")
    href_elem = ET.SubElement(response, "{DAV:}href")
    href_elem.text = href

    propstat = ET.SubElement(response, "{DAV:}propstat")
    prop = ET.SubElement(propstat, "{DAV:}prop")

    # Resource type
    res_type = ET.SubElement(prop, "{DAV:}resourcetype")
    if is_folder:
        ET.SubElement(res_type, "{DAV:}collection")

    # Name
    name_elem = ET.SubElement(prop, "{DAV:}displayname")
    name_elem.text = name

    # Size
    size_elem = ET.SubElement(prop, "{DAV:}getcontentlength")
    size_elem.text = str(size)

    # Modified date
    modified_elem = ET.SubElement(prop, "{DAV:}getlastmodified")
    modified_elem.text = modified or datetime.utcnow().strftime("%a, %d %b %Y %H:%M:%S GMT")

    ET.SubElement(propstat, "{DAV:}status").text = "HTTP/1.1 200 OK"
    return response


def _xml_to_bytes(root: ET.Element) -> bytes:
    return ET.tostring(root, encoding="utf-8", xml_declaration=True)


def _fmt_date(dt) -> str:
    if not dt:
        return datetime.utcnow().strftime("%a, %d %b %Y %H:%M:%S GMT")
    if isinstance(dt, str):
        return dt
    return dt.strftime("%a, %d %b %Y %H:%M:%S GMT")


# ─── WebDAV Endpoints ───

@router.request("PROPFIND", "/webdav/{path:path}", include_in_schema=False)
@router.request("PROPFIND", "/webdav", include_in_schema=False)
async def webdav_propfind(
    request: Request,
    path: str = "",
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """List files (WebDAV PROPFIND)"""
    depth = request.headers.get("Depth", "1")

    multistatus = ET.Element("{DAV:}multistatus")

    # Root
    root_resp = _to_xml_response("/webdav/", True, "Stora", 0, _fmt_date(datetime.utcnow()))
    multistatus.append(root_resp)

    if depth != "0":
        # List files at root
        files = (await db.execute(
            select(FileItem).where(
                FileItem.user_id == user.id, FileItem.deleted_at.is_(None), FileItem.folder_id.is_(None)
            ).order_by(FileItem.filename)
        )).scalars().all()
        folders = (await db.execute(
            select(Folder).where(Folder.user_id == user.id, Folder.parent_id.is_(None)).order_by(Folder.name)
        )).scalars().all()

        for f in folders:
            multistatus.append(_to_xml_response(f"/webdav/{f.name}/", True, f.name))
        for f in files:
            multistatus.append(_to_xml_response(
                f"/webdav/{f.filename}", False, f.filename, f.file_size or 0, _fmt_date(f.updated_at or f.created_at)
            ))

    return Response(content=_xml_to_bytes(multistatus), media_type="application/xml; charset=utf-8")


@router.request("GET", "/webdav/{path:path}", include_in_schema=False)
async def webdav_get(
    path: str,
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """Download file (WebDAV GET)"""
    filename = path.strip("/")
    file = (await db.execute(
        select(FileItem).where(
            FileItem.user_id == user.id, FileItem.filename == filename, FileItem.deleted_at.is_(None)
        )
    )).scalar_one_or_none()
    if not file:
        raise HTTPException(status_code=404)

    storage_path = getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        raise HTTPException(status_code=404)

    from fastapi.responses import FileResponse
    return FileResponse(storage_path, filename=file.filename, media_type=file.mime_type or "application/octet-stream")


@router.request("PUT", "/webdav/{path:path}", include_in_schema=False)
async def webdav_put(
    path: str,
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """Upload file (WebDAV PUT)"""
    filename = path.strip("/")
    content = await request.body()

    from shared.services.files import create_file
    try:
        await create_file(db, user.id, filename, content)
    except ValueError as e:
        raise HTTPException(status_code=507, detail=str(e))

    return Response(status_code=201)


@router.request("MKCOL", "/webdav/{path:path}", include_in_schema=False)
async def webdav_mkcol(
    path: str,
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """Create folder (WebDAV MKCOL)"""
    name = path.strip("/")
    folder = Folder(user_id=user.id, name=name)
    db.add(folder)
    await db.commit()
    return Response(status_code=201)


@router.request("DELETE", "/webdav/{path:path}", include_in_schema=False)
async def webdav_delete(
    path: str,
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """Delete file (move to trash)"""
    filename = path.strip("/")
    file = (await db.execute(
        select(FileItem).where(FileItem.user_id == user.id, FileItem.filename == filename)
    )).scalar_one_or_none()
    if file:
        from sqlalchemy import func
        file.deleted_at = func.now()
        await db.commit()
    return Response(status_code=204)


@router.request("MOVE", "/webdav/{path:path}", include_in_schema=False)
async def webdav_move(
    path: str,
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    user: User = Depends(webdav_auth),
):
    """Rename/move file"""
    dest = request.headers.get("Destination", "")
    dest_filename = dest.split("/")[-1]
    filename = path.strip("/")

    file = (await db.execute(
        select(FileItem).where(FileItem.user_id == user.id, FileItem.filename == filename)
    )).scalar_one_or_none()
    if file:
        file.filename = dest_filename
        await db.commit()

    return Response(status_code=204)
