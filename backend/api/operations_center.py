from typing import Any, Literal, Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import require_permission
from bot.application import (
    AccountLifecycleService,
    AlertService,
    DiagnosticService,
    RiskRuleService,
    TaskService,
)
from bot.application.auth_service import WebIdentity
from bot.domain import Actor, secret_fingerprint


router = APIRouter(prefix="/admin", tags=["operations-center"])
risk_rules = RiskRuleService()
diagnostics = DiagnosticService()
alerts = AlertService()
lifecycle = AccountLifecycleService()
tasks = TaskService()


class RiskRulePayload(BaseModel):
    name: str = Field(min_length=2, max_length=100)
    event_pattern: str = Field(min_length=2, max_length=100)
    severity: Literal["info", "warning", "danger"] = "warning"
    threshold_count: int = Field(1, ge=1, le=100_000)
    window_minutes: int = Field(10, ge=1, le=10_080)
    cooldown_minutes: int = Field(30, ge=1, le=43_200)
    enabled: bool = True
    telegram_alert: bool = True


class RiskRuleUpdatePayload(RiskRulePayload):
    expected_revision: int = Field(ge=1)


class BatchOperationPayload(BaseModel):
    action: Literal[
        "suspend",
        "restore",
        "extend",
        "grant_coins",
        "grant_registration_days",
        "notify",
        "clear_account",
    ]
    tg_ids: list[int] = Field(default_factory=list, max_length=500)
    account_ids: list[str] = Field(default_factory=list, max_length=500)
    parameters: dict[str, Any] = Field(default_factory=dict)
    confirm: bool = False


@router.get("/risk/rules")
async def list_risk_rules(
    _identity: WebIdentity = Depends(require_permission("security:read", telegram_only=True)),
):
    return await run_in_threadpool(risk_rules.list)


@router.post("/risk/rules", status_code=201)
async def create_risk_rule(
    payload: RiskRulePayload,
    identity: WebIdentity = Depends(require_permission("security:manage", csrf=True, telegram_only=True)),
):
    try:
        return await run_in_threadpool(risk_rules.create, payload.model_dump(), Actor.web(identity.tg))
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


@router.patch("/risk/rules/{rule_id}")
async def update_risk_rule(
    rule_id: int,
    payload: RiskRuleUpdatePayload,
    identity: WebIdentity = Depends(require_permission("security:manage", csrf=True, telegram_only=True)),
):
    data = payload.model_dump(exclude={"expected_revision"})
    try:
        result = await run_in_threadpool(
            risk_rules.update,
            rule_id,
            data,
            payload.expected_revision,
            Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="风险规则不存在")
    return result


@router.get("/diagnostics")
async def diagnostic_summary(
    history_limit: int = Query(40, ge=10, le=200),
    _identity: WebIdentity = Depends(require_permission("tasks:read", telegram_only=True)),
):
    return await run_in_threadpool(diagnostics.summary, history_limit)


@router.post("/diagnostics/run", status_code=202)
async def run_diagnostics(
    idempotency_key: str = Header(..., alias="Idempotency-Key", min_length=8, max_length=128),
    identity: WebIdentity = Depends(require_permission("tasks:update", csrf=True, telegram_only=True)),
):
    result = await run_in_threadpool(
        tasks.enqueue,
        task_type="monitor.diagnostics",
        payload={"source": "admin"},
        actor=Actor.web(identity.tg),
        idempotency_key="diagnostics:" + secret_fingerprint(f"{identity.tg}:{idempotency_key}"),
    )
    if not result.ok:
        raise HTTPException(status_code=400, detail="诊断任务创建失败")
    return result.data


@router.get("/alerts/deliveries")
async def alert_deliveries(
    status: Optional[Literal["pending", "sent", "failed"]] = None,
    limit: int = Query(100, ge=1, le=200),
    _identity: WebIdentity = Depends(require_permission("security:read", telegram_only=True)),
):
    return await run_in_threadpool(alerts.list, status=status, limit=limit)


@router.post("/operations/batches", status_code=202)
async def enqueue_batch_operation(
    payload: BatchOperationPayload,
    idempotency_key: str = Header(..., alias="Idempotency-Key", min_length=8, max_length=128),
    identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True)),
):
    if not payload.confirm:
        raise HTTPException(status_code=409, detail="批量操作需要明确确认后才能执行")
    result = await run_in_threadpool(
        lifecycle.enqueue_batch,
        action=payload.action,
        tg_ids=payload.tg_ids,
        account_ids=payload.account_ids,
        parameters=payload.parameters,
        actor=Actor.web(identity.tg),
        idempotency_key="users-batch:" + secret_fingerprint(f"{identity.tg}:{payload.action}:{idempotency_key}"),
    )
    if result.status == "invalid_targets":
        raise HTTPException(status_code=422, detail="没有有效的目标用户，或目标数量超过 500")
    if result.status == "invalid_parameters":
        raise HTTPException(status_code=422, detail="批量操作参数无效")
    if not result.ok:
        raise HTTPException(status_code=400, detail="批量任务创建失败")
    return result.data


@router.get("/operations/lifecycle")
async def lifecycle_history(
    tg: Optional[int] = Query(None, ge=1),
    account_id: Optional[str] = Query(None, min_length=36, max_length=36),
    limit: int = Query(100, ge=1, le=200),
    _identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True)),
):
    return await run_in_threadpool(
        lifecycle.history,
        tg=tg,
        account_id=account_id,
        limit=limit,
    )
