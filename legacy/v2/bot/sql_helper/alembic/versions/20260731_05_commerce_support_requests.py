"""commerce, support tickets and media requests

Revision ID: 20260731_05
Revises: 20260731_04
Create Date: 2026-07-31 16:00:00
"""

import json
from datetime import datetime
from uuid import NAMESPACE_URL, uuid5

from alembic import op
import sqlalchemy as sa


revision = "20260731_05"
down_revision = "20260731_04"
branch_labels = None
depends_on = None


def _tables():
    return set(sa.inspect(op.get_bind()).get_table_names())


def upgrade() -> None:
    tables = _tables()
    if "recharge_products" not in tables:
        op.create_table(
            "recharge_products",
            sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
            sa.Column("name", sa.String(100), nullable=False),
            sa.Column("description", sa.String(500), nullable=True),
            sa.Column("amount_cents", sa.Integer(), nullable=False),
            sa.Column("coins", sa.Integer(), nullable=False),
            sa.Column("bonus_coins", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("sort_order", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_recharge_products_enabled_sort", "recharge_products", ["enabled", "sort_order"])

    if "recharge_orders" not in tables:
        op.create_table(
            "recharge_orders",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("order_no", sa.String(32), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("product_id", sa.Integer(), nullable=True),
            sa.Column("product_name", sa.String(100), nullable=False),
            sa.Column("amount_cents", sa.Integer(), nullable=False),
            sa.Column("coins", sa.Integer(), nullable=False),
            sa.Column("bonus_coins", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("payment_method", sa.String(32), nullable=False, server_default="manual"),
            sa.Column("payment_reference", sa.String(255), nullable=True),
            sa.Column("status", sa.String(32), nullable=False, server_default="pending"),
            sa.Column("user_note", sa.String(500), nullable=True),
            sa.Column("admin_note", sa.String(500), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("paid_at", sa.DateTime(), nullable=True),
            sa.Column("credited_at", sa.DateTime(), nullable=True),
            sa.Column("canceled_at", sa.DateTime(), nullable=True),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("order_no", name="uq_recharge_orders_order_no"),
        )
        op.create_index("ix_recharge_orders_tg_created", "recharge_orders", ["tg", "created_at"])
        op.create_index("ix_recharge_orders_status_created", "recharge_orders", ["status", "created_at"])

    if "billing_entries" not in tables:
        op.create_table(
            "billing_entries",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("order_id", sa.String(36), nullable=True),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("entry_type", sa.String(32), nullable=False),
            sa.Column("amount_cents", sa.Integer(), nullable=True),
            sa.Column("coins", sa.Integer(), nullable=True),
            sa.Column("description", sa.String(500), nullable=False),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_billing_entries_tg_created", "billing_entries", ["tg", "created_at"])
        op.create_index("ix_billing_entries_order", "billing_entries", ["order_id", "created_at"])
        op.create_index("ix_billing_entries_type_created", "billing_entries", ["entry_type", "created_at"])

    if "support_tickets" not in tables:
        op.create_table(
            "support_tickets",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("ticket_no", sa.String(32), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("subject", sa.String(200), nullable=False),
            sa.Column("category", sa.String(32), nullable=False, server_default="general"),
            sa.Column("priority", sa.String(20), nullable=False, server_default="normal"),
            sa.Column("status", sa.String(32), nullable=False, server_default="open"),
            sa.Column("assignee_tg", sa.BigInteger(), nullable=True),
            sa.Column("last_reply_kind", sa.String(32), nullable=False, server_default="user"),
            sa.Column("last_reply_at", sa.DateTime(), nullable=False),
            sa.Column("resolved_at", sa.DateTime(), nullable=True),
            sa.Column("closed_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("ticket_no", name="uq_support_tickets_ticket_no"),
        )
        op.create_index("ix_support_tickets_tg_updated", "support_tickets", ["tg", "updated_at"])
        op.create_index("ix_support_tickets_status_priority", "support_tickets", ["status", "priority"])
        op.create_index("ix_support_tickets_assignee", "support_tickets", ["assignee_tg", "status"])

    if "ticket_messages" not in tables:
        op.create_table(
            "ticket_messages",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("ticket_id", sa.String(36), nullable=False),
            sa.Column("sender_kind", sa.String(32), nullable=False),
            sa.Column("sender_tg", sa.BigInteger(), nullable=True),
            sa.Column("body", sa.Text(), nullable=False),
            sa.Column("internal", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_ticket_messages_ticket_created", "ticket_messages", ["ticket_id", "created_at"])

    if "media_requests" not in tables:
        op.create_table(
            "media_requests",
            sa.Column("id", sa.String(128), nullable=False),
            sa.Column("request_no", sa.String(32), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("title", sa.String(255), nullable=False),
            sa.Column("year", sa.Integer(), nullable=True),
            sa.Column("media_type", sa.String(32), nullable=False, server_default="other"),
            sa.Column("description", sa.Text(), nullable=True),
            sa.Column("status", sa.String(32), nullable=False, server_default="submitted"),
            sa.Column("priority", sa.String(20), nullable=False, server_default="normal"),
            sa.Column("source", sa.String(32), nullable=False, server_default="web"),
            sa.Column("external_ref", sa.String(255), nullable=True),
            sa.Column("download_id", sa.String(255), nullable=True),
            sa.Column("cost_coins", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("progress", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("admin_note", sa.String(1000), nullable=True),
            sa.Column("reviewed_by", sa.BigInteger(), nullable=True),
            sa.Column("reviewed_at", sa.DateTime(), nullable=True),
            sa.Column("completed_at", sa.DateTime(), nullable=True),
            sa.Column("canceled_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("request_no", name="uq_media_requests_request_no"),
            sa.UniqueConstraint("download_id", name="uq_media_requests_download_id"),
        )
        op.create_index("ix_media_requests_tg_created", "media_requests", ["tg", "created_at"])
        op.create_index("ix_media_requests_status_updated", "media_requests", ["status", "updated_at"])

    now = datetime.utcnow()
    conn = op.get_bind()
    if "recharge_products" not in tables:
        products = (
            ("体验包", "适合轻量使用", 600, 60, 0, 10),
            ("标准包", "常用积分补充", 3000, 330, 30, 20),
            ("支持者包", "包含额外赠送积分", 6800, 800, 120, 30),
        )
        for name, description, amount, coins, bonus, sort_order in products:
            conn.execute(
                sa.text(
                    """
                    INSERT INTO recharge_products
                    (name, description, amount_cents, coins, bonus_coins, enabled, sort_order, revision, created_at, updated_at)
                    VALUES (:name, :description, :amount, :coins, :bonus, 1, :sort_order, 1, :now, :now)
                    """
                ),
                {"name": name, "description": description, "amount": amount, "coins": coins, "bonus": bonus, "sort_order": sort_order, "now": now},
            )

    if "request_records" in tables:
        rows = conn.execute(
            sa.text(
                """
                SELECT download_id, tg, request_name, cost, detail, download_state,
                       transfer_state, progress, create_at, update_at
                FROM request_records
                """
            )
        ).mappings()
        for row in rows:
            request_id = str(uuid5(NAMESPACE_URL, f"sakura-request:{row['download_id']}"))
            exists = conn.execute(sa.text("SELECT 1 FROM media_requests WHERE id = :id"), {"id": request_id}).first()
            if exists:
                continue
            transfer = row["transfer_state"]
            transfer_text = str(transfer).lower() if transfer is not None else ""
            download = row["download_state"]
            status = (
                "completed" if transfer_text in {"true", "1", "success", "completed"} else
                "rejected" if transfer_text in {"false", "0", "failed", "error"} else
                "downloading" if download in {"downloading", "completed"} else
                "rejected" if download == "failed" else "approved"
            )
            try:
                cost = int(float(row["cost"] or 0))
            except (TypeError, ValueError):
                cost = 0
            try:
                progress = max(0, min(100, int(float(row["progress"] or 0))))
            except (TypeError, ValueError):
                progress = 0
            if status == "completed":
                progress = 100
            conn.execute(
                sa.text(
                    """
                    INSERT INTO media_requests
                    (id, request_no, tg, title, media_type, description, status, priority,
                     source, download_id, cost_coins, progress, created_at, updated_at)
                    VALUES
                    (:id, :request_no, :tg, :title, 'other', :description, :status, 'normal',
                     'telegram', :download_id, :cost, :progress, :created_at, :updated_at)
                    """
                ),
                {
                    "id": request_id,
                    "request_no": f"MP-{request_id[:10].upper()}",
                    "tg": row["tg"],
                    "title": row["request_name"],
                    "description": row["detail"],
                    "status": status,
                    "download_id": row["download_id"],
                    "cost": cost,
                    "progress": progress,
                    "created_at": row["create_at"] or now,
                    "updated_at": row["update_at"] or now,
                },
            )

    if "web_roles" in tables:
        role_additions = {
            "admin": ["billing:*", "tickets:*", "requests:*"],
            "operator": ["billing:read", "billing:update", "tickets:*", "requests:*"],
            "auditor": ["billing:read", "tickets:read", "requests:read"],
        }
        for role, additions in role_additions.items():
            row = conn.execute(
                sa.text("SELECT permissions_json FROM web_roles WHERE name = :role"),
                {"role": role},
            ).first()
            if row is None:
                continue
            try:
                current = json.loads(row[0] or "[]")
            except (TypeError, ValueError):
                current = []
            permissions = list(dict.fromkeys([*current, *additions]))
            conn.execute(
                sa.text("UPDATE web_roles SET permissions_json = :permissions WHERE name = :role"),
                {"permissions": json.dumps(permissions, ensure_ascii=False), "role": role},
            )


def downgrade() -> None:
    for table_name in (
        "media_requests",
        "ticket_messages",
        "support_tickets",
        "billing_entries",
        "recharge_orders",
        "recharge_products",
    ):
        if table_name in _tables():
            op.drop_table(table_name)
