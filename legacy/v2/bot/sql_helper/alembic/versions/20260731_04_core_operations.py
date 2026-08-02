"""core operations console models

Revision ID: 20260731_04
Revises: 20260730_03
Create Date: 2026-07-31 12:00:00
"""

from alembic import op
import sqlalchemy as sa


revision = "20260731_04"
down_revision = "20260730_03"
branch_labels = None
depends_on = None


def _tables():
    return set(sa.inspect(op.get_bind()).get_table_names())


def upgrade() -> None:
    tables = _tables()
    if "line_endpoints" not in tables:
        op.create_table(
            "line_endpoints",
            sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
            sa.Column("name", sa.String(100), nullable=False),
            sa.Column("base_url", sa.String(512), nullable=False),
            sa.Column("region", sa.String(100), nullable=True),
            sa.Column("carrier", sa.String(100), nullable=True),
            sa.Column("audience", sa.String(32), nullable=False, server_default="all"),
            sa.Column("weight", sa.Integer(), nullable=False, server_default="100"),
            sa.Column("sort_order", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("maintenance", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("last_status", sa.String(32), nullable=False, server_default="unknown"),
            sa.Column("last_latency_ms", sa.Integer(), nullable=True),
            sa.Column("last_error", sa.String(512), nullable=True),
            sa.Column("last_checked_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("base_url"),
        )
        op.create_index("ix_line_endpoints_enabled_sort", "line_endpoints", ["enabled", "sort_order"])
        op.create_index("ix_line_endpoints_status", "line_endpoints", ["last_status", "last_checked_at"])

    if "line_health_samples" not in tables:
        op.create_table(
            "line_health_samples",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("line_id", sa.Integer(), nullable=False),
            sa.Column("success", sa.Boolean(), nullable=False),
            sa.Column("status_code", sa.Integer(), nullable=True),
            sa.Column("latency_ms", sa.Integer(), nullable=True),
            sa.Column("error_message", sa.String(512), nullable=True),
            sa.Column("checked_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_line_health_line_checked", "line_health_samples", ["line_id", "checked_at"])

    if "playback_sessions" not in tables:
        op.create_table(
            "playback_sessions",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("session_id", sa.String(128), nullable=False),
            sa.Column("emby_user_id", sa.String(128), nullable=True),
            sa.Column("emby_user_name", sa.String(255), nullable=True),
            sa.Column("tg", sa.BigInteger(), nullable=True),
            sa.Column("item_id", sa.String(128), nullable=True),
            sa.Column("item_name", sa.String(512), nullable=True),
            sa.Column("series_name", sa.String(512), nullable=True),
            sa.Column("item_type", sa.String(64), nullable=True),
            sa.Column("client_name", sa.String(255), nullable=True),
            sa.Column("app_version", sa.String(64), nullable=True),
            sa.Column("device_key", sa.String(255), nullable=True),
            sa.Column("device_name", sa.String(255), nullable=True),
            sa.Column("remote_address", sa.String(128), nullable=True),
            sa.Column("position_ticks", sa.BigInteger(), nullable=False, server_default="0"),
            sa.Column("runtime_ticks", sa.BigInteger(), nullable=False, server_default="0"),
            sa.Column("progress_percent", sa.Float(), nullable=False, server_default="0"),
            sa.Column("is_paused", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("is_transcoding", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("started_at", sa.DateTime(), nullable=False),
            sa.Column("last_seen_at", sa.DateTime(), nullable=False),
            sa.Column("ended_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_playback_sessions_active", "playback_sessions", ["ended_at", "last_seen_at"])
        op.create_index("ix_playback_sessions_session_active", "playback_sessions", ["session_id", "ended_at"])
        op.create_index("ix_playback_sessions_user_started", "playback_sessions", ["emby_user_id", "started_at"])
        op.create_index("ix_playback_sessions_device_started", "playback_sessions", ["device_key", "started_at"])

    if "known_devices" not in tables:
        op.create_table(
            "known_devices",
            sa.Column("device_key", sa.String(255), nullable=False),
            sa.Column("emby_user_id", sa.String(128), nullable=True),
            sa.Column("emby_user_name", sa.String(255), nullable=True),
            sa.Column("tg", sa.BigInteger(), nullable=True),
            sa.Column("device_name", sa.String(255), nullable=True),
            sa.Column("client_name", sa.String(255), nullable=True),
            sa.Column("app_version", sa.String(64), nullable=True),
            sa.Column("last_ip", sa.String(128), nullable=True),
            sa.Column("trusted", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("banned", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("risk_level", sa.String(20), nullable=False, server_default="normal"),
            sa.Column("notes", sa.Text(), nullable=True),
            sa.Column("playback_count", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("first_seen_at", sa.DateTime(), nullable=False),
            sa.Column("last_seen_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("device_key"),
        )
        op.create_index("ix_known_devices_user_seen", "known_devices", ["emby_user_id", "last_seen_at"])
        op.create_index("ix_known_devices_risk_seen", "known_devices", ["risk_level", "last_seen_at"])

    op.execute(
        """
        UPDATE web_roles SET permissions_json =
        '["users:*","codes:*","partitions:*","tasks:*","audit:read","security:read","settings:read","roles:read","dashboard:read","playback:*","devices:*","lines:*"]'
        WHERE name = 'admin'
        """
    )
    op.execute(
        """
        UPDATE web_roles SET permissions_json =
        '["users:read","users:update","codes:*","partitions:*","tasks:read","dashboard:read","playback:read","playback:stop","devices:read","devices:update","lines:read"]'
        WHERE name = 'operator'
        """
    )
    op.execute(
        """
        UPDATE web_roles SET permissions_json =
        '["users:read","tasks:read","audit:read","security:read","dashboard:read","playback:read","devices:read","lines:read"]'
        WHERE name = 'auditor'
        """
    )


def downgrade() -> None:
    for table_name in (
        "known_devices",
        "playback_sessions",
        "line_health_samples",
        "line_endpoints",
    ):
        if table_name in _tables():
            op.drop_table(table_name)
