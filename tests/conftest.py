"""pytest configuration for Stora tests"""
import pytest


@pytest.fixture(scope="session")
def anyio_backend():
    return "asyncio"
