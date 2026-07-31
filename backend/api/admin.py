import csv
import io
import json
from datetime import datetime
from typing import Literal, Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.responses import Response
from starlette.concurrency import run_in_threadpool

from backend.dependencies import (
    get_auth_service,
    owner_identity,
    require_permission,
)
from bot.application import AdminQueryService, PointService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor, secret_fingerprint


router = APIRouter(prefix="/admin", tags=["administration"])
queries = AdminQueryService()
points = PointService()


class PointAdjustmentRequest(BaseModel):
    amount: int = Field(ge=-2147483648, le=2147483647)
    balance_type: Literal["coins", "registration_days"] = "coins"
    reason: str = Field(min_length=3, max_length=255)
    allow_negative: bool = False


class RoleAssignmentRequest(BaseModel):
    role: str = Field(min_length=3, max_length=64)
    enabled: bool


class RolePayload(BaseModel):
    name: str = Field(min_length=3, max_length=32)
    permissions: list[str] = Field(default_factory=list, max_length=100)


class RolePermissionsPayload(BaseModel):
    permissions: list[str] = Field(default_factory=list, max_length=100)


@router.get("/overview")
async def overview(
    _identity: WebIdentity = Depends(
        require_permission("users:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(queries.overview)


@router.get("/users")
async def list_users(
    search: Optional[str] = Query(None, max_length=255),
    level: Optional[Literal["a", "b", "c", "d"]] = None,
    sort_by: Literal["tg", "name", "created_at", "expires_at", "coins"] = "tg",
    sort_order: Literal["asc", "desc"] = "desc",
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("users:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(
        queries.list_users,
        search=search,
        level=level,
        sort_by=sort_by,
        sort_order=sort_order,
        limit=limit,
        offset=offset,
    )


@router.get("/users/{tg}")
async def get_user(
    tg: int,
    _identity: WebIdentity = Depends(
        require_permission("users:read", telegram_only=True)
    ),
):
    user = await run_in_threadpool(queries.get_user, tg)
    if not user:
        raise HTTPException(status_code=404, detail="用户不存在")
    user["roles"] = await run_in_threadpool(get_auth_service().roles_for_user, tg)
    return user


@router.post("/users/{tg}/points")
async def adjust_points(
    tg: int,
    payload: PointAdjustmentRequest,
    idempotency_key: str = Header(
        ...,
        alias="Idempotency-Key",
        min_length=8,
        max_length=128,
    ),
    identity: WebIdentity = Depends(
        require_permission("users:update", csrf=True, telegram_only=True)
    ),
):
    result = await run_in_threadpool(
        points.adjust,
        tg=tg,
        amount=payload.amount,
        balance_type=payload.balance_type,
        reason=payload.reason,
        actor=Actor.web(identity.tg),
        allow_negative=payload.allow_negative,
        idempotency_key=(
            "web:"
            + secret_fingerprint(f"{identity.tg}:{idempotency_key}")
        ),
    )
    if result.status == "user_not_found":
        raise HTTPException(status_code=404, detail="用户不存在")
    if result.status == "insufficient_balance":
        raise HTTPException(status_code=409, detail="余额不足")
    if result.status == "overflow":
        raise HTTPException(status_code=409, detail="结果超出安全范围")
    if not result.ok:
        raise HTTPException(status_code=500, detail="调整失败")
    return result.data


@router.get("/audit")
async def audit_logs(
    search: Optional[str] = Query(None, max_length=255),
    actor_kind: Optional[str] = Query(None, max_length=32),
    actor_id: Optional[str] = Query(None, max_length=128),
    action: Optional[str] = Query(None, max_length=100),
    resource_type: Optional[str] = Query(None, max_length=64),
    outcome: Optional[str] = Query(None, max_length=32),
    date_from: Optional[datetime] = None,
    date_to: Optional[datetime] = None,
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(
        require_permission("audit:read", telegram_only=True)
    ),
):
    return await run_in_threadpool(
        queries.audit_logs,
        search=search,
        actor_kind=actor_kind,
        actor_id=actor_id,
        action=action,
        resource_type=resource_type,
        outcome=outcome,
        date_from=date_from,
        date_to=date_to,
        limit=limit,
        offset=offset,
    )


@router.get("/audit/export")
async def export_audit_logs(
    search: Optional[str] = Query(None, max_length=255),
    actor_kind: Optional[str] = Query(None, max_length=32),
    actor_id: Optional[str] = Query(None, max_length=128),
    action: Optional[str] = Query(None, max_length=100),
    resource_type: Optional[str] = Query(None, max_length=64),
    outcome: Optional[str] = Query(None, max_length=32),
    date_from: Optional[datetime] = None,
    date_to: Optional[datetime] = None,
    identity: WebIdentity = Depends(
        require_permission("audit:export", telegram_only=True)
    ),
):
    result = await run_in_threadpool(
        queries.audit_logs,
        search=search,
        actor_kind=actor_kind,
        actor_id=actor_id,
        action=action,
        resource_type=resource_type,
        outcome=outcome,
        date_from=date_from,
        date_to=date_to,
        limit=10000,
        offset=0,
    )
    await run_in_threadpool(
        queries.record_audit_export,
        actor=Actor.web(identity.tg),
        filters={
            "search": search,
            "actor_kind": actor_kind,
            "actor_id": actor_id,
            "action": action,
            "resource_type": resource_type,
            "outcome": outcome,
            "date_from": date_from,
            "date_to": date_to,
        },
        count=len(result["items"]),
    )
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(
        [
            "id",
            "created_at",
            "actor_kind",
            "actor_id",
            "action",
            "resource_type",
            "resource_id",
            "outcome",
            "ip_address",
            "request_id",
            "detail",
        ]
    )
    for item in result["items"]:
        writer.writerow(
            [
                item["id"],
                item["created_at"],
                item["actor_kind"],
                item["actor_id"],
                item["action"],
                item["resource_type"],
                item["resource_id"],
                item["outcome"],
                item["ip_address"],
                item["request_id"],
                json.dumps(item["detail"], ensure_ascii=False, default=str),
            ]
        )
    content = "\ufeff" + output.getvalue()
    return Response(
        content=content.encode("utf-8"),
        media_type="text/csv; charset=utf-8",
        headers={"Content-Disposition": 'attachment; filename="sakura-audit.csv"'},
    )


@router.get("/roles")
async def list_roles(
    _identity: WebIdentity = Depends(
        require_permission("roles:read", telegram_only=True)
    ),
):
    return {"items": await run_in_threadpool(get_auth_service().list_roles)}


@router.get("/roles/catalog")
async def role_permission_catalog(
    _identity: WebIdentity = Depends(
        require_permission("roles:read", telegram_only=True)
    ),
):
    return {"items": get_auth_service().permission_catalog()}


@router.post("/roles", status_code=201)
async def create_role(
    payload: RolePayload,
    identity: WebIdentity = Depends(owner_identity),
):
    result = await run_in_threadpool(
        get_auth_service().create_role,
        name=payload.name,
        permissions=payload.permissions,
        actor_tg=identity.tg,
    )
    if result.status == "invalid_name":
        raise HTTPException(status_code=400, detail="角色名只能使用小写字母、数字、下划线或短横线")
    if result.status == "invalid_permissions":
        raise HTTPException(status_code=400, detail="包含未知权限")
    if result.status == "role_exists":
        raise HTTPException(status_code=409, detail="角色名已存在")
    return result.data


@router.patch("/roles/{role_id}")
async def update_role(
    role_id: int,
    payload: RolePermissionsPayload,
    identity: WebIdentity = Depends(owner_identity),
):
    result = await run_in_threadpool(
        get_auth_service().update_role,
        role_id=role_id,
        permissions=payload.permissions,
        actor_tg=identity.tg,
    )
    if result.status == "role_not_found":
        raise HTTPException(status_code=404, detail="角色不存在")
    if result.status == "protected_role":
        raise HTTPException(status_code=409, detail="该系统角色不能修改")
    if result.status == "invalid_permissions":
        raise HTTPException(status_code=400, detail="包含未知权限")
    return result.data


@router.delete("/roles/{role_id}", status_code=204)
async def delete_role(
    role_id: int,
    identity: WebIdentity = Depends(owner_identity),
):
    result = await run_in_threadpool(
        get_auth_service().delete_role,
        role_id=role_id,
        actor_tg=identity.tg,
    )
    if result.status == "role_not_found":
        raise HTTPException(status_code=404, detail="角色不存在")
    if result.status == "protected_role":
        raise HTTPException(status_code=409, detail="系统角色不能删除")
    if result.status == "role_in_use":
        raise HTTPException(
            status_code=409,
            detail=f"仍有 {result.data['member_count']} 名用户使用该角色",
        )


@router.put("/users/{tg}/role")
async def assign_role(
    tg: int,
    payload: RoleAssignmentRequest,
    identity: WebIdentity = Depends(owner_identity),
):
    result = await run_in_threadpool(
        get_auth_service().set_role,
        target_tg=tg,
        role_name=payload.role,
        enabled=payload.enabled,
        actor_tg=identity.tg,
    )
    if result.status == "user_not_found":
        raise HTTPException(status_code=404, detail="用户不存在")
    if result.status == "invalid_role":
        raise HTTPException(status_code=400, detail="角色不存在或不可分配")
    return result.data
