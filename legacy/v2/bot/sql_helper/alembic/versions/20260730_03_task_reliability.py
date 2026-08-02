"""durable task worker and reliability metadata

Revision ID: 20260730_03
Revises: 20260730_02
Create Date: 2026-07-30 20:30:00
"""

from alembic import op
import sqlalchemy as sa


revision = "20260730_03"
down_revision = "20260730_02"
branch_labels = None
depends_on = None


def _tables():
    return set(sa.inspect(op.get_bind()).get_table_names())


def _columns(table_name: str):
    return {item["name"] for item in sa.inspect(op.get_bind()).get_columns(table_name)}


def _indexes(table_name: str):
    return {item["name"] for item in sa.inspect(op.get_bind()).get_indexes(table_name)}


def upgrade() -> None:
    columns = _columns("operation_tasks")
    additions = (
        ("max_retries", sa.Column("max_retries", sa.Integer(), nullable=False, server_default="3")),
        ("next_run_at", sa.Column("next_run_at", sa.DateTime(), nullable=True)),
        ("locked_by", sa.Column("locked_by", sa.String(128), nullable=True)),
        ("lease_expires_at", sa.Column("lease_expires_at", sa.DateTime(), nullable=True)),
        ("last_heartbeat_at", sa.Column("last_heartbeat_at", sa.DateTime(), nullable=True)),
        (
            "cancel_requested",
            sa.Column(
                "cancel_requested",
                sa.Boolean(),
                nullable=False,
                server_default=sa.false(),
            ),
        ),
    )
    for name, column in additions:
        if name not in columns:
            op.add_column("operation_tasks", column)

    op.execute(
        "UPDATE operation_tasks SET next_run_at = COALESCE(next_run_at, created_at)"
    )
    op.alter_column(
        "operation_tasks",
        "next_run_at",
        existing_type=sa.DateTime(),
        nullable=False,
    )
    if "ix_operation_tasks_next_run" not in _indexes("operation_tasks"):
        op.create_index(
            "ix_operation_tasks_next_run",
            "operation_tasks",
            ["status", "next_run_at"],
        )
    if "ix_operation_tasks_lease" not in _indexes("operation_tasks"):
        op.create_index(
            "ix_operation_tasks_lease",
            "operation_tasks",
            ["status", "lease_expires_at"],
        )

    if "worker_heartbeats" not in _tables():
        op.create_table(
            "worker_heartbeats",
            sa.Column("worker_id", sa.String(128), nullable=False),
            sa.Column("worker_kind", sa.String(64), nullable=False),
            sa.Column("hostname", sa.String(255), nullable=False),
            sa.Column("process_id", sa.Integer(), nullable=False),
            sa.Column("status", sa.String(32), nullable=False, server_default="starting"),
            sa.Column("current_task_id", sa.String(36), nullable=True),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("started_at", sa.DateTime(), nullable=False),
            sa.Column("last_seen_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("worker_id"),
        )
        op.create_index(
            "ix_worker_heartbeats_kind_seen",
            "worker_heartbeats",
            ["worker_kind", "last_seen_at"],
        )

def downgrade() -> None:
    if "worker_heartbeats" in _tables():
        op.drop_table("worker_heartbeats")

    indexes = _indexes("operation_tasks")
    if "ix_operation_tasks_lease" in indexes:
        op.drop_index("ix_operation_tasks_lease", table_name="operation_tasks")
    if "ix_operation_tasks_next_run" in indexes:
        op.drop_index("ix_operation_tasks_next_run", table_name="operation_tasks")

    columns = _columns("operation_tasks")
    for name in (
        "cancel_requested",
        "last_heartbeat_at",
        "lease_expires_at",
        "locked_by",
        "next_run_at",
        "max_retries",
    ):
        if name in columns:
            op.drop_column("operation_tasks", name)
