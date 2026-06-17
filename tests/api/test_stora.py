"""
Stora API Tests — pytest with httpx AsyncClient

Run:  cd tests && pytest -v
"""
import pytest
from httpx import ASGITransport, AsyncClient

# Import the app — may need path adjustments
import sys, os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.app import create_app
from src.setting import TestingConfig

# Use test config
import src.setting
src.setting.settings = TestingConfig()

app = create_app(TestingConfig())


@pytest.fixture
async def client():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac


@pytest.fixture
async def auth_client(client: AsyncClient):
    """Return an authenticated client"""
    resp = await client.post("/api/v2/auth/register", data={
        "username": "testuser",
        "email": "test@stora.dev",
        "password": "test123456",
        "password_confirm": "test123456",
    })
    resp = await client.post("/api/v2/auth/login", data={
        "username": "testuser",
        "password": "test123456",
    })
    data = resp.json()
    token = data.get("data", {}).get("access_token", "")
    client.headers.update({"Authorization": f"Bearer {token}"})
    return client


# ─── Auth Tests ───

@pytest.mark.asyncio
async def test_register(client: AsyncClient):
    resp = await client.post("/api/v2/auth/register", data={
        "username": "newuser",
        "email": "new@stora.dev",
        "password": "pass123456",
        "password_confirm": "pass123456",
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True
    assert data["data"]["username"] == "newuser"


@pytest.mark.asyncio
async def test_register_password_mismatch(client: AsyncClient):
    resp = await client.post("/api/v2/auth/register", data={
        "username": "user1",
        "email": "u1@stora.dev",
        "password": "abc123",
        "password_confirm": "abc456",
    })
    assert resp.json()["success"] is False


@pytest.mark.asyncio
async def test_login_success(client: AsyncClient):
    # Register first
    await client.post("/api/v2/auth/register", data={
        "username": "loginuser", "email": "login@stora.dev",
        "password": "pass123456", "password_confirm": "pass123456",
    })
    resp = await client.post("/api/v2/auth/login", data={
        "username": "loginuser", "password": "pass123456",
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True
    assert "access_token" in data["data"]


@pytest.mark.asyncio
async def test_login_wrong_password(client: AsyncClient):
    resp = await client.post("/api/v2/auth/login", data={
        "username": "nonexistent", "password": "wrong",
    })
    assert resp.json()["success"] is False


@pytest.mark.asyncio
async def test_get_profile(auth_client: AsyncClient):
    resp = await auth_client.get("/api/v2/auth/me")
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True


# ─── File Tests ───

@pytest.mark.asyncio
async def test_create_folder(auth_client: AsyncClient):
    resp = await auth_client.post("/api/v2/files/folders", params={"name": "TestFolder"})
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True
    assert data["data"]["name"] == "TestFolder"


@pytest.mark.asyncio
async def test_list_files(auth_client: AsyncClient):
    resp = await auth_client.get("/api/v2/files")
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True


@pytest.mark.asyncio
async def test_upload_file(auth_client: AsyncClient):
    resp = await auth_client.post("/api/v2/files/upload", files={
        "file": ("hello.txt", b"Hello Stora!", "text/plain"),
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True
    assert data["data"]["file"]["filename"] == "hello.txt"


@pytest.mark.asyncio
async def test_upload_and_download(auth_client: AsyncClient):
    # Upload
    resp = await auth_client.post("/api/v2/files/upload", files={
        "file": ("download_test.txt", b"Download me!", "text/plain"),
    })
    file_id = resp.json()["data"]["file"]["id"]

    # Download
    resp = await auth_client.get(f"/api/v2/files/download/{file_id}")
    assert resp.status_code == 200
    assert b"Download me!" in resp.content


@pytest.mark.asyncio
async def test_share_file(auth_client: AsyncClient):
    # Upload a file first
    resp = await auth_client.post("/api/v2/files/upload", files={
        "file": ("share_test.txt", b"Share me!", "text/plain"),
    })
    file_id = resp.json()["data"]["file"]["id"]

    # Create share
    resp = await auth_client.post("/api/v2/files/shares", data={
        "file_id": file_id,
        "permission": "read",
    })
    assert resp.status_code == 200
    assert resp.json()["success"] is True


@pytest.mark.asyncio
async def test_get_quota(auth_client: AsyncClient):
    resp = await auth_client.get("/api/v2/users/me/quota")
    assert resp.status_code == 200
    data = resp.json()
    assert data["success"] is True
    assert "max_storage" in data["data"]
