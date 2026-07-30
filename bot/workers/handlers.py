import os
from typing import Any, Awaitable, Callable


TaskHandler = Callable[[dict], Awaitable[dict]]


async def sync_favorites_handler(_payload: dict) -> dict:
    from bot.scheduler.sync_favorites import sync_favorites

    result = await sync_favorites()
    return result if isinstance(result, dict) else {"completed": True}


async def sync_moviepilot_handler(_payload: dict) -> dict:
    from bot.scheduler.sync_mp_download import sync_download_tasks

    result = await sync_download_tasks()
    return result if isinstance(result, dict) else {"completed": True}


async def partition_access_handler(_payload: dict) -> dict:
    from bot.scheduler.partition_access import check_partition_access

    result = await check_partition_access()
    return result if isinstance(result, dict) else {"completed": True}


async def expired_accounts_handler(_payload: dict) -> dict:
    from bot.scheduler.check_ex import check_expired

    result = await check_expired()
    return result if isinstance(result, dict) else {"completed": True}


async def backup_database_handler(_payload: dict) -> dict:
    from bot.scheduler.backup_db import DbBackupUtils

    backup_file = await DbBackupUtils.backup_db()
    if not backup_file:
        raise RuntimeError("Database backup did not produce an output file")
    return {
        "completed": True,
        "filename": os.path.basename(str(backup_file)),
    }


TASK_HANDLERS: dict[str, TaskHandler] = {
    "sync.favorites": sync_favorites_handler,
    "sync.moviepilot": sync_moviepilot_handler,
    "maintenance.partition_access": partition_access_handler,
    "maintenance.expired_accounts": expired_accounts_handler,
    "maintenance.backup_database": backup_database_handler,
}
