import os

from bot import LOGGER
from bot.application import CoreOperationsService
from bot.func_helper.scheduler import scheduler


operations = CoreOperationsService()


async def sync_core_operations():
    result = await operations.sync_live_sessions()
    if result["source"] != "emby":
        LOGGER.warning(f"核心运营数据同步失败: {result.get('error')}")
    return {
        "source": result["source"],
        "live_sessions": result["total"],
        "error": result.get("error"),
    }


try:
    sync_seconds = max(10, int(os.getenv("SAKURA_OPERATIONS_SYNC_SECONDS", "30")))
except ValueError:
    sync_seconds = 30
    LOGGER.warning("SAKURA_OPERATIONS_SYNC_SECONDS 无效，已使用默认值 30 秒")
scheduler.add_job(
    sync_core_operations,
    "interval",
    seconds=sync_seconds,
    id="sync_core_operations",
)
