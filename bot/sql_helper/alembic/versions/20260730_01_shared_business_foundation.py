"""shared business layer data foundation

Revision ID: 20260730_01
Revises: 20260315_02
Create Date: 2026-07-30 12:00:00
"""

from alembic import op
import sqlalchemy as sa

revision = "20260730_01"
down_revision = "20260315_02"
branch_labels = None
depends_on = None


def _table_names():
    return set(sa.inspect(op.get_bind()).get_table_names())


def _column_names(table_name: str):
    return {item["name"] for item in sa.inspect(op.get_bind()).get_columns(table_name)}


def upgrade() -> None:
    tables = _table_names()

    if "idempotency_records" not in tables:
        op.create_table(
            "idempotency_records",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("scope", sa.String(100), nullable=False),
            sa.Column("idempotency_key", sa.String(128), nullable=False),
            sa.Column("result_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("expires_at", sa.DateTime(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("scope", "idempotency_key", name="uq_idempotency_scope_key"),
        )
        op.create_index("ix_idempotency_expires_at", "idempotency_records", ["expires_at"])

    if "audit_logs" not in tables:
        op.create_table(
            "audit_logs",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("request_id", sa.String(64), nullable=True),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("actor_name", sa.String(255), nullable=True),
            sa.Column("action", sa.String(100), nullable=False),
            sa.Column("resource_type", sa.String(64), nullable=False),
            sa.Column("resource_id", sa.String(255), nullable=True),
            sa.Column("outcome", sa.String(32), nullable=False, server_default="success"),
            sa.Column("detail_json", sa.Text(), nullable=True),
            sa.Column("ip_address", sa.String(64), nullable=True),
            sa.Column("user_agent", sa.String(512), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_audit_logs_actor", "audit_logs", ["actor_kind", "actor_id"])
        op.create_index("ix_audit_logs_resource", "audit_logs", ["resource_type", "resource_id"])
        op.create_index("ix_audit_logs_created_at", "audit_logs", ["created_at"])

    if "point_transactions" not in tables:
        op.create_table(
            "point_transactions",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("balance_type", sa.String(32), nullable=False),
            sa.Column("amount", sa.Integer(), nullable=False),
            sa.Column("balance_after", sa.Integer(), nullable=False),
            sa.Column("reason", sa.String(255), nullable=False),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("idempotency_key", sa.String(128), nullable=True),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_point_transactions_tg_created", "point_transactions", ["tg", "created_at"])
        op.create_index("ix_point_transactions_actor", "point_transactions", ["actor_kind", "actor_id"])

    if "operation_tasks" not in tables:
        op.create_table(
            "operation_tasks",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("task_type", sa.String(100), nullable=False),
            sa.Column("status", sa.String(32), nullable=False, server_default="pending"),
            sa.Column("progress", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("owner_kind", sa.String(32), nullable=False),
            sa.Column("owner_id", sa.String(128), nullable=False),
            sa.Column("idempotency_key", sa.String(128), nullable=True),
            sa.Column("input_json", sa.Text(), nullable=True),
            sa.Column("result_json", sa.Text(), nullable=True),
            sa.Column("error_message", sa.Text(), nullable=True),
            sa.Column("retry_count", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("started_at", sa.DateTime(), nullable=True),
            sa.Column("finished_at", sa.DateTime(), nullable=True),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("idempotency_key", name="uq_operation_tasks_idempotency_key"),
        )
        op.create_index("ix_operation_tasks_status_created", "operation_tasks", ["status", "created_at"])
        op.create_index("ix_operation_tasks_owner", "operation_tasks", ["owner_kind", "owner_id"])

    if "system_events" not in tables:
        op.create_table(
            "system_events",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("event_type", sa.String(100), nullable=False),
            sa.Column("aggregate_type", sa.String(64), nullable=False),
            sa.Column("aggregate_id", sa.String(255), nullable=True),
            sa.Column("payload_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("published_at", sa.DateTime(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_system_events_publish", "system_events", ["published_at", "created_at"])
        op.create_index("ix_system_events_type", "system_events", ["event_type"])

    if "job_runs" not in tables:
        op.create_table(
            "job_runs",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("job_name", sa.String(100), nullable=False),
            sa.Column("trigger_kind", sa.String(32), nullable=False, server_default="scheduler"),
            sa.Column("status", sa.String(32), nullable=False, server_default="running"),
            sa.Column("summary_json", sa.Text(), nullable=True),
            sa.Column("error_message", sa.Text(), nullable=True),
            sa.Column("started_at", sa.DateTime(), nullable=False),
            sa.Column("finished_at", sa.DateTime(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_job_runs_job_started", "job_runs", ["job_name", "started_at"])

    if "security_events" not in tables:
        op.create_table(
            "security_events",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("event_type", sa.String(100), nullable=False),
            sa.Column("severity", sa.String(20), nullable=False, server_default="info"),
            sa.Column("subject_kind", sa.String(32), nullable=True),
            sa.Column("subject_id", sa.String(128), nullable=True),
            sa.Column("ip_address", sa.String(64), nullable=True),
            sa.Column("detail_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_security_events_type_created", "security_events", ["event_type", "created_at"])
        op.create_index("ix_security_events_subject", "security_events", ["subject_kind", "subject_id"])

    if "dynamic_settings" not in tables:
        op.create_table(
            "dynamic_settings",
            sa.Column("setting_key", sa.String(128), nullable=False),
            sa.Column("value_json", sa.Text(), nullable=False),
            sa.Column("value_type", sa.String(32), nullable=False, server_default="json"),
            sa.Column("is_secret", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("updated_by_kind", sa.String(32), nullable=False),
            sa.Column("updated_by_id", sa.String(128), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("setting_key"),
        )

    if "config_revisions" not in tables:
        op.create_table(
            "config_revisions",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("setting_key", sa.String(128), nullable=False),
            sa.Column("revision", sa.Integer(), nullable=False),
            sa.Column("old_value_json", sa.Text(), nullable=True),
            sa.Column("new_value_json", sa.Text(), nullable=False),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_config_revisions_key_revision", "config_revisions", ["setting_key", "revision"])

    if "web_sessions" not in tables:
        op.create_table(
            "web_sessions",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("token_hash", sa.String(128), nullable=False),
            sa.Column("csrf_hash", sa.String(128), nullable=False),
            sa.Column("ip_address", sa.String(64), nullable=True),
            sa.Column("user_agent", sa.String(512), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("last_seen_at", sa.DateTime(), nullable=False),
            sa.Column("expires_at", sa.DateTime(), nullable=False),
            sa.Column("revoked_at", sa.DateTime(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("token_hash", name="uq_web_sessions_token_hash"),
        )
        op.create_index("ix_web_sessions_tg_expires", "web_sessions", ["tg", "expires_at"])

    if "web_login_requests" not in tables:
        op.create_table(
            "web_login_requests",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("request_token_hash", sa.String(128), nullable=False),
            sa.Column("status", sa.String(32), nullable=False, server_default="pending"),
            sa.Column("requested_tg", sa.BigInteger(), nullable=True),
            sa.Column("approved_tg", sa.BigInteger(), nullable=True),
            sa.Column("ip_address", sa.String(64), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("expires_at", sa.DateTime(), nullable=False),
            sa.Column("approved_at", sa.DateTime(), nullable=True),
            sa.Column("consumed_at", sa.DateTime(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("request_token_hash", name="uq_web_login_requests_token"),
        )
        op.create_index("ix_web_login_requests_status_expires", "web_login_requests", ["status", "expires_at"])

    if "web_roles" not in tables:
        op.create_table(
            "web_roles",
            sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
            sa.Column("name", sa.String(64), nullable=False),
            sa.Column("permissions_json", sa.Text(), nullable=False),
            sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("name"),
        )

    if "web_role_members" not in tables:
        op.create_table(
            "web_role_members",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("role_id", sa.Integer(), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("created_by", sa.BigInteger(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("role_id", "tg", name="uq_web_role_members_role_tg"),
        )
        op.create_index("ix_web_role_members_tg", "web_role_members", ["tg"])

    partition_columns = _column_names("partition_codes")
    if "status" not in partition_columns:
        op.add_column(
            "partition_codes",
            sa.Column("status", sa.String(20), nullable=False, server_default="available"),
        )
        op.create_index("ix_partition_codes_status", "partition_codes", ["status"])
    if "reserved_by" not in partition_columns:
        op.add_column("partition_codes", sa.Column("reserved_by", sa.BigInteger(), nullable=True))
    if "reserved_at" not in partition_columns:
        op.add_column("partition_codes", sa.Column("reserved_at", sa.DateTime(), nullable=True))
    if "reservation_token" not in partition_columns:
        op.add_column("partition_codes", sa.Column("reservation_token", sa.String(36), nullable=True))
        op.create_unique_constraint(
            "uq_partition_codes_reservation_token",
            "partition_codes",
            ["reservation_token"],
        )

    op.execute(
        """
        INSERT IGNORE INTO web_roles
            (name, permissions_json, is_system, created_at, updated_at)
        VALUES
            ('owner', '["*"]', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
            ('admin', '["users:*","codes:*","partitions:*","tasks:*","audit:read","settings:read"]', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
            ('operator', '["users:read","users:update","codes:*","partitions:*","tasks:read"]', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
            ('auditor', '["users:read","tasks:read","audit:read","security:read"]', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
            ('user', '["self:*"]', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP())
        """
    )


def downgrade() -> None:
    partition_columns = _column_names("partition_codes")
    if "reservation_token" in partition_columns:
        op.drop_constraint("uq_partition_codes_reservation_token", "partition_codes", type_="unique")
        op.drop_column("partition_codes", "reservation_token")
    if "reserved_at" in partition_columns:
        op.drop_column("partition_codes", "reserved_at")
    if "reserved_by" in partition_columns:
        op.drop_column("partition_codes", "reserved_by")
    if "status" in partition_columns:
        op.drop_index("ix_partition_codes_status", table_name="partition_codes")
        op.drop_column("partition_codes", "status")

    for table_name in (
        "web_role_members",
        "web_roles",
        "web_login_requests",
        "web_sessions",
        "config_revisions",
        "dynamic_settings",
        "security_events",
        "job_runs",
        "system_events",
        "operation_tasks",
        "point_transactions",
        "audit_logs",
        "idempotency_records",
    ):
        if table_name in _table_names():
            op.drop_table(table_name)
