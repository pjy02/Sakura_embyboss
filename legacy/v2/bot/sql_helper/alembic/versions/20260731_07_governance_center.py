"""risk event lifecycle and governance permissions

Revision ID: 20260731_07
Revises: 20260731_06
Create Date: 2026-07-31 21:00:00
"""

import json

from alembic import op
import sqlalchemy as sa


revision = "20260731_07"
down_revision = "20260731_06"
branch_labels = None
depends_on = None


def _tables():
    return set(sa.inspect(op.get_bind()).get_table_names())


def _columns(table_name: str):
    return {
        column["name"]
        for column in sa.inspect(op.get_bind()).get_columns(table_name)
    }


def _add_permissions() -> None:
    if "web_roles" not in _tables():
        return
    additions = {
        "admin": ["security:manage", "settings:manage"],
        "operator": ["security:read"],
    }
    conn = op.get_bind()
    for role, permissions_to_add in additions.items():
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
        merged = list(dict.fromkeys([*current, *permissions_to_add]))
        conn.execute(
            sa.text(
                "UPDATE web_roles SET permissions_json = :permissions "
                "WHERE name = :role"
            ),
            {
                "permissions": json.dumps(merged, ensure_ascii=False),
                "role": role,
            },
        )


def upgrade() -> None:
    if "security_events" in _tables():
        columns = _columns("security_events")
        additions = (
            ("status", sa.Column("status", sa.String(20), nullable=False, server_default="open")),
            ("assigned_to", sa.Column("assigned_to", sa.BigInteger(), nullable=True)),
            ("resolution_note", sa.Column("resolution_note", sa.String(1000), nullable=True)),
            ("resolved_by", sa.Column("resolved_by", sa.BigInteger(), nullable=True)),
            ("resolved_at", sa.Column("resolved_at", sa.DateTime(), nullable=True)),
            ("updated_at", sa.Column("updated_at", sa.DateTime(), nullable=True)),
        )
        for name, column in additions:
            if name not in columns:
                op.add_column("security_events", column)
        op.execute(
            "UPDATE security_events SET updated_at = created_at "
            "WHERE updated_at IS NULL"
        )
        with op.batch_alter_table("security_events") as batch:
            batch.alter_column("updated_at", existing_type=sa.DateTime(), nullable=False)
        indexes = {
            item["name"]
            for item in sa.inspect(op.get_bind()).get_indexes("security_events")
        }
        if "ix_security_events_status_created" not in indexes:
            op.create_index(
                "ix_security_events_status_created",
                "security_events",
                ["status", "created_at"],
            )
    _add_permissions()


def downgrade() -> None:
    if "security_events" not in _tables():
        return
    indexes = {
        item["name"]
        for item in sa.inspect(op.get_bind()).get_indexes("security_events")
    }
    if "ix_security_events_status_created" in indexes:
        op.drop_index("ix_security_events_status_created", table_name="security_events")
    columns = _columns("security_events")
    for name in (
        "updated_at",
        "resolved_at",
        "resolved_by",
        "resolution_note",
        "assigned_to",
        "status",
    ):
        if name in columns:
            op.drop_column("security_events", name)
