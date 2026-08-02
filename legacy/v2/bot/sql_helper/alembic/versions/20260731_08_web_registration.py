"""web registration verification and reservation state

Revision ID: 20260731_08
Revises: 20260731_07
Create Date: 2026-07-31 23:30:00
"""

from alembic import op
import sqlalchemy as sa
from datetime import datetime


revision = "20260731_08"
down_revision = "20260731_07"
branch_labels = None
depends_on = None


def _columns(table_name: str):
    inspector = sa.inspect(op.get_bind())
    if table_name not in inspector.get_table_names():
        return set()
    return {column["name"] for column in inspector.get_columns(table_name)}


def upgrade() -> None:
    columns = _columns("web_login_requests")
    if columns and "purpose" not in columns:
        op.add_column(
            "web_login_requests",
            sa.Column(
                "purpose",
                sa.String(32),
                nullable=False,
                server_default="login",
            ),
        )
    session_columns = _columns("web_sessions")
    if session_columns and "purpose" not in session_columns:
        op.add_column(
            "web_sessions",
            sa.Column(
                "purpose",
                sa.String(32),
                nullable=False,
                server_default="login",
            ),
        )
    inspector = sa.inspect(op.get_bind())
    if "registration_state" not in inspector.get_table_names():
        table = op.create_table(
            "registration_state",
            sa.Column("id", sa.Integer(), autoincrement=False, nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.bulk_insert(table, [{"id": 1, "updated_at": datetime.utcnow()}])


def downgrade() -> None:
    inspector = sa.inspect(op.get_bind())
    if "registration_state" in inspector.get_table_names():
        op.drop_table("registration_state")
    columns = _columns("web_login_requests")
    if "purpose" in columns:
        op.drop_column("web_login_requests", "purpose")
    session_columns = _columns("web_sessions")
    if "purpose" in session_columns:
        op.drop_column("web_sessions", "purpose")
