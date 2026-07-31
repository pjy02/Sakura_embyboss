"""Standalone durable task worker.

This process does not start Telegram MTProto polling.  Registration, lifecycle,
diagnostic and integration tasks therefore continue when the Bot service is
disabled or reconnecting.
"""

import asyncio
import signal

from bot import LOGGER
from bot.application import DynamicSettingsService
from bot.workers.task_worker import TaskWorker


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
    LOGGER.info("Sakura standalone core workers started: %s", len(workers))
    await stop.wait()
    await asyncio.gather(*(worker.stop() for worker in workers))
    for runner in runners:
        if not runner.done():
            runner.cancel()


if __name__ == "__main__":
    asyncio.run(main())
