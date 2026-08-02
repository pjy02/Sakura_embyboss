from datetime import datetime, timedelta
from typing import Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import current_identity, require_permission
from bot.application import AccountService, CodeService, DynamicSettingsService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor


admin_router = APIRouter(prefix="/admin", tags=["accounts-and-memberships"])
me_router = APIRouter(prefix="/me", tags=["canonical-account"])
accounts = AccountService()
codes = CodeService()
settings_service = DynamicSettingsService()


class PlanPayload(BaseModel):
    code: str = Field(min_length=2, max_length=64, pattern=r"^[a-z][a-z0-9_-]+$")
    name: str = Field(min_length=2, max_length=100)
    description: Optional[str] = Field(default=None, max_length=1000)
    duration_days: int = Field(default=30, ge=0, le=3650)
    legacy_level: str = Field(default="b", pattern=r"^[abcd]$")
    entitlements: dict = Field(default_factory=dict)
    enabled: bool = True
    is_default: bool = False
    sort_order: int = Field(default=0, ge=-10000, le=10000)


class PlanUpdatePayload(BaseModel):
    revision: int = Field(ge=1)
    name: Optional[str] = Field(default=None, min_length=2, max_length=100)
    description: Optional[str] = Field(default=None, max_length=1000)
    duration_days: Optional[int] = Field(default=None, ge=0, le=3650)
    legacy_level: Optional[str] = Field(default=None, pattern=r"^[abcd]$")
    entitlements: Optional[dict] = None
    enabled: Optional[bool] = None
    is_default: Optional[bool] = None
    sort_order: Optional[int] = Field(default=None, ge=-10000, le=10000)


class AssignPlanPayload(BaseModel):
    plan_id: int = Field(gt=0)
    duration_days: Optional[int] = Field(default=None, ge=0, le=3650)


class TagPayload(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    color: str = Field(default="#8b7cf6", pattern=r"^#[0-9a-fA-F]{6}$")
    description: Optional[str] = Field(default=None, max_length=500)


class TagAssignmentPayload(BaseModel):
    account_ids: list[str] = Field(min_length=1, max_length=500)
    tag_ids: list[int] = Field(default_factory=list, max_length=100)
    mode: str = Field(pattern=r"^(add|remove|replace)$")


class GenerateCodesPayload(BaseModel):
    kind: str = Field(pattern=r"^(registration|renewal|whitelist)$")
    days: int = Field(default=30, ge=0, le=3650)
    count: int = Field(default=1, ge=1, le=500)
    expires_at: Optional[datetime] = None


@me_router.get("/account")
async def my_canonical_account(identity: WebIdentity = Depends(current_identity)):
    result = await run_in_threadpool(accounts.get, identity.account_id)
    if not result:
        raise HTTPException(status_code=404, detail="账号不存在")
    return result


@me_router.get("/account/ledger")
async def my_account_ledger(
    limit: int = Query(default=20, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    result = await run_in_threadpool(
        accounts.ledger,
        identity.account_id,
        limit=limit,
        offset=offset,
    )
    if result is None:
        raise HTTPException(status_code=404, detail="账号不存在")
    return result


@admin_router.get("/accounts")
async def list_accounts(
    search: Optional[str] = Query(default=None, max_length=255),
    status: Optional[str] = Query(default=None),
    tag_id: Optional[int] = Query(default=None, gt=0),
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    _identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True)),
):
    return await run_in_threadpool(accounts.list_accounts, search=search, status=status, tag_id=tag_id, limit=limit, offset=offset)


@admin_router.get("/accounts/{account_id}")
async def account_detail(account_id: str, _identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True))):
    result = await run_in_threadpool(accounts.get, account_id)
    if not result:
        raise HTTPException(status_code=404, detail="账号不存在")
    return result


@admin_router.get("/accounts/{account_id}/ledger")
async def account_ledger(
    account_id: str,
    limit: int = Query(default=100, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    _identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True)),
):
    result = await run_in_threadpool(
        accounts.ledger,
        account_id,
        limit=limit,
        offset=offset,
    )
    if result is None:
        raise HTTPException(status_code=404, detail="账号不存在")
    return result


@admin_router.get("/membership-plans")
async def list_membership_plans(_identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True))):
    return await run_in_threadpool(accounts.list_plans)


@admin_router.post("/membership-plans", status_code=201)
async def create_membership_plan(payload: PlanPayload, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(accounts.create_plan, payload.model_dump(), Actor(kind="account", identifier=identity.account_id))
    except Exception as error:
        raise HTTPException(status_code=409, detail="方案代码已存在或数据无效") from error


@admin_router.patch("/membership-plans/{plan_id}")
async def update_membership_plan(plan_id: int, payload: PlanUpdatePayload, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    data = payload.model_dump(exclude_unset=True)
    try:
        result = await run_in_threadpool(accounts.update_plan, plan_id, data, Actor(kind="account", identifier=identity.account_id))
    except RuntimeError as error:
        raise HTTPException(status_code=409, detail=str(error)) from error
    if not result:
        raise HTTPException(status_code=404, detail="会员方案不存在")
    return result


@admin_router.post("/accounts/{account_id}/membership")
async def assign_membership(account_id: str, payload: AssignPlanPayload, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    result = await run_in_threadpool(accounts.assign_plan, account_id=account_id, plan_id=payload.plan_id, duration_days=payload.duration_days, actor=Actor(kind="account", identifier=identity.account_id))
    if not result.ok:
        raise HTTPException(status_code=404, detail="账号或会员方案不存在")
    return result.data


@admin_router.get("/account-tags")
async def list_account_tags(_identity: WebIdentity = Depends(require_permission("users:read", telegram_only=True))):
    return await run_in_threadpool(accounts.list_tags)


@admin_router.post("/account-tags", status_code=201)
async def create_account_tag(payload: TagPayload, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    result = await run_in_threadpool(accounts.create_tag, name=payload.name, color=payload.color, description=payload.description, actor=Actor(kind="account", identifier=identity.account_id))
    if not result.ok:
        raise HTTPException(status_code=409 if result.status == "exists" else 422, detail="标签已存在或格式无效")
    return result.data


@admin_router.delete("/account-tags/{tag_id}", status_code=204)
async def delete_account_tag(tag_id: int, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    deleted = await run_in_threadpool(accounts.delete_tag, tag_id, Actor(kind="account", identifier=identity.account_id))
    if not deleted:
        raise HTTPException(status_code=404, detail="标签不存在")


@admin_router.post("/accounts/tags/batch")
async def batch_assign_tags(payload: TagAssignmentPayload, identity: WebIdentity = Depends(require_permission("users:update", csrf=True, telegram_only=True))):
    return await run_in_threadpool(accounts.assign_tags, account_ids=payload.account_ids, tag_ids=payload.tag_ids, mode=payload.mode, actor=Actor(kind="account", identifier=identity.account_id))


@admin_router.get("/invitation-codes")
async def list_invitation_codes(
    kind: Optional[str] = Query(default=None), status: Optional[str] = Query(default=None), search: Optional[str] = Query(default=None, max_length=255),
    limit: int = Query(default=50, ge=1, le=200), offset: int = Query(default=0, ge=0),
    _identity: WebIdentity = Depends(require_permission("codes:read", telegram_only=True)),
):
    return await run_in_threadpool(codes.list_codes, kind=kind, status=status, search=search, limit=limit, offset=offset)


@admin_router.post("/invitation-codes", status_code=201)
async def generate_invitation_codes(payload: GenerateCodesPayload, identity: WebIdentity = Depends(require_permission("codes:create", csrf=True, telegram_only=True))):
    logo_setting, batch_setting, expiry_setting = await run_in_threadpool(
        lambda: (
            settings_service.get("site.logo"),
            settings_service.get("registration.batch_limit"),
            settings_service.get("registration.invite_expiry_days"),
        )
    )
    if payload.count > int(batch_setting["value"]):
        raise HTTPException(
            status_code=422,
            detail=f"单次最多生成 {batch_setting['value']} 个邀请码",
        )
    expires_at = payload.expires_at or datetime.now() + timedelta(
        days=int(expiry_setting["value"])
    )
    result = await run_in_threadpool(codes.generate, kind=payload.kind, days=payload.days, count=payload.count, logo=str(logo_setting["value"]), issuer_tg=identity.tg, issuer_account_id=identity.account_id, expires_at=expires_at, actor=Actor(kind="account", identifier=identity.account_id))
    if not result.ok:
        raise HTTPException(status_code=422, detail="邀请码参数无效")
    return result.data


@admin_router.post("/invitation-codes/{code_value}/revoke")
async def revoke_invitation_code(code_value: str, identity: WebIdentity = Depends(require_permission("codes:revoke", csrf=True, telegram_only=True))):
    result = await run_in_threadpool(codes.revoke, code_value=code_value, actor=Actor(kind="account", identifier=identity.account_id))
    if not result.ok:
        raise HTTPException(status_code=409 if result.status == "used" else 404, detail="邀请码不存在或已经使用")
    return result.data
