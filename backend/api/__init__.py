from .admin import router as admin_router
from .auth import router as auth_router
from .commerce import admin_router as commerce_admin_router
from .commerce import me_router as commerce_me_router
from .community import admin_router as community_admin_router
from .community import me_router as community_me_router
from .me import router as me_router
from .operations import router as operations_router
from .tasks import admin_router as tasks_router
from .tasks import events_router

__all__ = [
    "admin_router",
    "auth_router",
    "commerce_admin_router",
    "commerce_me_router",
    "community_admin_router",
    "community_me_router",
    "events_router",
    "me_router",
    "operations_router",
    "tasks_router",
]
