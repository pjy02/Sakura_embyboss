from .admin import router as admin_router
from .auth import router as auth_router
from .me import router as me_router
from .tasks import admin_router as tasks_router
from .tasks import events_router

__all__ = [
    "admin_router",
    "auth_router",
    "events_router",
    "me_router",
    "tasks_router",
]
