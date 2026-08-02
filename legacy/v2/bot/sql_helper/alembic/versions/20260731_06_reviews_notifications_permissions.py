"""reviews, notifications and permission extensions

Revision ID: 20260731_06
Revises: 20260731_05
Create Date: 2026-07-31 18:00:00
"""

import json

from alembic import op
import sqlalchemy as sa


revision = "20260731_06"
down_revision = "20260731_05"
branch_labels = None
depends_on = None


def _tables():
    return set(sa.inspect(op.get_bind()).get_table_names())


def upgrade() -> None:
    tables = _tables()
    if "media_reviews" not in tables:
        op.create_table(
            "media_reviews",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("media_key", sa.String(255), nullable=False),
            sa.Column("media_title", sa.String(255), nullable=False),
            sa.Column("media_year", sa.Integer(), nullable=True),
            sa.Column("rating", sa.Integer(), nullable=False),
            sa.Column("content", sa.Text(), nullable=False),
            sa.Column("spoiler", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("status", sa.String(32), nullable=False, server_default="pending"),
            sa.Column("like_count", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("report_count", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("admin_note", sa.String(1000), nullable=True),
            sa.Column("moderated_by", sa.BigInteger(), nullable=True),
            sa.Column("moderated_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("tg", "media_key", name="uq_media_reviews_user_media"),
        )
        op.create_index("ix_media_reviews_status_created", "media_reviews", ["status", "created_at"])
        op.create_index("ix_media_reviews_media", "media_reviews", ["media_key", "status"])
        op.create_index("ix_media_reviews_tg_created", "media_reviews", ["tg", "created_at"])

    if "review_reactions" not in tables:
        op.create_table(
            "review_reactions",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("review_id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("review_id", "tg", name="uq_review_reactions_review_tg"),
        )
        op.create_index("ix_review_reactions_tg", "review_reactions", ["tg", "created_at"])

    if "review_reports" not in tables:
        op.create_table(
            "review_reports",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("review_id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("reason", sa.String(32), nullable=False),
            sa.Column("detail", sa.String(500), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("review_id", "tg", name="uq_review_reports_review_tg"),
        )
        op.create_index("ix_review_reports_review", "review_reports", ["review_id", "created_at"])

    if "user_notifications" not in tables:
        op.create_table(
            "user_notifications",
            sa.Column("id", sa.String(36), nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("category", sa.String(32), nullable=False),
            sa.Column("title", sa.String(200), nullable=False),
            sa.Column("body", sa.String(2000), nullable=False),
            sa.Column("severity", sa.String(20), nullable=False, server_default="info"),
            sa.Column("action_url", sa.String(500), nullable=True),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("read_at", sa.DateTime(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_user_notifications_tg_created", "user_notifications", ["tg", "created_at"])
        op.create_index("ix_user_notifications_tg_read", "user_notifications", ["tg", "read_at"])
        op.create_index("ix_user_notifications_category", "user_notifications", ["category", "created_at"])

    if "notification_preferences" not in tables:
        op.create_table(
            "notification_preferences",
            sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
            sa.Column("tg", sa.BigInteger(), nullable=False),
            sa.Column("category", sa.String(32), nullable=False),
            sa.Column("web_enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("telegram_enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("tg", "category", name="uq_notification_preferences_tg_category"),
        )
        op.create_index("ix_notification_preferences_tg", "notification_preferences", ["tg"])

    if "web_roles" in tables:
        additions = {
            "admin": ["reviews:*", "notifications:*", "roles:manage", "audit:export"],
            "operator": ["reviews:*", "notifications:read", "notifications:send"],
            "auditor": ["reviews:read", "notifications:read"],
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
                sa.text("UPDATE web_roles SET permissions_json = :permissions WHERE name = :role"),
                {
                    "permissions": json.dumps(merged, ensure_ascii=False),
                    "role": role,
                },
            )


def downgrade() -> None:
    for table_name in (
        "notification_preferences",
        "user_notifications",
        "review_reports",
        "review_reactions",
        "media_reviews",
    ):
        if table_name in _tables():
            op.drop_table(table_name)
