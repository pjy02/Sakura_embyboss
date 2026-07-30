import asyncio
import os
import socket

from bot import LOGGER
from bot.application.reliability_service import ReliabilityService


class EventRelay:
    def __init__(self, poll_seconds: float = 1.0):
        self.poll_seconds = poll_seconds
        self.service = ReliabilityService()
        self.worker_id = f"event-relay:{socket.gethostname()}:{os.getpid()}"
        self._version = 0
        self._condition = asyncio.Condition()
        self._task: asyncio.Task | None = None
        self._stopping = asyncio.Event()

    @property
    def version(self) -> int:
        return self._version

    async def start(self):
        if self._task and not self._task.done():
            return
        self._stopping.clear()
        self._task = asyncio.create_task(self._run(), name="sakura-event-relay")

    async def stop(self):
        self._stopping.set()
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            except Exception as error:
                LOGGER.error("Event relay stopped after an error: %s", error)
        try:
            await asyncio.to_thread(
                self.service.heartbeat,
                worker_id=self.worker_id,
                worker_kind="event-relay",
                status="stopped",
            )
        except Exception as error:
            LOGGER.error("Failed to persist event relay shutdown: %s", error)

    async def wait_for_change(self, version: int, timeout: float = 10.0) -> int:
        if self._version != version:
            return self._version
        try:
            async with self._condition:
                await asyncio.wait_for(
                    self._condition.wait_for(lambda: self._version != version),
                    timeout=timeout,
                )
        except asyncio.TimeoutError:
            pass
        return self._version

    async def _run(self):
        try:
            await asyncio.to_thread(
                self.service.heartbeat,
                worker_id=self.worker_id,
                worker_kind="event-relay",
                status="running",
            )
        except Exception as error:
            LOGGER.error("Initial event relay heartbeat failed: %s", error)
        heartbeat_counter = 0
        cleanup_counter = 0
        while not self._stopping.is_set():
            try:
                events = await asyncio.to_thread(self.service.dispatch_outbox, 100)
                if events:
                    async with self._condition:
                        self._version += 1
                        self._condition.notify_all()
                heartbeat_counter += 1
                cleanup_counter += 1
                if heartbeat_counter >= 15:
                    heartbeat_counter = 0
                    await asyncio.to_thread(
                        self.service.heartbeat,
                        worker_id=self.worker_id,
                        worker_kind="event-relay",
                        status="running",
                        metadata={"last_batch_size": len(events)},
                    )
                if cleanup_counter >= max(1, int(3600 / self.poll_seconds)):
                    cleanup_counter = 0
                    cleanup = await asyncio.to_thread(self.service.cleanup)
                    LOGGER.info("Reliability history cleanup completed: %s", cleanup)
            except asyncio.CancelledError:
                raise
            except Exception as error:
                LOGGER.error("Event relay failed: %s", error)
                await asyncio.sleep(min(5.0, self.poll_seconds * 2))
                continue
            await asyncio.sleep(self.poll_seconds)


event_relay = EventRelay()
