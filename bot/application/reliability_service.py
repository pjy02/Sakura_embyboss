import json
import os
import socket
from datetime import timedelta
from typing import Optional

from sqlalchemy import func, or_

from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import (
    IdempotencyRecord,
    JobRun,
    OperationTask,
    SystemEvent,
    WorkerHeartbeat,
    utcnow,
)


def _loads(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


def serialize_event(row) -> dict:
    return {
        "id": row.id,
        "event_type": row.event_type,
        "aggregate_type": row.aggregate_type,
        "aggregate_id": row.aggregate_id,
        "payload": _loads(row.payload_json),
        "created_at": row.created_at,
    }


class ReliabilityService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def events_after(
        self,
        *,
        after_id: int,
        limit: int = 100,
        user_tg: Optional[int] = None,
        event_prefixes: Optional[tuple[str, ...]] = None,
    ) -> list[dict]:
        with self._uow_factory() as uow:
            rows = uow.operations.list_events_after(
                after_id=after_id,
                limit=limit,
                aggregate_type="user" if user_tg is not None else None,
                aggregate_id=str(user_tg) if user_tg is not None else None,
                event_prefixes=event_prefixes,
            )
            return [serialize_event(row) for row in rows]

    def dispatch_outbox(self, limit: int = 100) -> list[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            rows = uow.operations.list_unpublished_events(limit)
            events = [serialize_event(row) for row in rows]
            uow.operations.mark_events_published(
                [int(row.id) for row in rows],
                now,
            )
            return events

    def latest_event_id(
        self,
        user_tg: Optional[int] = None,
        event_prefixes: Optional[tuple[str, ...]] = None,
    ) -> int:
        with self._uow_factory() as uow:
            query = uow.operations.session.query(func.max(SystemEvent.id))
            if user_tg is not None:
                query = query.filter(
                    SystemEvent.aggregate_type == "user",
                    SystemEvent.aggregate_id == str(user_tg),
                )
            if event_prefixes is not None:
                if not event_prefixes:
                    return 0
                query = query.filter(
                    or_(
                        *[
                            SystemEvent.event_type.like(f"{prefix}.%")
                            for prefix in event_prefixes
                        ]
                    )
                )
            return int(query.scalar() or 0)

    def heartbeat(
        self,
        *,
        worker_id: str,
        worker_kind: str,
        status: str,
        current_task_id: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> None:
        with self._uow_factory() as uow:
            uow.operations.upsert_worker_heartbeat(
                worker_id=worker_id,
                worker_kind=worker_kind,
                hostname=socket.gethostname(),
                process_id=os.getpid(),
                status=status,
                current_task_id=current_task_id,
                metadata=metadata,
                now=utcnow(),
            )

    def status(self, stale_after_seconds: int = 45) -> dict:
        now = utcnow()
        stale_before = now - timedelta(seconds=stale_after_seconds)
        with self._uow_factory() as uow:
            workers = []
            for row in uow.operations.list_worker_heartbeats():
                stale = row.last_seen_at < stale_before
                workers.append(
                    {
                        "worker_id": row.worker_id,
                        "worker_kind": row.worker_kind,
                        "hostname": row.hostname,
                        "process_id": row.process_id,
                        "status": "stale" if stale else row.status,
                        "current_task_id": row.current_task_id,
                        "metadata": _loads(row.metadata_json),
                        "started_at": row.started_at,
                        "last_seen_at": row.last_seen_at,
                        "stale": stale,
                    }
                )
            counts = {
                status: int(
                    uow.operations.session.query(func.count(OperationTask.id))
                    .filter(OperationTask.status == status)
                    .scalar()
                    or 0
                )
                for status in (
                    "pending",
                    "retrying",
                    "running",
                    "succeeded",
                    "failed",
                    "canceled",
                )
            }
            oldest_pending = (
                uow.operations.session.query(func.min(OperationTask.created_at))
                .filter(OperationTask.status.in_(("pending", "retrying")))
                .scalar()
            )
            active_workers = [
                item
                for item in workers
                if not item["stale"] and item["status"] != "stopped"
            ]
            task_workers = [
                item for item in active_workers if item["worker_kind"] == "task-worker"
            ]
            event_relays = [
                item for item in active_workers if item["worker_kind"] == "event-relay"
            ]
            components = {
                "task_worker": "healthy" if task_workers else "degraded",
                "event_relay": "healthy" if event_relays else "degraded",
            }
            return {
                "status": (
                    "healthy"
                    if all(value == "healthy" for value in components.values())
                    else "degraded"
                ),
                "components": components,
                "workers": workers,
                "task_counts": counts,
                "oldest_pending_at": oldest_pending,
                "checked_at": now,
            }

    def cleanup(
        self,
        *,
        event_days: int = 7,
        completed_task_days: int = 30,
        job_run_days: int = 90,
        heartbeat_days: int = 7,
    ) -> dict:
        now = utcnow()
        with self._uow_factory() as uow:
            session = uow.operations.session
            deleted_events = (
                session.query(SystemEvent)
                .filter(
                    SystemEvent.published_at.isnot(None),
                    SystemEvent.created_at < now - timedelta(days=event_days),
                )
                .delete(synchronize_session=False)
            )
            deleted_tasks = (
                session.query(OperationTask)
                .filter(
                    OperationTask.status.in_(("succeeded", "canceled")),
                    OperationTask.finished_at
                    < now - timedelta(days=completed_task_days),
                )
                .delete(synchronize_session=False)
            )
            deleted_jobs = (
                session.query(JobRun)
                .filter(JobRun.finished_at < now - timedelta(days=job_run_days))
                .delete(synchronize_session=False)
            )
            deleted_heartbeats = (
                session.query(WorkerHeartbeat)
                .filter(
                    WorkerHeartbeat.last_seen_at
                    < now - timedelta(days=heartbeat_days)
                )
                .delete(synchronize_session=False)
            )
            deleted_idempotency = (
                session.query(IdempotencyRecord)
                .filter(
                    IdempotencyRecord.expires_at.isnot(None),
                    IdempotencyRecord.expires_at < now,
                )
                .delete(synchronize_session=False)
            )
            return {
                "events": deleted_events,
                "tasks": deleted_tasks,
                "job_runs": deleted_jobs,
                "heartbeats": deleted_heartbeats,
                "idempotency_records": deleted_idempotency,
            }
