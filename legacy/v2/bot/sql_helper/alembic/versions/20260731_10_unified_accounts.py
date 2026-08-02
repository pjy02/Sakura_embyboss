"""unified accounts, identities, memberships, tags and canonical ledger

Revision ID: 20260731_10
Revises: 20260731_09
Create Date: 2026-07-31 23:59:00
"""

from datetime import datetime
from uuid import NAMESPACE_URL, uuid5

from alembic import op
import sqlalchemy as sa


revision = "20260731_10"
down_revision = "20260731_09"
branch_labels = None
depends_on = None


def _columns(table: str) -> set[str]:
    return {item["name"] for item in sa.inspect(op.get_bind()).get_columns(table)}


def upgrade() -> None:
    bind = op.get_bind()
    inspector = sa.inspect(bind)
    tables = set(inspector.get_table_names())
    now = datetime.utcnow()

    if "accounts" not in tables:
        op.create_table(
            "accounts",
            sa.Column("id", sa.String(36), primary_key=True),
            sa.Column("legacy_tg", sa.BigInteger(), nullable=False),
            sa.Column("display_name", sa.String(255), nullable=True),
            sa.Column("status", sa.String(20), nullable=False, server_default="active"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("legacy_tg", name="uq_accounts_legacy_tg"),
        )
        op.create_index("ix_accounts_status_created", "accounts", ["status", "created_at"])

    if "account_identities" not in tables:
        op.create_table(
            "account_identities",
            sa.Column("id", sa.String(36), primary_key=True),
            sa.Column("account_id", sa.String(36), nullable=False),
            sa.Column("provider", sa.String(32), nullable=False),
            sa.Column("subject", sa.String(255), nullable=False),
            sa.Column("username", sa.String(255), nullable=True),
            sa.Column("username_normalized", sa.String(255), nullable=True),
            sa.Column("password_hash", sa.String(512), nullable=True),
            sa.Column("verified_at", sa.DateTime(), nullable=True),
            sa.Column("last_used_at", sa.DateTime(), nullable=True),
            sa.Column("disabled", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("provider", "subject", name="uq_account_identities_provider_subject"),
            sa.UniqueConstraint("provider", "username_normalized", name="uq_account_identities_provider_username"),
            sa.UniqueConstraint("account_id", "provider", name="uq_account_identities_account_provider"),
        )
        op.create_index("ix_account_identities_account", "account_identities", ["account_id", "provider"])

    if "membership_plans" not in tables:
        plans = op.create_table(
            "membership_plans",
            sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
            sa.Column("code", sa.String(64), nullable=False),
            sa.Column("name", sa.String(100), nullable=False),
            sa.Column("description", sa.String(1000), nullable=True),
            sa.Column("duration_days", sa.Integer(), nullable=False, server_default="30"),
            sa.Column("legacy_level", sa.String(1), nullable=False, server_default="b"),
            sa.Column("entitlements_json", sa.Text(), nullable=False),
            sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
            sa.Column("is_default", sa.Boolean(), nullable=False, server_default=sa.false()),
            sa.Column("sort_order", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("code", name="uq_membership_plans_code"),
        )
        op.bulk_insert(plans, [
            {"code": "standard", "name": "普通会员", "description": "兼容原普通用户等级", "duration_days": 30, "legacy_level": "b", "entitlements_json": '{"whitelist":false}', "enabled": True, "is_default": True, "sort_order": 10, "revision": 1, "created_at": now, "updated_at": now},
            {"code": "whitelist", "name": "白名单会员", "description": "兼容原白名单用户等级", "duration_days": 30, "legacy_level": "a", "entitlements_json": '{"whitelist":true}', "enabled": True, "is_default": False, "sort_order": 20, "revision": 1, "created_at": now, "updated_at": now},
        ])

    if "account_memberships" not in tables:
        op.create_table(
            "account_memberships",
            sa.Column("id", sa.String(36), primary_key=True),
            sa.Column("account_id", sa.String(36), nullable=False),
            sa.Column("plan_id", sa.Integer(), nullable=False),
            sa.Column("status", sa.String(20), nullable=False, server_default="active"),
            sa.Column("starts_at", sa.DateTime(), nullable=False),
            sa.Column("expires_at", sa.DateTime(), nullable=True),
            sa.Column("source", sa.String(32), nullable=False, server_default="migration"),
            sa.Column("created_by_kind", sa.String(32), nullable=False, server_default="system"),
            sa.Column("created_by_id", sa.String(128), nullable=False, server_default="migration"),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
        )
        op.create_index("ix_account_memberships_account_status", "account_memberships", ["account_id", "status"])
        op.create_index("ix_account_memberships_expires", "account_memberships", ["status", "expires_at"])

    if "account_tags" not in tables:
        op.create_table(
            "account_tags",
            sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
            sa.Column("name", sa.String(64), nullable=False),
            sa.Column("color", sa.String(20), nullable=False, server_default="#8b7cf6"),
            sa.Column("description", sa.String(500), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("name", name="uq_account_tags_name"),
        )

    if "account_tag_assignments" not in tables:
        op.create_table(
            "account_tag_assignments",
            sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
            sa.Column("account_id", sa.String(36), nullable=False),
            sa.Column("tag_id", sa.Integer(), nullable=False),
            sa.Column("assigned_by_kind", sa.String(32), nullable=False),
            sa.Column("assigned_by_id", sa.String(128), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("account_id", "tag_id", name="uq_account_tag_assignments"),
        )
        op.create_index("ix_account_tag_assignments_tag", "account_tag_assignments", ["tag_id", "account_id"])

    if "account_wallets" not in tables:
        op.create_table(
            "account_wallets",
            sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
            sa.Column("account_id", sa.String(36), nullable=False),
            sa.Column("balance_type", sa.String(32), nullable=False),
            sa.Column("balance", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("revision", sa.Integer(), nullable=False, server_default="1"),
            sa.Column("updated_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("account_id", "balance_type", name="uq_account_wallets_account_type"),
        )

    if "account_ledger_entries" not in tables:
        op.create_table(
            "account_ledger_entries",
            sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
            sa.Column("source_transaction_id", sa.BigInteger(), nullable=True),
            sa.Column("account_id", sa.String(36), nullable=False),
            sa.Column("legacy_tg", sa.BigInteger(), nullable=True),
            sa.Column("balance_type", sa.String(32), nullable=False),
            sa.Column("amount", sa.Integer(), nullable=False),
            sa.Column("balance_after", sa.Integer(), nullable=False),
            sa.Column("reason", sa.String(255), nullable=False),
            sa.Column("scope", sa.String(100), nullable=False),
            sa.Column("idempotency_key", sa.String(128), nullable=True),
            sa.Column("actor_kind", sa.String(32), nullable=False),
            sa.Column("actor_id", sa.String(128), nullable=False),
            sa.Column("metadata_json", sa.Text(), nullable=True),
            sa.Column("created_at", sa.DateTime(), nullable=False),
            sa.UniqueConstraint("scope", "idempotency_key", name="uq_account_ledger_scope_key"),
            sa.UniqueConstraint("source_transaction_id", name="uq_account_ledger_source_transaction"),
        )
        op.create_index("ix_account_ledger_account_created", "account_ledger_entries", ["account_id", "created_at"])

    for table, columns in (
        ("web_sessions", [("account_id", sa.String(36))]),
        ("point_transactions", [("account_id", sa.String(36))]),
        ("account_lifecycle_events", [("account_id", sa.String(36))]),
        ("Rcode", [("issuer_account_id", sa.String(36)), ("used_by_account_id", sa.String(36)), ("reserved_by_account_id", sa.String(36)), ("reserved_until", sa.DateTime()), ("status", sa.String(20)), ("expires_at", sa.DateTime())]),
    ):
        existing = _columns(table)
        with op.batch_alter_table(table) as batch:
            for name, kind in columns:
                if name not in existing:
                    batch.add_column(sa.Column(name, kind, nullable=True))

    # Deterministic IDs make the migration safe to reason about and repeat in tests.
    emby_rows = bind.execute(sa.text("SELECT tg, name, lv, cr, ex, iv, us, embyid FROM emby")).mappings().all()
    plan_rows = bind.execute(sa.text("SELECT id, code FROM membership_plans")).mappings().all()
    plan_ids = {row["code"]: row["id"] for row in plan_rows}
    for row in emby_rows:
        tg = int(row["tg"])
        account_id = str(uuid5(NAMESPACE_URL, f"sakura-account:{tg}"))
        exists = bind.execute(sa.text("SELECT id FROM accounts WHERE id=:id"), {"id": account_id}).first()
        if not exists:
            bind.execute(sa.text("INSERT INTO accounts (id, legacy_tg, display_name, status, created_at, updated_at) VALUES (:id,:tg,:name,:status,:created,:updated)"), {"id": account_id, "tg": tg, "name": row["name"] or f"TG {tg}", "status": "suspended" if row["lv"] == "c" else "active", "created": row["cr"] or now, "updated": now})
            bind.execute(sa.text("INSERT INTO account_identities (id, account_id, provider, subject, username, username_normalized, password_hash, verified_at, last_used_at, disabled, metadata_json, created_at, updated_at) VALUES (:identity_id,:account_id,'telegram',:subject,NULL,NULL,NULL,:verified,NULL,0,NULL,:created,:updated)"), {"identity_id": str(uuid5(NAMESPACE_URL, f"sakura-identity:telegram:{tg}")), "account_id": account_id, "subject": str(tg), "verified": row["cr"] or now, "created": row["cr"] or now, "updated": now})
            for balance_type, balance in (("coins", int(row["iv"] or 0)), ("registration_days", int(row["us"] or 0))):
                bind.execute(sa.text("INSERT INTO account_wallets (account_id, balance_type, balance, revision, updated_at) VALUES (:account_id,:balance_type,:balance,1,:updated)"), {"account_id": account_id, "balance_type": balance_type, "balance": balance, "updated": now})
            if row["embyid"]:
                plan_code = "whitelist" if row["lv"] == "a" else "standard"
                bind.execute(sa.text("INSERT INTO account_memberships (id, account_id, plan_id, status, starts_at, expires_at, source, created_by_kind, created_by_id, created_at, updated_at) VALUES (:id,:account_id,:plan_id,:status,:starts,:expires,'migration','system','migration',:created,:updated)"), {"id": str(uuid5(NAMESPACE_URL, f"sakura-membership:{tg}")), "account_id": account_id, "plan_id": plan_ids[plan_code], "status": "suspended" if row["lv"] == "c" else "active", "starts": row["cr"] or now, "expires": row["ex"], "created": now, "updated": now})

        bind.execute(sa.text("UPDATE web_sessions SET account_id=:account_id WHERE tg=:tg AND account_id IS NULL"), {"account_id": account_id, "tg": tg})
        bind.execute(sa.text("UPDATE point_transactions SET account_id=:account_id WHERE tg=:tg AND account_id IS NULL"), {"account_id": account_id, "tg": tg})
        bind.execute(sa.text("UPDATE account_lifecycle_events SET account_id=:account_id WHERE tg=:tg AND account_id IS NULL"), {"account_id": account_id, "tg": tg})
        bind.execute(sa.text("UPDATE Rcode SET issuer_account_id=:account_id WHERE tg=:tg AND issuer_account_id IS NULL"), {"account_id": account_id, "tg": tg})
        bind.execute(sa.text("UPDATE Rcode SET used_by_account_id=:account_id WHERE used=:tg AND used_by_account_id IS NULL"), {"account_id": account_id, "tg": tg})

    # Preserve the full historical points trail in the canonical ledger. New
    # writes are dual-written by OperationRepository after this migration.
    transaction_rows = bind.execute(sa.text(
        "SELECT id, account_id, tg, balance_type, amount, balance_after, reason, "
        "actor_kind, actor_id, idempotency_key, metadata_json, created_at "
        "FROM point_transactions WHERE account_id IS NOT NULL"
    )).mappings().all()
    existing_sources = {
        int(item[0])
        for item in bind.execute(sa.text(
            "SELECT source_transaction_id FROM account_ledger_entries "
            "WHERE source_transaction_id IS NOT NULL"
        )).all()
    }
    existing_keys = {
        (str(item[0]), str(item[1]))
        for item in bind.execute(sa.text(
            "SELECT scope, idempotency_key FROM account_ledger_entries "
            "WHERE idempotency_key IS NOT NULL"
        )).all()
    }
    for row in transaction_rows:
        if int(row["id"]) in existing_sources:
            continue
        scope = f"points.{row['balance_type']}"
        ledger_key = row["idempotency_key"]
        if ledger_key is not None and (scope, str(ledger_key)) in existing_keys:
            ledger_key = f"{str(ledger_key)[:96]}:legacy:{row['id']}"
        if ledger_key is not None:
            existing_keys.add((scope, str(ledger_key)))
        bind.execute(sa.text(
            "INSERT INTO account_ledger_entries "
            "(source_transaction_id, account_id, legacy_tg, balance_type, amount, balance_after, reason, scope, "
            "idempotency_key, actor_kind, actor_id, metadata_json, created_at) "
            "VALUES (:source_transaction_id, :account_id, :legacy_tg, :balance_type, :amount, :balance_after, "
            ":reason, :scope, :idempotency_key, :actor_kind, :actor_id, :metadata_json, :created_at)"
        ), {
            "source_transaction_id": row["id"],
            "account_id": row["account_id"],
            "legacy_tg": row["tg"],
            "balance_type": row["balance_type"],
            "amount": row["amount"],
            "balance_after": row["balance_after"],
            "reason": row["reason"],
            "scope": scope,
            "idempotency_key": ledger_key,
            "actor_kind": row["actor_kind"],
            "actor_id": row["actor_id"],
            "metadata_json": row["metadata_json"],
            "created_at": row["created_at"],
        })


def downgrade() -> None:
    tables = set(sa.inspect(op.get_bind()).get_table_names())
    for table, columns in (
        ("Rcode", ["expires_at", "status", "reserved_until", "reserved_by_account_id", "used_by_account_id", "issuer_account_id"]),
        ("account_lifecycle_events", ["account_id"]),
        ("point_transactions", ["account_id"]),
        ("web_sessions", ["account_id"]),
    ):
        if table in tables:
            existing = _columns(table)
            with op.batch_alter_table(table) as batch:
                for column in columns:
                    if column in existing:
                        batch.drop_column(column)
    for table in ("account_ledger_entries", "account_wallets", "account_tag_assignments", "account_tags", "account_memberships", "membership_plans", "account_identities", "accounts"):
        if table in tables:
            op.drop_table(table)
