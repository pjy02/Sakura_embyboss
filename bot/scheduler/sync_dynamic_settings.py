import asyncio

from bot import LOGGER
from bot.application import DynamicSettingsService
from bot.func_helper.scheduler import scheduler


settings_service = DynamicSettingsService()


async def sync_dynamic_settings():
    result = await asyncio.to_thread(settings_service.apply_runtime_overrides)
    return {"applied": result["count"]}


scheduler.add_job(
    sync_dynamic_settings,
    "interval",
    seconds=15,
    id="sync_dynamic_settings",
)
LOGGER.info("动态设置同步任务已启用")
