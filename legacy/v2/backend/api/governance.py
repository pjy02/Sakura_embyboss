from typing import Any, Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import require_permission
from bot.application import DynamicSettingsService, RiskEventService
from bot.application.auth_service import WebIdentity
from bot.application.governance_service import (
    InvalidSettingValue,
    SettingConflictError,
    UnknownSettingError,
)
from bot.domain import Actor


router = APIRouter(prefix="/admin", tags=["governance"])
risk_events = RiskEventService()
dynamic_settings = DynamicSettingsService()


class RiskEventUpdate(BaseModel):
    status: Literal["open", "acknowledged", "resolved", "ignored"]
    assigned_to: Optional[int] = None
    resolution_note: Optional[str] = Field(None, max_length=1000)


class SettingUpdate(BaseModel):
    value: Any
    expected_revision: int = Field(ge=0)


class SettingRollback(BaseModel):
    target_revision: int = Field(ge=1)
    expected_revision: int = Field(ge=0)


@router.get("/risk/summary")
async def risk_summary(
    _identity: WebIdentity = Depends(
        require_permission("security:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(risk_events.summary)


@router.get("/risk/events")
async def list_risk_events(
    search: Optional[str] = Query(None, max_length=255),
    severity: Optional[Literal["info", "warning", "danger"]] = None,
    status: Optional[
        Literal["open", "acknowledged", "resolved", "ignored"]
    ] = None,
    event_type: Optional[str] = Query(None, max_length=100),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("security:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(
        risk_events.list,
        search=search,
        severity=severity,
        status=status,
        event_type=event_type,
        limit=limit,
        offset=offset,
    )


@router.get("/risk/events/{event_id}")
async def get_risk_event(
    event_id: int,
    _identity: WebIdentity = Depends(
        require_permission("security:read", telegram_only=True)
    ),
):
    result = await run_in_threadpool(risk_events.get, event_id)
    if result is None:
        raise HTTPException(status_code=404, detail="风险事件不存在")
    return result


@router.patch("/risk/events/{event_id}")
async def update_risk_event(
    event_id: int,
    payload: RiskEventUpdate,
    identity: WebIdentity = Depends(
        require_permission("security:manage", csrf=True, telegram_only=True)
    ),
):
    result = await run_in_threadpool(
        risk_events.update,
        event_id,
        status=payload.status,
        assigned_to=payload.assigned_to,
        resolution_note=payload.resolution_note,
        actor=Actor.web(identity.tg),
    )
    if result is None:
        raise HTTPException(status_code=404, detail="风险事件不存在")
    return result


@router.get("/settings")
async def list_settings(
    _identity: WebIdentity = Depends(
        require_permission("settings:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(dynamic_settings.list)


@router.get("/settings/{setting_key:path}/history")
async def setting_history(
    setting_key: str,
    limit: int = Query(30, ge=1, le=100),
    _identity: WebIdentity = Depends(
        require_permission("settings:read", telegram_only=True)
    ),
):
    try:
        return await run_in_threadpool(
            dynamic_settings.history,
            setting_key,
            limit,
        )
    except UnknownSettingError:
        raise HTTPException(status_code=404, detail="设置项不存在")


@router.patch("/settings/{setting_key:path}")
async def update_setting(
    setting_key: str,
    payload: SettingUpdate,
    identity: WebIdentity = Depends(
        require_permission("settings:manage", csrf=True, telegram_only=True)
    ),
):
    try:
        return await run_in_threadpool(
            dynamic_settings.update,
            setting_key,
            value=payload.value,
            expected_revision=payload.expected_revision,
            actor=Actor.web(identity.tg),
        )
    except UnknownSettingError:
        raise HTTPException(status_code=404, detail="设置项不存在")
    except SettingConflictError as error:
        raise HTTPException(status_code=409, detail=str(error))
    except InvalidSettingValue as error:
        raise HTTPException(status_code=422, detail=str(error))


@router.post("/settings/{setting_key:path}/rollback")
async def rollback_setting(
    setting_key: str,
    payload: SettingRollback,
    identity: WebIdentity = Depends(
        require_permission("settings:manage", csrf=True, telegram_only=True)
    ),
):
    try:
        return await run_in_threadpool(
            dynamic_settings.rollback,
            setting_key,
            target_revision=payload.target_revision,
            expected_revision=payload.expected_revision,
            actor=Actor.web(identity.tg),
        )
    except UnknownSettingError:
        raise HTTPException(status_code=404, detail="设置项或历史版本不存在")
    except SettingConflictError as error:
        raise HTTPException(status_code=409, detail=str(error))
    except InvalidSettingValue as error:
        raise HTTPException(status_code=422, detail=str(error))
