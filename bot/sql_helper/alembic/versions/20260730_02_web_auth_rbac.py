"""web authentication and RBAC additions

Revision ID: 20260730_02
Revises: 20260730_01
Create Date: 2026-07-30 15:00:00
"""

from alembic import op
import sqlalchemy as sa

revision = "20260730_02"
down_revision = "20260730_01"
branch_labels = None
depends_on = None


def _columns(table_name: str):
    return {item["name"] for item in sa.inspect(op.get_bind()).get_columns(table_name)}


def upgrade() -> None:
    session_columns = _columns("web_sessions")
    if "auth_method" not in session_columns:
        op.add_column(
            "web_sessions",
            sa.Column(
                "auth_method",
                sa.String(32),
                nullable=False,
                server_default="telegram",
            ),
        )

    request_columns = _columns("web_login_requests")
    if "claimed_at" not in request_columns:
        op.add_column(
            "web_login_requests",
            sa.Column("claimed_at", sa.DateTime(), nullable=True),
        )

    op.execute(
        """
        UPDATE web_roles
        SET permissions_json = '["users:*","codes:*","partitions:*","tasks:*","audit:read","security:read","settings:read","roles:read"]'
        WHERE name = 'admin'
        """
    )
    op.execute(
        """
        UPDATE web_roles
        SET permissions_json = '["users:read","users:update","codes:*","partitions:*","tasks:read"]'
        WHERE name = 'operator'
        """
    )
    op.execute(
        """
        UPDATE web_roles
        SET permissions_json = '["users:read","tasks:read","audit:read","security:read"]'
        WHERE name = 'auditor'
        """
    )


def downgrade() -> None:
    request_columns = _columns("web_login_requests")
    if "claimed_at" in request_columns:
        op.drop_column("web_login_requests", "claimed_at")

    session_columns = _columns("web_sessions")
    if "auth_method" in session_columns:
        op.drop_column("web_sessions", "auth_method")
