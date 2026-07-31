from __future__ import annotations

import json
from datetime import datetime
from typing import Optional
from uuid import uuid4

from bot.application.community_service import add_notification
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import utcnow
from bot.sql_helper.sql_commerce import (
    BillingEntry,
    MediaRequest,
    RechargeOrder,
    RechargeProduct,
    SupportTicket,
    TicketMessage,
)


MAX_INT_VALUE = 2147483647


def _json(value) -> Optional[str]:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str) if value is not None else None


def serialize_product(row: RechargeProduct) -> dict:
    return {
        "id": row.id,
        "name": row.name,
        "description": row.description,
        "amount_cents": row.amount_cents,
        "coins": row.coins,
        "bonus_coins": row.bonus_coins,
        "enabled": bool(row.enabled),
        "sort_order": row.sort_order,
        "revision": row.revision,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


def serialize_order(row: RechargeOrder) -> dict:
    return {
        "id": row.id,
        "order_no": row.order_no,
        "tg": row.tg,
        "product_id": row.product_id,
        "product_name": row.product_name,
        "amount_cents": row.amount_cents,
        "coins": row.coins,
        "bonus_coins": row.bonus_coins,
        "payment_method": row.payment_method,
        "payment_reference": row.payment_reference,
        "status": row.status,
        "user_note": row.user_note,
        "admin_note": row.admin_note,
        "created_at": row.created_at,
        "paid_at": row.paid_at,
        "credited_at": row.credited_at,
        "canceled_at": row.canceled_at,
        "updated_at": row.updated_at,
    }


def serialize_billing(row: BillingEntry) -> dict:
    metadata = None
    if row.metadata_json:
        try:
            metadata = json.loads(row.metadata_json)
        except (TypeError, ValueError):
            pass
    return {
        "id": row.id,
        "order_id": row.order_id,
        "tg": row.tg,
        "entry_type": row.entry_type,
        "amount_cents": row.amount_cents,
        "coins": row.coins,
        "description": row.description,
        "actor_kind": row.actor_kind,
        "actor_id": row.actor_id,
        "metadata": metadata,
        "created_at": row.created_at,
    }


def serialize_ticket(row: SupportTicket) -> dict:
    return {
        "id": row.id,
        "ticket_no": row.ticket_no,
        "tg": row.tg,
        "subject": row.subject,
        "category": row.category,
        "priority": row.priority,
        "status": row.status,
        "assignee_tg": row.assignee_tg,
        "last_reply_kind": row.last_reply_kind,
        "last_reply_at": row.last_reply_at,
        "resolved_at": row.resolved_at,
        "closed_at": row.closed_at,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


def serialize_message(row: TicketMessage) -> dict:
    return {
        "id": row.id,
        "ticket_id": row.ticket_id,
        "sender_kind": row.sender_kind,
        "sender_tg": row.sender_tg,
        "body": row.body,
        "internal": bool(row.internal),
        "created_at": row.created_at,
    }


def serialize_media_request(row: MediaRequest) -> dict:
    return {
        "id": row.id,
        "request_no": row.request_no,
        "tg": row.tg,
        "title": row.title,
        "year": row.year,
        "media_type": row.media_type,
        "description": row.description,
        "status": row.status,
        "priority": row.priority,
        "source": row.source,
        "external_ref": row.external_ref,
        "download_id": row.download_id,
        "cost_coins": row.cost_coins,
        "progress": row.progress,
        "admin_note": row.admin_note,
        "reviewed_by": row.reviewed_by,
        "reviewed_at": row.reviewed_at,
        "completed_at": row.completed_at,
        "canceled_at": row.canceled_at,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


class CommerceService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list_products(self, *, enabled_only: bool) -> dict:
        with self._uow_factory() as uow:
            rows = uow.commerce.list_products(enabled_only=enabled_only)
            return {"items": [serialize_product(row) for row in rows], "total": len(rows)}

    def create_product(self, data: dict, *, actor: Actor) -> dict:
        with self._uow_factory() as uow:
            row = RechargeProduct(**data)
            uow.commerce.add_product(row)
            uow.flush()
            uow.operations.audit(
                actor=actor,
                action="billing.product.create",
                resource_type="recharge_product",
                resource_id=str(row.id),
                detail={"name": row.name, "amount_cents": row.amount_cents, "coins": row.coins},
            )
            uow.operations.event("billing.product.created", "recharge_product", str(row.id), {"name": row.name})
            return serialize_product(row)

    def update_product(self, product_id: int, data: dict, *, actor: Actor) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.commerce.get_product(product_id, for_update=True)
            if row is None:
                return None
            revision = data.pop("revision", None)
            if revision is not None and revision != row.revision:
                raise RuntimeError("商品已被其他管理员修改，请刷新后重试")
            for field in ("name", "description", "amount_cents", "coins", "bonus_coins", "enabled", "sort_order"):
                if field in data:
                    setattr(row, field, data[field])
            row.revision += 1
            row.updated_at = utcnow()
            uow.operations.audit(
                actor=actor,
                action="billing.product.update",
                resource_type="recharge_product",
                resource_id=str(product_id),
                detail=data,
            )
            uow.operations.event("billing.product.updated", "recharge_product", str(product_id), data)
            uow.flush()
            return serialize_product(row)

    def create_order(
        self,
        *,
        tg: int,
        product_id: int,
        user_note: Optional[str],
        actor: Actor,
        idempotency_key: Optional[str],
    ) -> Optional[dict]:
        now = utcnow()
        order_id = str(uuid4())
        with self._uow_factory() as uow:
            replay = uow.operations.get_idempotent_result("billing.order.create", idempotency_key)
            if replay:
                return replay
            product = uow.commerce.get_product(product_id)
            if product is None or not product.enabled:
                return None
            row = RechargeOrder(
                id=order_id,
                order_no=f"RC{now.strftime('%y%m%d%H%M')}{order_id[:6].upper()}",
                tg=tg,
                product_id=product.id,
                product_name=product.name,
                amount_cents=product.amount_cents,
                coins=product.coins,
                bonus_coins=product.bonus_coins,
                user_note=(user_note or "").strip() or None,
                created_at=now,
                updated_at=now,
            )
            uow.commerce.add_order(row)
            uow.commerce.add_billing_entry(
                BillingEntry(
                    order_id=row.id,
                    tg=tg,
                    entry_type="order_created",
                    amount_cents=row.amount_cents,
                    coins=row.coins + row.bonus_coins,
                    description=f"创建充值订单 {row.order_no}",
                    actor_kind=actor.kind,
                    actor_id=actor.identifier,
                )
            )
            uow.operations.audit(
                actor=actor,
                action="billing.order.create",
                resource_type="recharge_order",
                resource_id=row.id,
                detail={"order_no": row.order_no, "amount_cents": row.amount_cents},
            )
            uow.operations.event(
                "billing.order.created",
                "user",
                str(tg),
                {"resource_type": "recharge_order", "resource_id": row.id, "tg": tg, "order_no": row.order_no},
            )
            uow.flush()
            result = serialize_order(row)
            uow.operations.save_idempotent_result("billing.order.create", idempotency_key, result)
            return result

    def list_orders(self, *, tg=None, status=None, search=None, limit=50, offset=0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.commerce.list_orders(tg=tg, status=status, search=search, limit=limit, offset=offset)
            return {"items": [serialize_order(row) for row in rows], "total": total, "limit": limit, "offset": offset}

    def decide_order(
        self,
        order_id: str,
        *,
        approve: bool,
        payment_reference: Optional[str],
        admin_note: Optional[str],
        actor: Actor,
    ) -> Optional[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.commerce.get_order(order_id, for_update=True)
            if row is None:
                return None
            if row.status == "credited":
                return serialize_order(row)
            if row.status != "pending":
                raise RuntimeError("订单当前状态不允许审核")
            row.admin_note = (admin_note or "").strip() or None
            if not approve:
                row.status = "canceled"
                row.canceled_at = now
                row.updated_at = now
                entry_type = "order_rejected"
                description = f"充值订单 {row.order_no} 未通过"
            else:
                user = uow.users.get_for_update(row.tg)
                if user is None:
                    raise ValueError("订单用户不存在")
                credit = int(row.coins or 0) + int(row.bonus_coins or 0)
                old_balance = int(user.iv or 0)
                if old_balance + credit > MAX_INT_VALUE:
                    raise OverflowError("入账后积分超出安全范围")
                user.iv = old_balance + credit
                row.status = "credited"
                row.payment_reference = (payment_reference or "").strip() or None
                row.paid_at = now
                row.credited_at = now
                row.updated_at = now
                uow.operations.point_transaction(
                    tg=row.tg,
                    balance_type="coins",
                    amount=credit,
                    balance_after=user.iv,
                    reason=f"recharge:{row.order_no}",
                    actor=actor,
                    idempotency_key=f"recharge:{row.id}",
                    metadata={"order_id": row.id, "amount_cents": row.amount_cents},
                )
                entry_type = "order_credited"
                description = f"充值订单 {row.order_no} 已确认并入账"
            uow.commerce.add_billing_entry(
                BillingEntry(
                    order_id=row.id,
                    tg=row.tg,
                    entry_type=entry_type,
                    amount_cents=row.amount_cents,
                    coins=(row.coins + row.bonus_coins) if approve else 0,
                    description=description,
                    actor_kind=actor.kind,
                    actor_id=actor.identifier,
                    metadata_json=_json({"payment_reference": row.payment_reference, "admin_note": row.admin_note}),
                )
            )
            add_notification(
                uow,
                tg=row.tg,
                category="billing",
                title="充值已入账" if approve else "充值订单未通过",
                body=(
                    f"订单 {row.order_no} 已确认，{row.coins + row.bonus_coins} 积分已到账。"
                    if approve
                    else f"订单 {row.order_no} 未通过审核。"
                    + (f" 备注：{row.admin_note}" if row.admin_note else "")
                ),
                severity="success" if approve else "warning",
                action_url="/billing",
                metadata={"order_id": row.id, "status": row.status},
            )
            uow.operations.audit(
                actor=actor,
                action="billing.order.approve" if approve else "billing.order.reject",
                resource_type="recharge_order",
                resource_id=row.id,
                detail={"order_no": row.order_no, "tg": row.tg},
            )
            uow.operations.event(
                "billing.order.updated",
                "user",
                str(row.tg),
                {"resource_type": "recharge_order", "resource_id": row.id, "tg": row.tg, "status": row.status},
            )
            uow.flush()
            return serialize_order(row)

    def cancel_order(self, order_id: str, *, tg: int, actor: Actor) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.commerce.get_order(order_id, for_update=True)
            if row is None or row.tg != tg:
                return None
            if row.status != "pending":
                raise RuntimeError("只有待处理订单可以取消")
            row.status = "canceled"
            row.canceled_at = utcnow()
            row.updated_at = row.canceled_at
            uow.commerce.add_billing_entry(
                BillingEntry(
                    order_id=row.id,
                    tg=row.tg,
                    entry_type="order_canceled",
                    amount_cents=row.amount_cents,
                    coins=0,
                    description=f"用户取消充值订单 {row.order_no}",
                    actor_kind=actor.kind,
                    actor_id=actor.identifier,
                )
            )
            uow.operations.audit(actor=actor, action="billing.order.cancel", resource_type="recharge_order", resource_id=row.id)
            uow.operations.event(
                "billing.order.updated",
                "user",
                str(tg),
                {"resource_type": "recharge_order", "resource_id": row.id, "tg": tg, "status": "canceled"},
            )
            uow.flush()
            return serialize_order(row)

    def ledger(self, *, tg=None, entry_type=None, limit=50, offset=0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.commerce.list_billing_entries(tg=tg, entry_type=entry_type, limit=limit, offset=offset)
            return {"items": [serialize_billing(row) for row in rows], "total": total, "limit": limit, "offset": offset}


class TicketService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def create(self, *, tg: int, subject: str, category: str, priority: str, body: str, actor: Actor) -> dict:
        now = utcnow()
        ticket_id = str(uuid4())
        with self._uow_factory() as uow:
            row = SupportTicket(
                id=ticket_id,
                ticket_no=f"TK{now.strftime('%y%m%d')}{ticket_id[:6].upper()}",
                tg=tg,
                subject=subject.strip(),
                category=category,
                priority=priority,
                status="open",
                last_reply_kind="user",
                last_reply_at=now,
                created_at=now,
                updated_at=now,
            )
            uow.commerce.add_ticket(row)
            uow.commerce.add_ticket_message(
                TicketMessage(ticket_id=ticket_id, sender_kind="user", sender_tg=tg, body=body.strip(), created_at=now)
            )
            uow.operations.audit(actor=actor, action="ticket.create", resource_type="ticket", resource_id=ticket_id, detail={"ticket_no": row.ticket_no})
            uow.operations.event(
                "ticket.created",
                "user",
                str(tg),
                {"resource_type": "ticket", "resource_id": ticket_id, "tg": tg, "ticket_no": row.ticket_no},
            )
            uow.flush()
            return serialize_ticket(row)

    def list(self, *, tg=None, status=None, assignee_tg=None, search=None, limit=50, offset=0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.commerce.list_tickets(
                tg=tg, status=status, assignee_tg=assignee_tg, search=search, limit=limit, offset=offset
            )
            return {"items": [serialize_ticket(row) for row in rows], "total": total, "limit": limit, "offset": offset}

    def detail(self, ticket_id: str, *, tg=None, include_internal=False) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.commerce.get_ticket(ticket_id, tg=tg)
            if row is None:
                return None
            messages = uow.commerce.ticket_messages(ticket_id, include_internal=include_internal)
            return {**serialize_ticket(row), "messages": [serialize_message(item) for item in messages]}

    def reply(
        self,
        ticket_id: str,
        *,
        body: str,
        actor: Actor,
        tg_scope: Optional[int],
        internal: bool = False,
    ) -> Optional[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.commerce.get_ticket(ticket_id, tg=tg_scope, for_update=True)
            if row is None:
                return None
            if row.status == "closed":
                raise RuntimeError("已关闭工单不能继续回复")
            is_user = tg_scope is not None
            sender_kind = "user" if is_user else "admin"
            uow.commerce.add_ticket_message(
                TicketMessage(
                    ticket_id=ticket_id,
                    sender_kind=sender_kind,
                    sender_tg=int(actor.identifier) if actor.identifier.isdigit() else None,
                    body=body.strip(),
                    internal=bool(internal and not is_user),
                    created_at=now,
                )
            )
            row.last_reply_kind = sender_kind
            row.last_reply_at = now
            row.updated_at = now
            if not internal:
                row.status = "pending_staff" if is_user else "pending_user"
                row.resolved_at = None
            if not is_user and not internal:
                add_notification(
                    uow,
                    tg=row.tg,
                    category="ticket",
                    title=f"工单 {row.ticket_no} 收到回复",
                    body=body.strip(),
                    action_url="/tickets",
                    metadata={"ticket_id": row.id},
                )
            uow.operations.audit(
                actor=actor,
                action="ticket.reply",
                resource_type="ticket",
                resource_id=ticket_id,
                detail={"internal": bool(internal and not is_user)},
            )
            uow.operations.event(
                "ticket.updated",
                "user",
                str(row.tg),
                {"resource_type": "ticket", "resource_id": ticket_id, "tg": row.tg, "status": row.status},
            )
            uow.flush()
            return serialize_ticket(row)

    def update(
        self,
        ticket_id: str,
        *,
        status: Optional[str],
        priority: Optional[str],
        assignee_tg: Optional[int],
        assignee_supplied: bool,
        actor: Actor,
    ) -> Optional[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.commerce.get_ticket(ticket_id, for_update=True)
            if row is None:
                return None
            changes = {}
            if status is not None:
                row.status = status
                changes["status"] = status
                row.resolved_at = now if status == "resolved" else None
                row.closed_at = now if status == "closed" else None
            if priority is not None:
                row.priority = priority
                changes["priority"] = priority
            if assignee_supplied:
                row.assignee_tg = assignee_tg
                changes["assignee_tg"] = assignee_tg
            row.updated_at = now
            uow.operations.audit(actor=actor, action="ticket.update", resource_type="ticket", resource_id=ticket_id, detail=changes)
            uow.operations.event(
                "ticket.updated",
                "user",
                str(row.tg),
                {"resource_type": "ticket", "resource_id": ticket_id, "tg": row.tg, **changes},
            )
            uow.flush()
            return serialize_ticket(row)


class MediaRequestService:
    ALLOWED_USER_CANCEL = {"submitted", "reviewing"}

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def create(
        self,
        *,
        tg: int,
        title: str,
        year: Optional[int],
        media_type: str,
        description: Optional[str],
        actor: Actor,
    ) -> dict:
        now = utcnow()
        request_id = str(uuid4())
        with self._uow_factory() as uow:
            row = MediaRequest(
                id=request_id,
                request_no=f"RQ{now.strftime('%y%m%d')}{request_id[:6].upper()}",
                tg=tg,
                title=title.strip(),
                year=year,
                media_type=media_type,
                description=(description or "").strip() or None,
                source="web",
                created_at=now,
                updated_at=now,
            )
            uow.commerce.add_media_request(row)
            uow.operations.audit(actor=actor, action="request.create", resource_type="media_request", resource_id=request_id, detail={"title": row.title})
            uow.operations.event(
                "request.created",
                "user",
                str(tg),
                {"resource_type": "media_request", "resource_id": request_id, "tg": tg, "request_no": row.request_no},
            )
            uow.flush()
            return serialize_media_request(row)

    def list(self, *, tg=None, status=None, search=None, limit=50, offset=0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.commerce.list_media_requests(tg=tg, status=status, search=search, limit=limit, offset=offset)
            return {"items": [serialize_media_request(row) for row in rows], "total": total, "limit": limit, "offset": offset}

    def transfer_candidates(self, limit=200) -> list[dict]:
        with self._uow_factory() as uow:
            return [
                serialize_media_request(row)
                for row in uow.commerce.list_transfer_candidates(limit=limit)
            ]

    def cancel(self, request_id: str, *, tg: int, actor: Actor) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.commerce.get_media_request(request_id, tg=tg, for_update=True)
            if row is None:
                return None
            if row.status not in self.ALLOWED_USER_CANCEL:
                raise RuntimeError("当前求片状态不能取消")
            row.status = "canceled"
            row.canceled_at = utcnow()
            row.updated_at = row.canceled_at
            uow.operations.audit(actor=actor, action="request.cancel", resource_type="media_request", resource_id=request_id)
            uow.operations.event(
                "request.updated",
                "user",
                str(tg),
                {"resource_type": "media_request", "resource_id": request_id, "tg": tg, "status": "canceled"},
            )
            uow.flush()
            return serialize_media_request(row)

    def update(
        self,
        request_id: str,
        *,
        data: dict,
        actor: Actor,
    ) -> Optional[dict]:
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.commerce.get_media_request(request_id, for_update=True)
            if row is None:
                return None
            previous_status = row.status
            if data.get("download_id"):
                existing = uow.commerce.media_request_by_download_id(data["download_id"])
                if existing is not None and existing.id != row.id:
                    raise RuntimeError("该下载 ID 已绑定其他求片记录")
            for field in ("status", "priority", "external_ref", "download_id", "cost_coins", "progress", "admin_note"):
                if field in data:
                    setattr(row, field, data[field])
            row.reviewed_by = int(actor.identifier) if actor.identifier.isdigit() else None
            row.reviewed_at = now
            row.updated_at = now
            if row.status == "completed":
                row.completed_at = row.completed_at or now
                row.progress = 100
            else:
                row.completed_at = None
            if row.status == "canceled":
                row.canceled_at = row.canceled_at or now
            else:
                row.canceled_at = None
            if row.status != previous_status:
                add_notification(
                    uow,
                    tg=row.tg,
                    category="request",
                    title=f"求片《{row.title}》状态已更新",
                    body=f"当前状态：{row.status}。" + (f" 备注：{row.admin_note}" if row.admin_note else ""),
                    severity="success" if row.status == "completed" else "warning" if row.status == "rejected" else "info",
                    action_url="/requests",
                    metadata={"request_id": row.id, "status": row.status},
                )
            uow.operations.audit(actor=actor, action="request.update", resource_type="media_request", resource_id=request_id, detail=data)
            uow.operations.event(
                "request.updated",
                "user",
                str(row.tg),
                {"resource_type": "media_request", "resource_id": request_id, "tg": row.tg, "status": row.status},
            )
            uow.flush()
            return serialize_media_request(row)

    def import_download(
        self,
        *,
        tg: int,
        download_id: str,
        title: str,
        description: Optional[str],
        cost_coins: int,
        actor: Actor,
    ) -> dict:
        now = utcnow()
        with self._uow_factory() as uow:
            existing = uow.commerce.media_request_by_download_id(download_id)
            if existing:
                return serialize_media_request(existing)
            request_id = str(uuid4())
            row = MediaRequest(
                id=request_id,
                request_no=f"MP{now.strftime('%y%m%d')}{request_id[:6].upper()}",
                tg=tg,
                title=title[:255],
                description=description,
                status="approved",
                source="telegram",
                download_id=download_id,
                cost_coins=int(cost_coins),
                created_at=now,
                updated_at=now,
            )
            uow.commerce.add_media_request(row)
            uow.operations.audit(actor=actor, action="request.import", resource_type="media_request", resource_id=request_id, detail={"download_id": download_id})
            uow.operations.event(
                "request.created",
                "user",
                str(tg),
                {"resource_type": "media_request", "resource_id": request_id, "tg": tg, "request_no": row.request_no},
            )
            uow.flush()
            return serialize_media_request(row)

    def sync_download(
        self,
        download_id: str,
        *,
        download_state: Optional[str],
        transfer_state,
        progress: Optional[float],
    ) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.commerce.media_request_by_download_id(download_id)
            if row is None:
                return None
            previous = (row.status, row.progress, row.completed_at)
            previous_status = row.status
            transfer_text = str(transfer_state).lower() if transfer_state is not None else ""
            if transfer_text in {"true", "1", "success", "completed"}:
                row.status = "completed"
                row.progress = 100
                row.completed_at = row.completed_at or utcnow()
            elif transfer_state is not None and transfer_text in {"false", "0", "failed", "error"}:
                row.status = "rejected"
            elif download_state == "downloading":
                row.status = "downloading"
            elif download_state == "failed":
                row.status = "rejected"
            elif download_state == "completed":
                row.status = "downloading"
            if progress is not None and row.status != "completed":
                row.progress = max(0, min(100, int(float(progress))))
            if previous == (row.status, row.progress, row.completed_at):
                return serialize_media_request(row)
            row.updated_at = utcnow()
            if row.status != previous_status:
                add_notification(
                    uow,
                    tg=row.tg,
                    category="request",
                    title=f"求片《{row.title}》状态已更新",
                    body=f"当前状态：{row.status}，进度 {row.progress}%。",
                    severity="success" if row.status == "completed" else "warning" if row.status == "rejected" else "info",
                    action_url="/requests",
                    metadata={"request_id": row.id, "status": row.status, "progress": row.progress},
                )
            uow.operations.event(
                "request.updated",
                "user",
                str(row.tg),
                {
                    "resource_type": "media_request",
                    "resource_id": row.id,
                    "tg": row.tg,
                    "status": row.status,
                    "progress": row.progress,
                },
            )
            uow.flush()
            return serialize_media_request(row)
