"""
Stora API v3 — Mobile & Admin API routes
"""
from fastapi import APIRouter

ROUTE_REGISTRY_V3 = [
    ("src.api.v3.mobile.auth", "/api/v3/auth", ["mobile-auth"], True),
    ("src.api.v3.mobile.media", "/api/v3/media", ["mobile-media"], True),
    ("src.api.v3.mobile.users", "/api/v3/users", ["mobile-users"], True),
    ("src.api.v3.admin.dashboard", "/api/v3/admin/dashboard", ["admin-dashboard"], True),
    ("src.api.v3.admin.users", "/api/v3/admin/users", ["admin-users"], True),
    ("src.api.v3.admin.media", "/api/v3/admin/media", ["admin-media"], True),
    ("src.api.v3.admin.system", "/api/v3/admin/system", ["admin-system"], True),
    ("src.api.v3.admin.roles", "/api/v3/admin/roles", ["admin-roles"], True),
    ("src.api.v3.admin.notifications", "/api/v3/admin/notifications", ["admin-notifications"], True),
    ("src.api.v3.admin.backup", "/api/v3/admin/backup", ["admin-backup"], False),
]
