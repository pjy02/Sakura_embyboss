import json
from dataclasses import dataclass
from datetime import timedelta
from typing import Any, Optional
from uuid import uuid4

from sqlalchemy.exc import IntegrityError

from bot.application.results import ServiceResult
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import JobRun, OperationTask, utcnow


@dataclass(frozen=True)
class TaskDefinition:
    task_type: str
    label: str
    description: str
    risk: str
    timeout_seconds: int
    max_retries: int
    admin_exposed: bool = True


TASK_DEFINITIONS = {
    item.task_type: item
    for item in (
        TaskDefinition(
            "sync.favorites",
            "同步 Emby 收藏",
            "从 Emby 拉取全部用户收藏并同步到本地数据库。",
            "normal",
            1800,
            2,
        ),
        TaskDefinition(
            "sync.moviepilot",
            "同步 MoviePilot 状态",
            "刷新下载和入库任务状态。",
            "normal",
            600,
            3,
        ),
        TaskDefinition(
            "maintenance.partition_access",
            "检查分区授权",
            "回收已到期分区授权并通知受影响用户。",
            "warning",
            900,
            2,
        ),
        TaskDefinition(
            "maintenance.expired_accounts",
            "检查到期账户",
            "执行账户续期、冻结及 Emby 策略更新。",
            "danger",
            1800,
            1,
        ),
        TaskDefinition(
            "maintenance.backup_database",
            "备份数据库",
            "创建数据库备份并按保留策略清理旧文件。",
            "warning",
            1800,
            1,
        ),
        TaskDefinition(
            "registration.account",
            "创建 Emby 账号",
            "通过共享注册队列创建账号并写入用户资料。",
            "normal",
            180,
            0,
            False,
        ),
    )
}


def _loads(value: Optional[str]):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


def _sanitized_task_data(task_type: str, value, include_sensitive: bool):
    if include_sensitive or task_type != "registration.account":
        return value
    if not isinstance(value, dict):
        return value
    sanitized = dict(value)
    for key in ("safety_code", "registration_code", "emby_password"):
        if key in sanitized:
            sanitized[key] = "********"
    return sanitized


def serialize_task(row: OperationTask, *, include_sensitive: bool = False) -> dict:
    input_data = _loads(row.input_json)
    result_data = _loads(row.result_json)
    return {
        "id": row.id,
        "task_type": row.task_type,
        "label": TASK_DEFINITIONS.get(
            row.task_type,
            TaskDefinition(row.task_type, row.task_type, "", "normal", 600, 0),
        ).label,
        "status": row.status,
        "progress": int(row.progress or 0),
        "owner_kind": row.owner_kind,
        "owner_id": row.owner_id,
        "input": _sanitized_task_data(row.task_type, input_data, include_sensitive),
        "result": _sanitized_task_data(row.task_type, result_data, include_sensitive),
        "error_message": row.error_message,
        "retry_count": int(row.retry_count or 0),
        "max_retries": int(row.max_retries or 0),
        "next_run_at": row.next_run_at,
        "locked_by": row.locked_by,
        "cancel_requested": bool(row.cancel_requested),
        "created_at": row.created_at,
        "started_at": row.started_at,
        "finished_at": row.finished_at,
        "updated_at": row.updated_at,
    }


class TaskService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def definitions(self) -> list[dict]:
        return [
            {
                "task_type": item.task_type,
                "label": item.label,
                "description": item.description,
                "risk": item.risk,
                "timeout_seconds": item.timeout_seconds,
                "max_retries": item.max_retries,
            }
            for item in TASK_DEFINITIONS.values()
            if item.admin_exposed
        ]

    def enqueue(
        self,
        *,
        task_type: str,
        payload: Optional[dict],
        actor: Actor,
        idempotency_key: Optional[str],
    ) -> ServiceResult:
        definition = TASK_DEFINITIONS.get(task_type)
        if not definition:
            return ServiceResult("unsupported_task")
        now = utcnow()
        try:
            with self._uow_factory() as uow:
                existing = uow.operations.get_task_by_idempotency_key(idempotency_key)
                if existing:
                    return ServiceResult("ok", serialize_task(existing))
                task = OperationTask(
                    id=str(uuid4()),
                    task_type=task_type,
                    status="pending",
                    progress=0,
                    owner_kind=actor.kind,
                    owner_id=actor.identifier,
                    idempotency_key=idempotency_key,
                    input_json=json.dumps(payload or {}, ensure_ascii=False, default=str),
                    retry_count=0,
                    max_retries=definition.max_retries,
                    next_run_at=now,
                    cancel_requested=False,
                    created_at=now,
                    updated_at=now,
                )
                uow.operations.add_task(task)
                uow.operations.audit(
                    actor=actor,
                    action="task.enqueue",
                    resource_type="operation_task",
                    resource_id=task.id,
                    detail={"task_type": task_type},
                )
                uow.operations.event(
                    "task.created",
                    "operation_task",
                    task.id,
                    {"task_id": task.id, "task_type": task_type, "status": "pending"},
                )
                uow.flush()
                return ServiceResult("ok", serialize_task(task))
        except IntegrityError:
            if not idempotency_key:
                raise
            with self._uow_factory() as uow:
                existing = uow.operations.get_task_by_idempotency_key(idempotency_key)
                if existing:
                    return ServiceResult("ok", serialize_task(existing))
            raise

    def get(self, task_id: str) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.operations.get_task(task_id)
            return serialize_task(row) if row else None

    def list(
        self,
        *,
        statuses: Optional[list[str]] = None,
        task_type: Optional[str] = None,
        owner_id: Optional[str] = None,
        limit: int = 50,
        offset: int = 0,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.operations.list_tasks(
                statuses=statuses,
                task_type=task_type,
                owner_id=owner_id,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_task(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def cancel(self, task_id: str, actor: Actor) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            task = uow.operations.get_task(task_id, for_update=True)
            if not task:
                return ServiceResult("not_found")
            if task.status in {"succeeded", "failed", "canceled"}:
                return ServiceResult("terminal", serialize_task(task))
            task.cancel_requested = True
            if task.status in {"pending", "retrying"}:
                task.status = "canceled"
                task.finished_at = now
                task.progress = 100
            task.updated_at = now
            uow.operations.audit(
                actor=actor,
                action="task.cancel",
                resource_type="operation_task",
                resource_id=task.id,
                detail={"status": task.status},
            )
            uow.operations.event(
                "task.updated",
                "operation_task",
                task.id,
                {"task_id": task.id, "status": task.status, "cancel_requested": True},
            )
            return ServiceResult("ok", serialize_task(task))

    def retry(self, task_id: str, actor: Actor) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            task = uow.operations.get_task(task_id, for_update=True)
            if not task:
                return ServiceResult("not_found")
            if task.status not in {"failed", "canceled"}:
                return ServiceResult("not_retryable", serialize_task(task))
            task.status = "pending"
            task.progress = 0
            task.retry_count = 0
            task.error_message = None
            task.result_json = None
            task.next_run_at = now
            task.locked_by = None
            task.lease_expires_at = None
            task.last_heartbeat_at = None
            task.cancel_requested = False
            task.started_at = None
            task.finished_at = None
            task.updated_at = now
            uow.operations.audit(
                actor=actor,
                action="task.retry",
                resource_type="operation_task",
                resource_id=task.id,
            )
            uow.operations.event(
                "task.updated",
                "operation_task",
                task.id,
                {"task_id": task.id, "status": "pending"},
            )
            return ServiceResult("ok", serialize_task(task))

    def claim(self, *, worker_id: str, lease_seconds: int) -> Optional[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            expired = uow.operations.recover_expired_task_leases(now)
            for task in expired:
                task.locked_by = None
                task.lease_expires_at = None
                task.last_heartbeat_at = None
                task.retry_count = int(task.retry_count or 0) + 1
                if task.cancel_requested:
                    task.status = "canceled"
                    task.finished_at = now
                elif task.retry_count <= int(task.max_retries or 0):
                    task.status = "retrying"
                    task.next_run_at = now + timedelta(
                        seconds=min(300, 5 * (2 ** max(0, task.retry_count - 1)))
                    )
                    task.error_message = "Worker lease expired; task scheduled for retry"
                else:
                    task.status = "failed"
                    task.finished_at = now
                    task.error_message = "Worker lease expired and retry limit was reached"
                task.updated_at = now
                uow.operations.event(
                    "task.updated",
                    "operation_task",
                    task.id,
                    {"task_id": task.id, "status": task.status},
                )
            row = uow.operations.claim_next_task(
                worker_id=worker_id,
                now=now,
                lease_expires_at=now + timedelta(seconds=lease_seconds),
            )
            if not row:
                return None
            uow.operations.event(
                "task.updated",
                "operation_task",
                row.id,
                {"task_id": row.id, "status": "running"},
            )
            return serialize_task(row, include_sensitive=True)

    def heartbeat(self, task_id: str, worker_id: str, lease_seconds: int) -> bool:
        now = utcnow()
        with self._uow_factory() as uow:
            task = uow.operations.get_task(task_id, for_update=True)
            if not task or task.status != "running" or task.locked_by != worker_id:
                return False
            task.last_heartbeat_at = now
            task.lease_expires_at = now + timedelta(seconds=lease_seconds)
            task.updated_at = now
            return not task.cancel_requested

    def complete(self, task_id: str, worker_id: str, result: Any) -> bool:
        now = utcnow()
        with self._uow_factory() as uow:
            task = uow.operations.get_task(task_id, for_update=True)
            if not task or task.status != "running" or task.locked_by != worker_id:
                return False
            task.status = "canceled" if task.cancel_requested else "succeeded"
            task.progress = 100
            task.result_json = json.dumps(result or {}, ensure_ascii=False, default=str)
            task.error_message = None
            task.locked_by = None
            task.lease_expires_at = None
            task.finished_at = now
            task.updated_at = now
            uow.operations.event(
                "task.updated",
                "operation_task",
                task.id,
                {"task_id": task.id, "task_type": task.task_type, "status": task.status},
            )
            return True

    def fail(self, task_id: str, worker_id: str, error: str) -> bool:
        now = utcnow()
        with self._uow_factory() as uow:
            task = uow.operations.get_task(task_id, for_update=True)
            if not task or task.status != "running" or task.locked_by != worker_id:
                return False
            task.retry_count = int(task.retry_count or 0) + 1
            task.error_message = error[:4000]
            task.locked_by = None
            task.lease_expires_at = None
            task.last_heartbeat_at = None
            if task.cancel_requested:
                task.status = "canceled"
                task.finished_at = now
                task.progress = 100
            elif task.retry_count <= int(task.max_retries or 0):
                task.status = "retrying"
                task.next_run_at = now + timedelta(
                    seconds=min(300, 5 * (2 ** max(0, task.retry_count - 1)))
                )
            else:
                task.status = "failed"
                task.finished_at = now
            task.updated_at = now
            uow.operations.event(
                "task.updated",
                "operation_task",
                task.id,
                {
                    "task_id": task.id,
                    "task_type": task.task_type,
                    "status": task.status,
                    "retry_count": task.retry_count,
                },
            )
            return True

    def record_job_run(
        self,
        *,
        job_name: str,
        trigger_kind: str,
        status: str,
        started_at,
        summary: Optional[dict] = None,
        error_message: Optional[str] = None,
    ) -> None:
        with self._uow_factory() as uow:
            uow.operations.add_job_run(
                JobRun(
                    job_name=job_name,
                    trigger_kind=trigger_kind,
                    status=status,
                    summary_json=json.dumps(summary or {}, ensure_ascii=False, default=str),
                    error_message=(error_message or "")[:4000] or None,
                    started_at=started_at or utcnow(),
                    finished_at=utcnow(),
                )
            )

    def list_job_runs(self, limit: int = 50, offset: int = 0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.operations.list_job_runs(limit, offset)
            return {
                "items": [
                    {
                        "id": row.id,
                        "job_name": row.job_name,
                        "trigger_kind": row.trigger_kind,
                        "status": row.status,
                        "summary": _loads(row.summary_json),
                        "error_message": row.error_message,
                        "started_at": row.started_at,
                        "finished_at": row.finished_at,
                    }
                    for row in rows
                ],
                "total": total,
                "limit": limit,
                "offset": offset,
            }
