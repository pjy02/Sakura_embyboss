"""Commerce, support ticket and media request models."""

from sqlalchemy import (
    BigInteger,
    Boolean,
    Column,
    DateTime,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)

from bot.sql_helper import Base
from bot.sql_helper.sql_application import utcnow


class RechargeProduct(Base):
    __tablename__ = "recharge_products"
    __table_args__ = (Index("ix_recharge_products_enabled_sort", "enabled", "sort_order"),)

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(100), nullable=False)
    description = Column(String(500), nullable=True)
    amount_cents = Column(Integer, nullable=False)
    coins = Column(Integer, nullable=False)
    bonus_coins = Column(Integer, nullable=False, default=0)
    enabled = Column(Boolean, nullable=False, default=True)
    sort_order = Column(Integer, nullable=False, default=0)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class RechargeOrder(Base):
    __tablename__ = "recharge_orders"
    __table_args__ = (
        UniqueConstraint("order_no", name="uq_recharge_orders_order_no"),
        Index("ix_recharge_orders_tg_created", "tg", "created_at"),
        Index("ix_recharge_orders_status_created", "status", "created_at"),
    )

    id = Column(String(36), primary_key=True)
    order_no = Column(String(32), nullable=False)
    tg = Column(BigInteger, nullable=False)
    product_id = Column(Integer, nullable=True)
    product_name = Column(String(100), nullable=False)
    amount_cents = Column(Integer, nullable=False)
    coins = Column(Integer, nullable=False)
    bonus_coins = Column(Integer, nullable=False, default=0)
    payment_method = Column(String(32), nullable=False, default="manual")
    payment_reference = Column(String(255), nullable=True)
    status = Column(String(32), nullable=False, default="pending")
    user_note = Column(String(500), nullable=True)
    admin_note = Column(String(500), nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    paid_at = Column(DateTime, nullable=True)
    credited_at = Column(DateTime, nullable=True)
    canceled_at = Column(DateTime, nullable=True)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class BillingEntry(Base):
    __tablename__ = "billing_entries"
    __table_args__ = (
        Index("ix_billing_entries_tg_created", "tg", "created_at"),
        Index("ix_billing_entries_order", "order_id", "created_at"),
        Index("ix_billing_entries_type_created", "entry_type", "created_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    order_id = Column(String(36), nullable=True)
    tg = Column(BigInteger, nullable=False)
    entry_type = Column(String(32), nullable=False)
    amount_cents = Column(Integer, nullable=True)
    coins = Column(Integer, nullable=True)
    description = Column(String(500), nullable=False)
    actor_kind = Column(String(32), nullable=False)
    actor_id = Column(String(128), nullable=False)
    metadata_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class SupportTicket(Base):
    __tablename__ = "support_tickets"
    __table_args__ = (
        UniqueConstraint("ticket_no", name="uq_support_tickets_ticket_no"),
        Index("ix_support_tickets_tg_updated", "tg", "updated_at"),
        Index("ix_support_tickets_status_priority", "status", "priority"),
        Index("ix_support_tickets_assignee", "assignee_tg", "status"),
    )

    id = Column(String(36), primary_key=True)
    ticket_no = Column(String(32), nullable=False)
    tg = Column(BigInteger, nullable=False)
    subject = Column(String(200), nullable=False)
    category = Column(String(32), nullable=False, default="general")
    priority = Column(String(20), nullable=False, default="normal")
    status = Column(String(32), nullable=False, default="open")
    assignee_tg = Column(BigInteger, nullable=True)
    last_reply_kind = Column(String(32), nullable=False, default="user")
    last_reply_at = Column(DateTime, nullable=False, default=utcnow)
    resolved_at = Column(DateTime, nullable=True)
    closed_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class TicketMessage(Base):
    __tablename__ = "ticket_messages"
    __table_args__ = (Index("ix_ticket_messages_ticket_created", "ticket_id", "created_at"),)

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    ticket_id = Column(String(36), nullable=False)
    sender_kind = Column(String(32), nullable=False)
    sender_tg = Column(BigInteger, nullable=True)
    body = Column(Text, nullable=False)
    internal = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class MediaRequest(Base):
    __tablename__ = "media_requests"
    __table_args__ = (
        UniqueConstraint("request_no", name="uq_media_requests_request_no"),
        UniqueConstraint("download_id", name="uq_media_requests_download_id"),
        Index("ix_media_requests_tg_created", "tg", "created_at"),
        Index("ix_media_requests_status_updated", "status", "updated_at"),
    )

    id = Column(String(128), primary_key=True)
    request_no = Column(String(32), nullable=False)
    tg = Column(BigInteger, nullable=False)
    title = Column(String(255), nullable=False)
    year = Column(Integer, nullable=True)
    media_type = Column(String(32), nullable=False, default="other")
    description = Column(Text, nullable=True)
    status = Column(String(32), nullable=False, default="submitted")
    priority = Column(String(20), nullable=False, default="normal")
    source = Column(String(32), nullable=False, default="web")
    external_ref = Column(String(255), nullable=True)
    download_id = Column(String(255), nullable=True)
    cost_coins = Column(Integer, nullable=False, default=0)
    progress = Column(Integer, nullable=False, default=0)
    admin_note = Column(String(1000), nullable=True)
    reviewed_by = Column(BigInteger, nullable=True)
    reviewed_at = Column(DateTime, nullable=True)
    completed_at = Column(DateTime, nullable=True)
    canceled_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)
