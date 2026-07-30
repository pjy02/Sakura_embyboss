from .admin import router as admin_router
from .auth import router as auth_router
from .me import router as me_router

__all__ = ["admin_router", "auth_router", "me_router"]
