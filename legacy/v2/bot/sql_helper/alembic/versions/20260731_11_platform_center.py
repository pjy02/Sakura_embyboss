"""device rules, media catalog, integrations, automation and open API

Revision ID: 20260731_11
Revises: 20260731_10
Create Date: 2026-07-31 23:59:30
"""

from datetime import datetime
import json

from alembic import op
import sqlalchemy as sa


revision = "20260731_11"
down_revision = "20260731_10"
branch_labels = None
depends_on = None


BUILT_IN_CLIENTS = (
    "Emby for iOS", "Emby for Android", "Emby Theater", "Emby for macOS",
    "Emby for Apple TV", "Infuse-Direct", "SenPlayer", "AfuseKt", "Conflux",
    "Yamby", "Xfuse", "Terminus Player", "Reflix", "Forward", "Hills",
    "Tsukimi", "iPlay", "Filebox", "Emby Web", "Emby Windows", "Filebar",
)
BUILT_IN_BLOCKED = (
    ("命令行 curl", r".*curl.*"), ("命令行 wget", r".*wget.*"),
    ("Python 脚本", r".*python.*"), ("Spider", r".*spider.*"),
    ("Crawler", r".*crawler.*"), ("Scraper", r".*scraper.*"),
    ("Downloader", r".*downloader.*"), ("aria2", r".*aria2.*"),
    ("youtube-dl", r".*youtube-dl.*"), ("yt-dlp", r".*yt-dlp.*"),
    ("ffmpeg", r".*ffmpeg.*"), ("VLC", r".*vlc.*"),
)


def upgrade() -> None:
    now = datetime.utcnow()
    device_rules = op.create_table(
        "device_client_rules",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("pattern", sa.String(255), nullable=False),
        sa.Column("match_type", sa.String(20), nullable=False, server_default="contains"),
        sa.Column("action", sa.String(20), nullable=False, server_default="allow"),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("built_in", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("priority", sa.Integer(), nullable=False, server_default="100"),
        sa.Column("hit_count", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("notes", sa.String(500), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("name", name="uq_device_client_rules_name"),
    )
    op.create_index("ix_device_client_rules_enabled_priority", "device_client_rules", ["enabled", "priority"])
    op.bulk_insert(device_rules, [
        {"name": name, "pattern": pattern, "match_type": "regex", "action": "block", "enabled": True,
         "built_in": True, "priority": 10 + index, "hit_count": 0, "notes": "Sakura 内置高风险客户端", "revision": 1,
         "created_at": now, "updated_at": now}
        for index, (name, pattern) in enumerate(BUILT_IN_BLOCKED)
    ] + [
        {"name": name, "pattern": name, "match_type": "contains", "action": "allow", "enabled": True,
         "built_in": True, "priority": 100 + index, "hit_count": 0, "notes": "Sakura 内置兼容客户端", "revision": 1,
         "created_at": now, "updated_at": now}
        for index, name in enumerate(BUILT_IN_CLIENTS)
    ])

    op.create_table(
        "managed_credentials",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("provider", sa.String(64), nullable=False),
        sa.Column("credential_type", sa.String(32), nullable=False, server_default="api_token"),
        sa.Column("ciphertext", sa.Text(), nullable=False),
        sa.Column("fingerprint", sa.String(24), nullable=False),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("last_used_at", sa.DateTime(), nullable=True),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("provider", "name", name="uq_managed_credentials_provider_name"),
    )
    op.create_index("ix_managed_credentials_provider_active", "managed_credentials", ["provider", "active"])

    op.create_table(
        "emby_instances",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("base_url", sa.String(512), nullable=False),
        sa.Column("credential_id", sa.String(36), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("is_default", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("verify_tls", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("priority", sa.Integer(), nullable=False, server_default="100"),
        sa.Column("status", sa.String(20), nullable=False, server_default="unknown"),
        sa.Column("last_error", sa.String(500), nullable=True),
        sa.Column("last_latency_ms", sa.Integer(), nullable=True),
        sa.Column("last_checked_at", sa.DateTime(), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("name", name="uq_emby_instances_name"),
        sa.UniqueConstraint("base_url", name="uq_emby_instances_base_url"),
    )
    op.create_index("ix_emby_instances_enabled_priority", "emby_instances", ["enabled", "priority"])

    op.create_table(
        "account_emby_bindings",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("account_id", sa.String(36), nullable=False),
        sa.Column("instance_id", sa.String(36), nullable=False),
        sa.Column("emby_user_id", sa.String(128), nullable=False),
        sa.Column("emby_username", sa.String(255), nullable=False),
        sa.Column("status", sa.String(20), nullable=False, server_default="active"),
        sa.Column("is_primary", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("last_synced_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("instance_id", "emby_user_id", name="uq_binding_instance_user"),
        sa.UniqueConstraint("account_id", "instance_id", name="uq_binding_account_instance"),
    )
    op.create_index("ix_account_emby_bindings_account", "account_emby_bindings", ["account_id", "status"])

    op.create_table(
        "media_catalog_items",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("provider", sa.String(32), nullable=False, server_default="tmdb"),
        sa.Column("media_type", sa.String(20), nullable=False),
        sa.Column("provider_id", sa.String(64), nullable=False),
        sa.Column("title", sa.String(255), nullable=False),
        sa.Column("original_title", sa.String(255), nullable=True),
        sa.Column("year", sa.Integer(), nullable=True),
        sa.Column("overview", sa.Text(), nullable=True),
        sa.Column("poster_path", sa.String(512), nullable=True),
        sa.Column("backdrop_path", sa.String(512), nullable=True),
        sa.Column("vote_average", sa.String(16), nullable=True),
        sa.Column("payload_json", sa.Text(), nullable=True),
        sa.Column("cached_until", sa.DateTime(), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("provider", "media_type", "provider_id", name="uq_media_catalog_provider_item"),
    )
    op.create_index("ix_media_catalog_title_year", "media_catalog_items", ["title", "year"])
    op.create_index("ix_media_catalog_cached_until", "media_catalog_items", ["cached_until"])

    op.create_table(
        "automation_rules",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("description", sa.String(500), nullable=True),
        sa.Column("trigger_type", sa.String(32), nullable=False, server_default="event"),
        sa.Column("trigger_value", sa.String(255), nullable=False),
        sa.Column("conditions_json", sa.Text(), nullable=True),
        sa.Column("actions_json", sa.Text(), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("cooldown_seconds", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("last_cursor", sa.BigInteger(), nullable=False, server_default="0"),
        sa.Column("last_run_at", sa.DateTime(), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_automation_rules_trigger_enabled", "automation_rules", ["trigger_type", "enabled"])

    op.create_table(
        "automation_runs",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("rule_id", sa.String(36), nullable=False),
        sa.Column("event_id", sa.BigInteger(), nullable=True),
        sa.Column("status", sa.String(20), nullable=False, server_default="running"),
        sa.Column("action_results_json", sa.Text(), nullable=True),
        sa.Column("error_message", sa.String(1000), nullable=True),
        sa.Column("started_at", sa.DateTime(), nullable=False),
        sa.Column("finished_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("rule_id", "event_id", name="uq_automation_run_rule_event"),
    )
    op.create_index("ix_automation_runs_rule_created", "automation_runs", ["rule_id", "created_at"])

    op.create_table(
        "api_clients",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("key_prefix", sa.String(16), nullable=False),
        sa.Column("key_hash", sa.String(128), nullable=False),
        sa.Column("scopes_json", sa.Text(), nullable=False),
        sa.Column("active", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("last_used_at", sa.DateTime(), nullable=True),
        sa.Column("last_ip", sa.String(64), nullable=True),
        sa.Column("created_by", sa.String(128), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.UniqueConstraint("key_prefix", name="uq_api_clients_key_prefix"),
    )
    op.create_index("ix_api_clients_active_expires", "api_clients", ["active", "expires_at"])

    role_additions = {
        "admin": ["integrations:*", "automation:*", "api:*", "backups:*", "media:*"],
        "operator": ["integrations:read", "integrations:manage", "automation:read", "automation:manage", "api:read", "backups:read", "media:read"],
        "auditor": ["integrations:read", "automation:read", "api:read", "backups:read", "media:read"],
    }
    for role_name, additions in role_additions.items():
        role = op.get_bind().execute(
            sa.text("SELECT id, permissions_json FROM web_roles WHERE name=:name"),
            {"name": role_name},
        ).mappings().first()
        if role:
            try:
                current = json.loads(role["permissions_json"] or "[]")
            except (TypeError, ValueError):
                current = []
            merged = list(dict.fromkeys([*current, *additions]))
            op.get_bind().execute(
                sa.text("UPDATE web_roles SET permissions_json=:permissions, updated_at=:updated WHERE id=:id"),
                {"permissions": json.dumps(merged), "updated": now, "id": role["id"]},
            )


def downgrade() -> None:
    for table in (
        "api_clients", "automation_runs", "automation_rules", "media_catalog_items",
        "account_emby_bindings", "emby_instances", "managed_credentials", "device_client_rules",
    ):
        op.drop_table(table)
