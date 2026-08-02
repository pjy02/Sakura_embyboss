import asyncio
import json
from typing import Any, Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import current_identity, require_permission
from bot.application import ReliabilityService, TaskService
from bot.application.auth_service import WebIdentity
from bot.application.task_service import TASK_DEFINITIONS
from bot.domain import Actor, secret_fingerprint


admin_router = APIRouter(prefix="/admin", tags=["background-tasks"])
events_router = APIRouter(prefix="/events", tags=["realtime-events"])
tasks = TaskService()
reliability = ReliabilityService()

ADMIN_EVENT_PERMISSIONS = (
    ("user", "users:read"),
    ("points", "users:read"),
    ("code", "codes:read"),
    ("partition", "partitions:read"),
    ("task", "tasks:read"),
    ("playback", "playback:read"),
    ("device", "devices:read"),
    ("line", "lines:read"),
    ("billing", "billing:read"),
    ("ticket", "tickets:read"),
    ("request", "requests:read"),
    ("review", "reviews:read"),
    ("notification", "notifications:read"),
    ("audit", "audit:read"),
    ("security", "security:read"),
    ("role", "roles:read"),
    ("setting", "settings:read"),
)


class EnqueueTaskRequest(BaseModel):
    task_type: str = Field(min_length=3, max_length=100)
    payload: dict[str, Any] = Field(default_factory=dict)
    confirm: bool = False


def _not_found():
    raise HTTPException(status_code=404, detail="任务不存在")


@admin_router.get("/task-definitions")
async def task_definitions(
    _identity: WebIdentity = Depends(
        require_permission("tasks:read", telegram_only=True)
    ),
):
    return {"items": tasks.definitions()}


@admin_router.get("/tasks")
async def list_tasks(
    status: Optional[str] = Query(None, max_length=255),
    task_type: Optional[str] = Query(None, max_length=100),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("tasks:read", telegram_only=True)
    ),
):
    statuses = [item for item in (status or "").split(",") if item] or None
    allowed = {"pending", "retrying", "running", "succeeded", "failed", "canceled"}
    if statuses and any(item not in allowed for item in statuses):
        raise HTTPException(status_code=400, detail="任务状态筛选无效")
    return await run_in_threadpool(
        tasks.list,
        statuses=statuses,
        task_type=task_type,
        limit=limit,
        offset=offset,
    )


@admin_router.get("/tasks/{task_id}")
async def get_task(
    task_id: str,
    _identity: WebIdentity = Depends(
        require_permission("tasks:read", telegram_only=True)
    ),
):
    result = await run_in_threadpool(tasks.get, task_id)
    if not result:
        _not_found()
    return result


@admin_router.post("/tasks", status_code=202)
async def enqueue_task(
    payload: EnqueueTaskRequest,
    idempotency_key: str = Header(
        ...,
        alias="Idempotency-Key",
        min_length=8,
        max_length=128,
    ),
    identity: WebIdentity = Depends(
        require_permission("tasks:update", csrf=True, telegram_only=True)
    ),
):
    definition = TASK_DEFINITIONS.get(payload.task_type)
    if not definition or not definition.admin_exposed:
        raise HTTPException(status_code=400, detail="不支持此任务类型")
    if definition.risk in {"warning", "danger"} and not payload.confirm:
        raise HTTPException(status_code=409, detail="此任务需要明确确认后才能执行")
    result = await run_in_threadpool(
        tasks.enqueue,
        task_type=payload.task_type,
        payload=payload.payload,
        actor=Actor.web(identity.tg),
        idempotency_key=(
            "task:"
            + secret_fingerprint(
                f"{identity.tg}:{payload.task_type}:{idempotency_key}"
            )
        ),
    )
    if not result.ok:
        raise HTTPException(status_code=400, detail="任务创建失败")
    return result.data


@admin_router.post("/tasks/{task_id}/cancel")
async def cancel_task(
    task_id: str,
    identity: WebIdentity = Depends(
        require_permission("tasks:update", csrf=True, telegram_only=True)
    ),
):
    result = await run_in_threadpool(tasks.cancel, task_id, Actor.web(identity.tg))
    if result.status == "not_found":
        _not_found()
    if result.status == "terminal":
        raise HTTPException(status_code=409, detail="已结束的任务不能取消")
    return result.data


@admin_router.post("/tasks/{task_id}/retry")
async def retry_task(
    task_id: str,
    identity: WebIdentity = Depends(
        require_permission("tasks:update", csrf=True, telegram_only=True)
    ),
):
    result = await run_in_threadpool(tasks.retry, task_id, Actor.web(identity.tg))
    if result.status == "not_found":
        _not_found()
    if result.status == "not_retryable":
        raise HTTPException(status_code=409, detail="当前状态不能重跑")
    return result.data


@admin_router.get("/jobs")
async def job_runs(
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("tasks:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(tasks.list_job_runs, limit, offset)


@admin_router.get("/system/status")
async def system_status(
    _identity: WebIdentity = Depends(
        require_permission("tasks:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(reliability.status)


def _event_message(event: dict) -> str:
    return (
        f"id: {event['id']}\n"
        f"event: {event['event_type']}\n"
        f"data: {json.dumps(event, ensure_ascii=False, default=str)}\n\n"
    )


async def _stream_events(
    request: Request,
    *,
    after_id: int,
    user_tg: Optional[int],
    event_prefixes: Optional[tuple[str, ...]] = None,
):
    relay = request.app.state.event_relay
    relay_version = relay.version
    last_heartbeat = asyncio.get_running_loop().time()
    yield "retry: 3000\n\n"
    while not await request.is_disconnected():
        events = await asyncio.to_thread(
            reliability.events_after,
            after_id=after_id,
            limit=100,
            user_tg=user_tg,
            event_prefixes=event_prefixes,
        )
        if events:
            for event in events:
                after_id = max(after_id, int(event["id"]))
                yield _event_message(event)
            last_heartbeat = asyncio.get_running_loop().time()
            continue

        now = asyncio.get_running_loop().time()
        if now - last_heartbeat >= 15:
            yield ": heartbeat\n\n"
            last_heartbeat = now
        relay_version = await relay.wait_for_change(relay_version, timeout=5.0)


def _after_event_id(request: Request, after: int) -> int:
    header = request.headers.get("Last-Event-ID", "")
    if header.isdigit():
        return max(after, int(header))
    return after


def _admin_event_prefixes(identity: WebIdentity) -> tuple[str, ...]:
    return tuple(
        prefix
        for prefix, permission in ADMIN_EVENT_PERMISSIONS
        if identity.has_permission(permission)
    )


@events_router.get("/stream")
async def user_event_stream(
    request: Request,
    after: int = Query(0, ge=0),
    replay: bool = False,
    identity: WebIdentity = Depends(current_identity),
):
    cursor = _after_event_id(request, after)
    if cursor == 0 and not replay:
        cursor = await run_in_threadpool(reliability.latest_event_id, identity.tg)
    return StreamingResponse(
        _stream_events(
            request,
            after_id=cursor,
            user_tg=identity.tg,
        ),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache, no-transform",
            "X-Accel-Buffering": "no",
            "Connection": "keep-alive",
        },
    )


@admin_router.get("/events/stream")
async def admin_event_stream(
    request: Request,
    after: int = Query(0, ge=0),
    replay: bool = False,
    identity: WebIdentity = Depends(current_identity),
):
    if identity.auth_method != "telegram":
        raise HTTPException(
            status_code=403,
            detail="管理实时事件必须通过 Telegram 确认登录",
        )
    event_prefixes = _admin_event_prefixes(identity)
    if not event_prefixes:
        raise HTTPException(status_code=403, detail="没有可订阅的后台事件权限")
    cursor = _after_event_id(request, after)
    if cursor == 0 and not replay:
        cursor = await run_in_threadpool(
            reliability.latest_event_id,
            event_prefixes=event_prefixes,
        )
    return StreamingResponse(
        _stream_events(
            request,
            after_id=cursor,
            user_tg=None,
            event_prefixes=event_prefixes,
        ),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache, no-transform",
            "X-Accel-Buffering": "no",
            "Connection": "keep-alive",
        },
    )
