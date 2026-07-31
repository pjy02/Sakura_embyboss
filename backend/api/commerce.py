from typing import Literal, Optional

from fastapi import APIRouter, Depends, Header, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import csrf_protected_identity, current_identity, require_permission
from bot.application import CommerceService, MediaRequestService, TicketService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor, secret_fingerprint


admin_router = APIRouter(prefix="/admin", tags=["commerce-and-support"])
me_router = APIRouter(prefix="/me", tags=["self-service"])
commerce = CommerceService()
tickets = TicketService()
requests = MediaRequestService()


class ProductPayload(BaseModel):
    name: str = Field(min_length=2, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    amount_cents: int = Field(ge=1, le=100_000_000)
    coins: int = Field(ge=1, le=10_000_000)
    bonus_coins: int = Field(0, ge=0, le=10_000_000)
    enabled: bool = True
    sort_order: int = Field(0, ge=0, le=10000)


class ProductUpdatePayload(BaseModel):
    revision: Optional[int] = Field(None, ge=1)
    name: Optional[str] = Field(None, min_length=2, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    amount_cents: Optional[int] = Field(None, ge=1, le=100_000_000)
    coins: Optional[int] = Field(None, ge=1, le=10_000_000)
    bonus_coins: Optional[int] = Field(None, ge=0, le=10_000_000)
    enabled: Optional[bool] = None
    sort_order: Optional[int] = Field(None, ge=0, le=10000)


class CreateOrderPayload(BaseModel):
    product_id: int = Field(ge=1)
    user_note: Optional[str] = Field(None, max_length=500)


class OrderDecisionPayload(BaseModel):
    approve: bool
    payment_reference: Optional[str] = Field(None, max_length=255)
    admin_note: Optional[str] = Field(None, max_length=500)


class CreateTicketPayload(BaseModel):
    subject: str = Field(min_length=3, max_length=200)
    category: Literal["account", "playback", "billing", "request", "technical", "general"] = "general"
    priority: Literal["low", "normal", "high", "urgent"] = "normal"
    body: str = Field(min_length=2, max_length=5000)


class TicketReplyPayload(BaseModel):
    body: str = Field(min_length=1, max_length=5000)
    internal: bool = False


class TicketUpdatePayload(BaseModel):
    status: Optional[Literal["open", "pending_user", "pending_staff", "resolved", "closed"]] = None
    priority: Optional[Literal["low", "normal", "high", "urgent"]] = None
    assignee_tg: Optional[int] = Field(None, ge=1)


class CreateMediaRequestPayload(BaseModel):
    title: str = Field(min_length=1, max_length=255)
    year: Optional[int] = Field(None, ge=1888, le=2200)
    media_type: Literal["movie", "series", "anime", "documentary", "other"] = "other"
    description: Optional[str] = Field(None, max_length=2000)


class MediaRequestUpdatePayload(BaseModel):
    status: Optional[Literal["submitted", "reviewing", "approved", "searching", "downloading", "completed", "rejected", "canceled"]] = None
    priority: Optional[Literal["low", "normal", "high", "urgent"]] = None
    external_ref: Optional[str] = Field(None, max_length=255)
    download_id: Optional[str] = Field(None, max_length=255)
    cost_coins: Optional[int] = Field(None, ge=0, le=10_000_000)
    progress: Optional[int] = Field(None, ge=0, le=100)
    admin_note: Optional[str] = Field(None, max_length=1000)


@me_router.get("/recharge/products")
async def my_recharge_products(_identity: WebIdentity = Depends(current_identity)):
    return await run_in_threadpool(commerce.list_products, enabled_only=True)


@me_router.get("/recharge/orders")
async def my_recharge_orders(
    limit: int = Query(30, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(commerce.list_orders, tg=identity.tg, limit=limit, offset=offset)


@me_router.post("/recharge/orders", status_code=201)
async def create_my_recharge_order(
    payload: CreateOrderPayload,
    idempotency_key: str = Header(..., alias="Idempotency-Key", min_length=8, max_length=128),
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    result = await run_in_threadpool(
        commerce.create_order,
        tg=identity.tg,
        product_id=payload.product_id,
        user_note=payload.user_note,
        actor=Actor.web(identity.tg),
        idempotency_key="web:" + secret_fingerprint(f"{identity.tg}:{idempotency_key}"),
    )
    if result is None:
        raise HTTPException(status_code=404, detail="充值商品不存在或已停用")
    return result


@me_router.post("/recharge/orders/{order_id}/cancel")
async def cancel_my_order(
    order_id: str,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    try:
        result = await run_in_threadpool(commerce.cancel_order, order_id, tg=identity.tg, actor=Actor.web(identity.tg))
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="订单不存在")
    return result


@me_router.get("/billing/ledger")
async def my_billing_ledger(
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(commerce.ledger, tg=identity.tg, limit=limit, offset=offset)


@admin_router.get("/recharge/products")
async def admin_products(_identity: WebIdentity = Depends(require_permission("billing:read", telegram_only=True))):
    return await run_in_threadpool(commerce.list_products, enabled_only=False)


@admin_router.post("/recharge/products", status_code=201)
async def create_product(
    payload: ProductPayload,
    identity: WebIdentity = Depends(require_permission("billing:update", csrf=True, telegram_only=True)),
):
    return await run_in_threadpool(commerce.create_product, payload.model_dump(), actor=Actor.web(identity.tg))


@admin_router.patch("/recharge/products/{product_id}")
async def update_product(
    product_id: int,
    payload: ProductUpdatePayload,
    identity: WebIdentity = Depends(require_permission("billing:update", csrf=True, telegram_only=True)),
):
    try:
        result = await run_in_threadpool(
            commerce.update_product,
            product_id,
            payload.model_dump(exclude_unset=True),
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="充值商品不存在")
    return result


@admin_router.get("/recharge/orders")
async def admin_orders(
    status: Optional[str] = Query(None, max_length=32),
    search: Optional[str] = Query(None, max_length=255),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("billing:read", telegram_only=True)),
):
    return await run_in_threadpool(commerce.list_orders, status=status, search=search, limit=limit, offset=offset)


@admin_router.post("/recharge/orders/{order_id}/decision")
async def decide_order(
    order_id: str,
    payload: OrderDecisionPayload,
    identity: WebIdentity = Depends(require_permission("billing:update", csrf=True, telegram_only=True)),
):
    try:
        result = await run_in_threadpool(
            commerce.decide_order,
            order_id,
            approve=payload.approve,
            payment_reference=payload.payment_reference,
            admin_note=payload.admin_note,
            actor=Actor.web(identity.tg),
        )
    except (RuntimeError, ValueError, OverflowError) as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="订单不存在")
    return result


@admin_router.get("/billing/ledger")
async def admin_ledger(
    tg: Optional[int] = Query(None, ge=1),
    entry_type: Optional[str] = Query(None, max_length=32),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("billing:read", telegram_only=True)),
):
    return await run_in_threadpool(commerce.ledger, tg=tg, entry_type=entry_type, limit=limit, offset=offset)


@me_router.get("/tickets")
async def my_tickets(
    status: Optional[str] = Query(None, max_length=32),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(tickets.list, tg=identity.tg, status=status, limit=limit, offset=offset)


@me_router.post("/tickets", status_code=201)
async def create_my_ticket(
    payload: CreateTicketPayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    return await run_in_threadpool(
        tickets.create,
        tg=identity.tg,
        subject=payload.subject,
        category=payload.category,
        priority=payload.priority,
        body=payload.body,
        actor=Actor.web(identity.tg),
    )


@me_router.get("/tickets/{ticket_id}")
async def my_ticket_detail(
    ticket_id: str,
    identity: WebIdentity = Depends(current_identity),
):
    result = await run_in_threadpool(tickets.detail, ticket_id, tg=identity.tg, include_internal=False)
    if result is None:
        raise HTTPException(status_code=404, detail="工单不存在")
    return result


@me_router.post("/tickets/{ticket_id}/messages")
async def reply_my_ticket(
    ticket_id: str,
    payload: TicketReplyPayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    try:
        result = await run_in_threadpool(
            tickets.reply,
            ticket_id,
            body=payload.body,
            actor=Actor.web(identity.tg),
            tg_scope=identity.tg,
            internal=False,
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="工单不存在")
    return result


@admin_router.get("/tickets")
async def admin_tickets(
    status: Optional[str] = Query(None, max_length=32),
    search: Optional[str] = Query(None, max_length=255),
    assignee_tg: Optional[int] = Query(None, ge=1),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("tickets:read", telegram_only=True)),
):
    return await run_in_threadpool(
        tickets.list,
        status=status,
        search=search,
        assignee_tg=assignee_tg,
        limit=limit,
        offset=offset,
    )


@admin_router.get("/tickets/{ticket_id}")
async def admin_ticket_detail(
    ticket_id: str,
    _identity: WebIdentity = Depends(require_permission("tickets:read", telegram_only=True)),
):
    result = await run_in_threadpool(tickets.detail, ticket_id, include_internal=True)
    if result is None:
        raise HTTPException(status_code=404, detail="工单不存在")
    return result


@admin_router.post("/tickets/{ticket_id}/messages")
async def admin_ticket_reply(
    ticket_id: str,
    payload: TicketReplyPayload,
    identity: WebIdentity = Depends(require_permission("tickets:update", csrf=True, telegram_only=True)),
):
    try:
        result = await run_in_threadpool(
            tickets.reply,
            ticket_id,
            body=payload.body,
            actor=Actor.web(identity.tg),
            tg_scope=None,
            internal=payload.internal,
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="工单不存在")
    return result


@admin_router.patch("/tickets/{ticket_id}")
async def update_ticket(
    ticket_id: str,
    payload: TicketUpdatePayload,
    identity: WebIdentity = Depends(require_permission("tickets:update", csrf=True, telegram_only=True)),
):
    if not payload.model_fields_set:
        raise HTTPException(status_code=400, detail="没有可更新的工单字段")
    result = await run_in_threadpool(
        tickets.update,
        ticket_id,
        status=payload.status,
        priority=payload.priority,
        assignee_tg=payload.assignee_tg,
        assignee_supplied="assignee_tg" in payload.model_fields_set,
        actor=Actor.web(identity.tg),
    )
    if result is None:
        raise HTTPException(status_code=404, detail="工单不存在")
    return result


@me_router.get("/requests")
async def my_media_requests(
    status: Optional[str] = Query(None, max_length=32),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(requests.list, tg=identity.tg, status=status, limit=limit, offset=offset)


@me_router.post("/requests", status_code=201)
async def create_my_media_request(
    payload: CreateMediaRequestPayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    return await run_in_threadpool(
        requests.create,
        tg=identity.tg,
        title=payload.title,
        year=payload.year,
        media_type=payload.media_type,
        description=payload.description,
        actor=Actor.web(identity.tg),
    )


@me_router.post("/requests/{request_id}/cancel")
async def cancel_my_media_request(
    request_id: str,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    try:
        result = await run_in_threadpool(requests.cancel, request_id, tg=identity.tg, actor=Actor.web(identity.tg))
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="求片记录不存在")
    return result


@admin_router.get("/requests")
async def admin_media_requests(
    status: Optional[str] = Query(None, max_length=32),
    search: Optional[str] = Query(None, max_length=255),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("requests:read", telegram_only=True)),
):
    return await run_in_threadpool(requests.list, status=status, search=search, limit=limit, offset=offset)


@admin_router.patch("/requests/{request_id}")
async def update_media_request(
    request_id: str,
    payload: MediaRequestUpdatePayload,
    identity: WebIdentity = Depends(require_permission("requests:update", csrf=True, telegram_only=True)),
):
    if not payload.model_fields_set:
        raise HTTPException(status_code=400, detail="没有可更新的求片字段")
    try:
        result = await run_in_threadpool(
            requests.update,
            request_id,
            data=payload.model_dump(exclude_unset=True),
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="求片记录不存在")
    return result
