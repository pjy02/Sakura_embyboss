from typing import Optional

from sqlalchemy import or_
from sqlalchemy.orm import Session

from bot.sql_helper.sql_commerce import (
    BillingEntry,
    MediaRequest,
    RechargeOrder,
    RechargeProduct,
    SupportTicket,
    TicketMessage,
)


class CommerceRepository:
    def __init__(self, session: Session):
        self.session = session

    def get_product(self, product_id: int, *, for_update: bool = False):
        query = self.session.query(RechargeProduct).filter(RechargeProduct.id == product_id)
        return query.with_for_update().first() if for_update else query.first()

    def list_products(self, *, enabled_only: bool = False):
        query = self.session.query(RechargeProduct)
        if enabled_only:
            query = query.filter(RechargeProduct.enabled.is_(True))
        return query.order_by(RechargeProduct.sort_order.asc(), RechargeProduct.id.asc()).all()

    def add_product(self, row: RechargeProduct):
        self.session.add(row)

    def get_order(self, order_id: str, *, for_update: bool = False):
        query = self.session.query(RechargeOrder).filter(RechargeOrder.id == order_id)
        return query.with_for_update().first() if for_update else query.first()

    def add_order(self, row: RechargeOrder):
        self.session.add(row)

    def list_orders(self, *, tg=None, status=None, search=None, limit=50, offset=0):
        query = self.session.query(RechargeOrder)
        if tg is not None:
            query = query.filter(RechargeOrder.tg == tg)
        if status:
            query = query.filter(RechargeOrder.status == status)
        if search:
            pattern = f"%{search.strip()}%"
            conditions = [
                RechargeOrder.order_no.like(pattern),
                RechargeOrder.product_name.like(pattern),
                RechargeOrder.payment_reference.like(pattern),
            ]
            if search.strip().isdigit():
                conditions.append(RechargeOrder.tg == int(search))
            query = query.filter(or_(*conditions))
        total = query.count()
        return query.order_by(RechargeOrder.created_at.desc()).offset(offset).limit(limit).all(), total

    def add_billing_entry(self, row: BillingEntry):
        self.session.add(row)

    def list_billing_entries(self, *, tg=None, entry_type=None, limit=50, offset=0):
        query = self.session.query(BillingEntry)
        if tg is not None:
            query = query.filter(BillingEntry.tg == tg)
        if entry_type:
            query = query.filter(BillingEntry.entry_type == entry_type)
        total = query.count()
        return query.order_by(BillingEntry.created_at.desc()).offset(offset).limit(limit).all(), total

    def add_ticket(self, row: SupportTicket):
        self.session.add(row)

    def get_ticket(self, ticket_id: str, *, tg=None, for_update=False):
        query = self.session.query(SupportTicket).filter(SupportTicket.id == ticket_id)
        if tg is not None:
            query = query.filter(SupportTicket.tg == tg)
        return query.with_for_update().first() if for_update else query.first()

    def list_tickets(self, *, tg=None, status=None, assignee_tg=None, search=None, limit=50, offset=0):
        query = self.session.query(SupportTicket)
        if tg is not None:
            query = query.filter(SupportTicket.tg == tg)
        if status:
            query = query.filter(SupportTicket.status == status)
        if assignee_tg is not None:
            query = query.filter(SupportTicket.assignee_tg == assignee_tg)
        if search:
            pattern = f"%{search.strip()}%"
            conditions = [SupportTicket.ticket_no.like(pattern), SupportTicket.subject.like(pattern)]
            if search.strip().isdigit():
                conditions.append(SupportTicket.tg == int(search))
            query = query.filter(or_(*conditions))
        total = query.count()
        return query.order_by(SupportTicket.updated_at.desc()).offset(offset).limit(limit).all(), total

    def add_ticket_message(self, row: TicketMessage):
        self.session.add(row)

    def ticket_messages(self, ticket_id: str, *, include_internal: bool):
        query = self.session.query(TicketMessage).filter(TicketMessage.ticket_id == ticket_id)
        if not include_internal:
            query = query.filter(TicketMessage.internal.is_(False))
        return query.order_by(TicketMessage.created_at.asc(), TicketMessage.id.asc()).all()

    def add_media_request(self, row: MediaRequest):
        self.session.add(row)

    def get_media_request(self, request_id: str, *, tg=None, for_update=False):
        query = self.session.query(MediaRequest).filter(MediaRequest.id == request_id)
        if tg is not None:
            query = query.filter(MediaRequest.tg == tg)
        return query.with_for_update().first() if for_update else query.first()

    def media_request_by_download_id(self, download_id: str):
        return self.session.query(MediaRequest).filter(MediaRequest.download_id == download_id).first()

    def list_transfer_candidates(self, limit=200):
        return (
            self.session.query(MediaRequest)
            .filter(
                MediaRequest.download_id.isnot(None),
                MediaRequest.status.in_(("approved", "searching", "downloading")),
            )
            .order_by(MediaRequest.updated_at.asc())
            .limit(limit)
            .all()
        )

    def list_media_requests(self, *, tg=None, status=None, search=None, limit=50, offset=0):
        query = self.session.query(MediaRequest)
        if tg is not None:
            query = query.filter(MediaRequest.tg == tg)
        if status:
            query = query.filter(MediaRequest.status == status)
        if search:
            pattern = f"%{search.strip()}%"
            conditions = [
                MediaRequest.request_no.like(pattern),
                MediaRequest.title.like(pattern),
                MediaRequest.download_id.like(pattern),
            ]
            if search.strip().isdigit():
                conditions.append(MediaRequest.tg == int(search))
            query = query.filter(or_(*conditions))
        total = query.count()
        return query.order_by(MediaRequest.updated_at.desc()).offset(offset).limit(limit).all(), total
