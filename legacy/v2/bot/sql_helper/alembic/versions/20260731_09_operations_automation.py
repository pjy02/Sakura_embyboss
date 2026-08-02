"""risk automation, diagnostics and account lifecycle

Revision ID: 20260731_09
Revises: 20260731_08
Create Date: 2026-07-31 23:55:00
"""

from datetime import datetime

from alembic import op
import sqlalchemy as sa


revision = "20260731_09"
down_revision = "20260731_08"
branch_labels = None
depends_on = None


def upgrade() -> None:
    inspector = sa.inspect(op.get_bind())
    tables = set(inspector.get_table_names())
    if "risk_rules" not in tables:
        table = op.create_table(
            "risk_rules",
            sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
            sa.Column("name", sa.String(100), nullable=False),
            sa.Column("event_pattern", sa.String(100), nullable=False),
            sa.Column("severity", sa.String(20), nullable=False, server_default="warning"),
            sa.Column("threshold_count", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("window_minutes", sa.Integer(), nullable=False, server_default="10"),
            sa.Column("cooldown_minutes", sa.Integer(), nullable=False, server_default="30"),
            sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("telegram_alert", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
        )
        now = datetime.utcnow()
        op.bulk_insert(table, [
            {"name": "Emby 登录暴力尝试", "event_pattern": "auth.emby.failed", "severity": "danger", "threshold_count": 5, "window_minutes": 10, "cooldown_minutes": 30, "enabled": True, "telegram_alert": True, "revision": 1, "created_at": now, "updated_at": now},
            {"name": "Telegram 身份不匹配", "event_pattern": "auth.telegram.identity_mismatch", "severity": "warning", "threshold_count": 3, "window_minutes": 10, "cooldown_minutes": 30, "enabled": True, "telegram_alert": True, "revision": 1, "created_at": now, "updated_at": now},
            {"name": "封禁设备再次播放", "event_pattern": "device.banned_playback", "severity": "danger", "threshold_count": 1, "window_minutes": 5, "cooldown_minutes": 30, "enabled": True, "telegram_alert": True, "revision": 1, "created_at": now, "updated_at": now},
            {"name": "外部服务连续故障", "event_pattern": "service.probe.failed", "severity": "danger", "threshold_count": 1, "window_minutes": 10, "cooldown_minutes": 30, "enabled": True, "telegram_alert": True, "revision": 1, "created_at": now, "updated_at": now},
        ])
    if "service_probes" not in tables:
        op.create_table(
            "service_probes",
            sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
            sa.Column("service_name", sa.String(64), nullable=False),
            sa.Column("service_kind", sa.String(32), nullable=False),
            sa.Column("status", sa.String(20), nullable=False),
            sa.Column("latency_ms", sa.Integer(), nullable=True),
            sa.Column("status_code", sa.Integer(), nullable=True),
            sa.Column("message", sa.String(1000), nullable=True),
            sa.Column("detail_json", sa.Text(), nullable=True),
            sa.Column("checked_at", sa.DateTime(), nullable=False),
        )
        op.create_index("ix_service_probes_service_checked", "service_probes", ["service_name", "checked_at"])
    if "alert_deliveries" not in tables:
        op.create_table(
            "alert_deliveries",
            sa.Column("id", sa.String(36), primary_key=True),
            sa.Column("security_event_id", sa.BigInteger(), nullable=False),
            sa.Column("recipient_tg", sa.BigInteger(), nullable=False),
            sa.Column("status", sa.String(20), nullable=False, server_default="pending"),
            sa.Column("attempt_count", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("error_message", sa.String(1000), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("sent_at", sa.DateTime(), nullable=True),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("security_event_id", "recipient_tg", name="uq_alert_event_recipient"),
        )
        op.create_index("ix_alert_deliveries_status_created", "alert_deliveries", ["status", "created_at"])
    if "account_lifecycle_events" not in tables:
        op.create_table(
            "account_lifecycle_events",
            sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
            sa.Column("batch_id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("action", sa.String(32), nullable=False),
            sa.Column("status", sa.String(20), nullable=False),
            sa.Column("detail_json", sa.Text(), nullable=True),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("batch_id", "tg", "action", name="uq_account_lifecycle_batch_user_action"),
        )
        op.create_index("ix_account_lifecycle_tg_created", "account_lifecycle_events", ["tg", "created_at"])
        op.create_index("ix_account_lifecycle_action_created", "account_lifecycle_events", ["action", "created_at"])


def downgrade() -> None:
    inspector = sa.inspect(op.get_bind())
    tables = set(inspector.get_table_names())
    for table in ("account_lifecycle_events", "alert_deliveries", "service_probes", "risk_rules"):
        if table in tables:
            op.drop_table(table)
