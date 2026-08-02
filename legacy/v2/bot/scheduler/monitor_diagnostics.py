import os

from bot import LOGGER
from bot.application import DiagnosticService
from bot.func_helper.scheduler import scheduler


diagnostics = DiagnosticService()


async def monitor_diagnostics():
    try:
        return await diagnostics.run()
    except Exception as error:
        LOGGER.error(f"系统诊断任务失败: {error}")
        return {"status": "failed", "error": str(error)}


try:
    interval = max(30, int(os.getenv("SAKURA_DIAGNOSTIC_INTERVAL_SECONDS", "60")))
except ValueError:
    interval = 60

scheduler.add_job(
    monitor_diagnostics,
    "interval",
    seconds=interval,
    id="monitor_diagnostics",
)
