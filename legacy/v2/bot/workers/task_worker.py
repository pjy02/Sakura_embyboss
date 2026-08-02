import asyncio
import os
import socket
import traceback
from contextlib import suppress
from uuid import uuid4

from bot import LOGGER
from bot.application.reliability_service import ReliabilityService
from bot.application.governance_service import DynamicSettingsService
from bot.application.task_service import TASK_DEFINITIONS, TaskService
from bot.workers.handlers import TASK_HANDLERS


class TaskWorker:
    def __init__(
        self,
        *,
        poll_seconds: float | None = None,
        lease_seconds: int | None = None,
        heartbeat_seconds: int | None = None,
    ):
        self.poll_seconds = poll_seconds or float(
            os.getenv("SAKURA_TASK_POLL_SECONDS", "1")
        )
        self.lease_seconds = lease_seconds or int(
            os.getenv("SAKURA_TASK_LEASE_SECONDS", "45")
        )
        self.heartbeat_seconds = heartbeat_seconds or int(
            os.getenv("SAKURA_TASK_HEARTBEAT_SECONDS", "12")
        )
        self.worker_id = f"task:{socket.gethostname()}:{uuid4().hex[:10]}"
        self.tasks = TaskService()
        self.reliability = ReliabilityService()
        self.settings = DynamicSettingsService()
        self._runner: asyncio.Task | None = None
        self._stopping = asyncio.Event()
        self._last_worker_heartbeat = 0.0
        self._last_worker_state: tuple[str, str | None] | None = None

    def schedule(self):
        if self._runner and not self._runner.done():
            return self._runner
        loop = asyncio.get_event_loop()
        self._stopping.clear()
        self._runner = loop.create_task(self.run(), name="sakura-task-worker")
        return self._runner

    async def stop(self):
        self._stopping.set()
        if self._runner:
            self._runner.cancel()
            with suppress(asyncio.CancelledError):
                await self._runner

    async def run(self):
        LOGGER.info("Task worker started: %s", self.worker_id)
        await self._heartbeat("idle")
        try:
            while not self._stopping.is_set():
                try:
                    task = await asyncio.to_thread(
                        self.tasks.claim,
                        worker_id=self.worker_id,
                        lease_seconds=self.lease_seconds,
                    )
                except Exception as error:
                    LOGGER.error("Task worker queue poll failed: %s", error)
                    await asyncio.sleep(min(15.0, max(2.0, self.poll_seconds * 4)))
                    continue
                if not task:
                    await self._heartbeat("idle")
                    await asyncio.sleep(self.poll_seconds)
                    continue
                await self._execute(task)
        except asyncio.CancelledError:
            raise
        finally:
            await self._heartbeat("stopped")
            LOGGER.info("Task worker stopped: %s", self.worker_id)

    async def _execute(self, task: dict):
        task_id = task["id"]
        task_type = task["task_type"]
        handler = TASK_HANDLERS.get(task_type)
        definition = TASK_DEFINITIONS.get(task_type)
        if not handler or not definition:
            await asyncio.to_thread(
                self.tasks.fail,
                task_id,
                self.worker_id,
                f"No handler registered for task type {task_type}",
            )
            return

        await self._heartbeat("busy", task_id)
        LOGGER.info("Task started: %s (%s)", task_id, task_type)
        try:
            await asyncio.to_thread(self.settings.apply_runtime_overrides)
        except Exception as error:
            LOGGER.warning("Task worker could not refresh dynamic settings: %s", error)
        handler_task = asyncio.create_task(
            handler(task.get("input") or {}),
            name=f"sakura-task-{task_id}",
        )
        deadline = asyncio.get_running_loop().time() + definition.timeout_seconds
        try:
            while True:
                remaining = deadline - asyncio.get_running_loop().time()
                if remaining <= 0:
                    handler_task.cancel()
                    with suppress(asyncio.CancelledError, Exception):
                        await handler_task
                    raise TimeoutError(
                        f"Task exceeded timeout of {definition.timeout_seconds} seconds"
                    )
                done, _pending = await asyncio.wait(
                    {handler_task},
                    timeout=min(self.heartbeat_seconds, remaining),
                )
                if done:
                    result = await handler_task
                    await asyncio.to_thread(
                        self.tasks.complete,
                        task_id,
                        self.worker_id,
                        result,
                    )
                    LOGGER.info("Task completed: %s (%s)", task_id, task_type)
                    return
                should_continue = await asyncio.to_thread(
                    self.tasks.heartbeat,
                    task_id,
                    self.worker_id,
                    self.lease_seconds,
                )
                await self._heartbeat("busy", task_id)
                if not should_continue:
                    handler_task.cancel()
                    with suppress(asyncio.CancelledError, Exception):
                        await handler_task
                    await asyncio.to_thread(
                        self.tasks.complete,
                        task_id,
                        self.worker_id,
                        {"canceled": True},
                    )
                    LOGGER.info("Task canceled: %s (%s)", task_id, task_type)
                    return
        except asyncio.CancelledError:
            handler_task.cancel()
            raise
        except Exception as error:
            if not handler_task.done():
                handler_task.cancel()
                with suppress(asyncio.CancelledError, Exception):
                    await handler_task
            message = f"{type(error).__name__}: {error}"
            LOGGER.error(
                "Task failed: %s (%s): %s\n%s",
                task_id,
                task_type,
                message,
                traceback.format_exc(),
            )
            try:
                await asyncio.to_thread(
                    self.tasks.fail,
                    task_id,
                    self.worker_id,
                    message,
                )
            except Exception as persist_error:
                LOGGER.error(
                    "Failed to persist task failure for %s: %s",
                    task_id,
                    persist_error,
                )
        finally:
            await self._heartbeat("idle")

    async def _heartbeat(self, status: str, task_id: str | None = None):
        now = asyncio.get_running_loop().time()
        state = (status, task_id)
        if (
            self._last_worker_state == state
            and now - self._last_worker_heartbeat < self.heartbeat_seconds
        ):
            return
        try:
            await asyncio.to_thread(
                self.reliability.heartbeat,
                worker_id=self.worker_id,
                worker_kind="task-worker",
                status=status,
                current_task_id=task_id,
                metadata={
                    "poll_seconds": self.poll_seconds,
                    "lease_seconds": self.lease_seconds,
                },
            )
            self._last_worker_state = state
            self._last_worker_heartbeat = now
        except Exception as error:
            LOGGER.error("Task worker heartbeat failed: %s", error)


task_worker = TaskWorker()


def schedule_task_worker():
    if os.getenv("SAKURA_TASK_WORKER_ENABLED", "1").strip().lower() in {
        "0",
        "false",
        "no",
        "off",
    }:
        LOGGER.info("Task worker is disabled by SAKURA_TASK_WORKER_ENABLED")
        return None
    return task_worker.schedule()
