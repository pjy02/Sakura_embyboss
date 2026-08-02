"""Standalone durable task worker.

This process does not start Telegram MTProto polling.  Registration, lifecycle,
diagnostic and integration tasks therefore continue when the Bot service is
disabled or reconnecting.
"""

import asyncio
import os
import signal
import time
from datetime import datetime

from bot import LOGGER
from bot.application import DynamicSettingsService, TaskService
from bot.domain import Actor
from bot.workers.task_worker import TaskWorker


async def platform_scheduler(stop: asyncio.Event) -> None:
    """Enqueue recurring platform jobs without depending on Telegram polling."""
    settings = DynamicSettingsService()
    task_service = TaskService()
    last_slots: dict[str, str] = {}

    async def enqueue(task_type: str, key: str) -> None:
        if last_slots.get(task_type) == key:
            return
        result = await asyncio.to_thread(
            task_service.enqueue,
            task_type=task_type,
            payload={"scheduled_at": datetime.utcnow().isoformat()},
            actor=Actor.system("platform-scheduler"),
            idempotency_key=f"scheduler:{task_type}:{key}",
        )
        if result.ok:
            last_slots[task_type] = key

    while not stop.is_set():
        try:
            automation_enabled = bool((await asyncio.to_thread(settings.get, "scheduler.automation_enabled"))["value"])
            automation_seconds = int((await asyncio.to_thread(settings.get, "scheduler.automation_seconds"))["value"])
            diagnostics_minutes = int((await asyncio.to_thread(settings.get, "scheduler.diagnostics_minutes"))["value"])
            backup_enabled = bool((await asyncio.to_thread(settings.get, "scheduler.database_backup"))["value"])
            backup_hour = int((await asyncio.to_thread(settings.get, "scheduler.backup_hour"))["value"])
            moviepilot_enabled = bool((await asyncio.to_thread(settings.get, "integrations.moviepilot_enabled"))["value"])
            try:
                operations_seconds = max(10, int(os.getenv("SAKURA_OPERATIONS_SYNC_SECONDS", "30")))
            except ValueError:
                operations_seconds = 30
            now = datetime.now()
            epoch = int(time.time())
            if automation_enabled:
                await enqueue("automation.evaluate", str(epoch // max(5, automation_seconds)))
            diagnostic_slot = str(epoch // max(60, diagnostics_minutes * 60))
            await enqueue("monitor.diagnostics", diagnostic_slot)
            await enqueue("monitor.emby_instances", diagnostic_slot)
            if moviepilot_enabled:
                await enqueue("sync.moviepilot", str(epoch // 60))
            await enqueue("sync.core_operations", str(epoch // operations_seconds))
            if backup_enabled and now.hour == backup_hour:
                await enqueue("maintenance.backup_database", now.strftime("%Y%m%d"))
        except Exception as exc:
            LOGGER.error(f"Platform scheduler error: {exc}")
        try:
            await asyncio.wait_for(stop.wait(), timeout=5)
        except asyncio.TimeoutError:
            pass


async def main() -> None:
    settings = DynamicSettingsService()
    await asyncio.to_thread(settings.materialize_defaults)
    await asyncio.to_thread(settings.apply_runtime_overrides)
    worker_count = int(
        (await asyncio.to_thread(settings.get, "registration.worker_count"))["value"]
    )
    workers = [TaskWorker() for _index in range(max(1, min(worker_count, 50)))]
    loop = asyncio.get_running_loop()
    stop = asyncio.Event()

    def request_stop() -> None:
        stop.set()

    for signame in ("SIGINT", "SIGTERM"):
        signum = getattr(signal, signame, None)
        if signum is not None:
            try:
                loop.add_signal_handler(signum, request_stop)
            except NotImplementedError:
                pass

    runners = [
        asyncio.create_task(worker.run(), name=f"sakura-core-worker-{index + 1}")
        for index, worker in enumerate(workers)
    ]
    runners.append(asyncio.create_task(platform_scheduler(stop), name="sakura-platform-scheduler"))
    LOGGER.info("Sakura standalone core workers started: %s", len(workers))
    await stop.wait()
    await asyncio.gather(*(worker.stop() for worker in workers))
    for runner in runners:
        if not runner.done():
            runner.cancel()


if __name__ == "__main__":
    asyncio.run(main())
