from typing import Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import require_permission
from bot.application import CoreOperationsService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor


router = APIRouter(prefix="/admin", tags=["core-operations"])
operations = CoreOperationsService()


class StopPlaybackRequest(BaseModel):
    reason: str = Field(min_length=3, max_length=255)


class DeviceUpdateRequest(BaseModel):
    trusted: Optional[bool] = None
    banned: Optional[bool] = None
    notes: Optional[str] = Field(None, max_length=1000)


class LineCreateRequest(BaseModel):
    name: str = Field(min_length=2, max_length=100)
    base_url: str = Field(min_length=8, max_length=512)
    region: Optional[str] = Field(None, max_length=100)
    carrier: Optional[str] = Field(None, max_length=100)
    audience: Literal["all", "whitelist"] = "all"
    weight: int = Field(100, ge=0, le=1000)
    sort_order: int = Field(0, ge=0, le=10000)
    enabled: bool = True
    maintenance: bool = False


class LineUpdateRequest(BaseModel):
    revision: Optional[int] = Field(None, ge=1)
    name: Optional[str] = Field(None, min_length=2, max_length=100)
    base_url: Optional[str] = Field(None, min_length=8, max_length=512)
    region: Optional[str] = Field(None, max_length=100)
    carrier: Optional[str] = Field(None, max_length=100)
    audience: Optional[Literal["all", "whitelist"]] = None
    weight: Optional[int] = Field(None, ge=0, le=1000)
    sort_order: Optional[int] = Field(None, ge=0, le=10000)
    enabled: Optional[bool] = None
    maintenance: Optional[bool] = None


@router.get("/dashboard/core")
async def core_dashboard(
    _identity: WebIdentity = Depends(
        require_permission("dashboard:read", telegram_only=True)
    ),
):
    sync = await operations.sync_live_sessions()
    result = await run_in_threadpool(operations.dashboard)
    result["emby_status"] = sync["source"]
    result["emby_error"] = sync["error"]
    if sync["source"] == "emby":
        result["live_sessions"] = sync["total"]
    return result


@router.get("/playback/live")
async def live_playback(
    _identity: WebIdentity = Depends(
        require_permission("playback:read", telegram_only=True)
    ),
):
    return await operations.sync_live_sessions()


@router.get("/playback/history")
async def playback_history(
    search: Optional[str] = Query(None, max_length=255),
    active_only: bool = False,
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("playback:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(
        operations.list_playback,
        search=search,
        active_only=active_only,
        limit=limit,
        offset=offset,
    )


@router.post("/playback/{session_id}/stop")
async def stop_playback(
    session_id: str,
    payload: StopPlaybackRequest,
    identity: WebIdentity = Depends(
        require_permission("playback:stop", csrf=True, telegram_only=True)
    ),
):
    success = await operations.stop_playback(
        session_id,
        reason=payload.reason,
        actor=Actor.web(identity.tg),
    )
    if not success:
        raise HTTPException(status_code=502, detail="Emby 未能终止该播放会话")
    return {"stopped": True, "session_id": session_id}


@router.get("/devices")
async def list_devices(
    search: Optional[str] = Query(None, max_length=255),
    risk_level: Optional[str] = Query(None, pattern="^(normal|warning|high)$"),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("devices:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(
        operations.list_devices,
        search=search,
        risk_level=risk_level,
        limit=limit,
        offset=offset,
    )


@router.patch("/devices/{device_key:path}")
async def update_device(
    device_key: str,
    payload: DeviceUpdateRequest,
    identity: WebIdentity = Depends(
        require_permission("devices:update", csrf=True, telegram_only=True)
    ),
):
    if not payload.model_fields_set:
        raise HTTPException(status_code=400, detail="没有可更新的设备字段")
    result = await run_in_threadpool(
        operations.update_device,
        device_key,
        trusted=payload.trusted,
        banned=payload.banned,
        notes=payload.notes if "notes" in payload.model_fields_set else None,
        actor=Actor.web(identity.tg),
    )
    if result is None:
        raise HTTPException(status_code=404, detail="设备不存在")
    return result


@router.get("/lines")
async def list_lines(
    _identity: WebIdentity = Depends(
        require_permission("lines:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(operations.list_lines)


@router.post("/lines", status_code=201)
async def create_line(
    payload: LineCreateRequest,
    identity: WebIdentity = Depends(
        require_permission("lines:update", csrf=True, telegram_only=True)
    ),
):
    try:
        return await run_in_threadpool(
            operations.create_line,
            payload.model_dump(),
            actor=Actor.web(identity.tg),
        )
    except ValueError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@router.patch("/lines/{line_id}")
async def update_line(
    line_id: int,
    payload: LineUpdateRequest,
    identity: WebIdentity = Depends(
        require_permission("lines:update", csrf=True, telegram_only=True)
    ),
):
    if not payload.model_fields_set:
        raise HTTPException(status_code=400, detail="没有可更新的线路字段")
    try:
        result = await run_in_threadpool(
            operations.update_line,
            line_id,
            payload.model_dump(exclude_unset=True),
            actor=Actor.web(identity.tg),
        )
    except ValueError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="线路不存在")
    return result


@router.post("/lines/{line_id}/probe")
async def probe_line(
    line_id: int,
    identity: WebIdentity = Depends(
        require_permission("lines:update", csrf=True, telegram_only=True)
    ),
):
    result = await operations.probe_line(line_id, actor=Actor.web(identity.tg))
    if result is None:
        raise HTTPException(status_code=404, detail="线路不存在")
    return result


@router.get("/lines/{line_id}/health")
async def line_health(
    line_id: int,
    limit: int = Query(30, ge=1, le=100),
    _identity: WebIdentity = Depends(
        require_permission("lines:read", telegram_only=True)
    ),
):
    result = await run_in_threadpool(operations.line_health, line_id, limit)
    if result is None:
        raise HTTPException(status_code=404, detail="线路不存在")
    return result
