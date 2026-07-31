"""Canonical account, identity, membership, tag and wallet models.

The legacy ``emby.tg`` record remains as a compatibility projection for Bot
handlers.  New Web and worker code owns users through ``Account.id``.
"""

from uuid import uuid4

from sqlalchemy import (
    BigInteger,
    Boolean,
    Column,
    DateTime,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)

from bot.sql_helper import Base
from bot.sql_helper.sql_application import utcnow


class Account(Base):
    __tablename__ = "accounts"
    __table_args__ = (
        UniqueConstraint("legacy_tg", name="uq_accounts_legacy_tg"),
        Index("ix_accounts_status_created", "status", "created_at"),
    )

    id = Column(String(36), primary_key=True, default=lambda: str(uuid4()))
    legacy_tg = Column(BigInteger, nullable=False)
    display_name = Column(String(255), nullable=True)
    status = Column(String(20), nullable=False, default="active")
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountIdentity(Base):
    __tablename__ = "account_identities"
    __table_args__ = (
        UniqueConstraint("provider", "subject", name="uq_account_identities_provider_subject"),
        UniqueConstraint("provider", "username_normalized", name="uq_account_identities_provider_username"),
        UniqueConstraint("account_id", "provider", name="uq_account_identities_account_provider"),
        Index("ix_account_identities_account", "account_id", "provider"),
    )

    id = Column(String(36), primary_key=True, default=lambda: str(uuid4()))
    account_id = Column(String(36), nullable=False)
    provider = Column(String(32), nullable=False)
    subject = Column(String(255), nullable=False)
    username = Column(String(255), nullable=True)
    username_normalized = Column(String(255), nullable=True)
    password_hash = Column(String(512), nullable=True)
    verified_at = Column(DateTime, nullable=True)
    last_used_at = Column(DateTime, nullable=True)
    disabled = Column(Boolean, nullable=False, default=False)
    metadata_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class MembershipPlan(Base):
    __tablename__ = "membership_plans"
    __table_args__ = (UniqueConstraint("code", name="uq_membership_plans_code"),)

    id = Column(Integer, primary_key=True, autoincrement=True)
    code = Column(String(64), nullable=False)
    name = Column(String(100), nullable=False)
    description = Column(String(1000), nullable=True)
    duration_days = Column(Integer, nullable=False, default=30)
    legacy_level = Column(String(1), nullable=False, default="b")
    entitlements_json = Column(Text, nullable=False, default="{}")
    enabled = Column(Boolean, nullable=False, default=True)
    is_default = Column(Boolean, nullable=False, default=False)
    sort_order = Column(Integer, nullable=False, default=0)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountMembership(Base):
    __tablename__ = "account_memberships"
    __table_args__ = (
        Index("ix_account_memberships_account_status", "account_id", "status"),
        Index("ix_account_memberships_expires", "status", "expires_at"),
    )

    id = Column(String(36), primary_key=True, default=lambda: str(uuid4()))
    account_id = Column(String(36), nullable=False)
    plan_id = Column(Integer, nullable=False)
    status = Column(String(20), nullable=False, default="active")
    starts_at = Column(DateTime, nullable=False, default=utcnow)
    expires_at = Column(DateTime, nullable=True)
    source = Column(String(32), nullable=False, default="migration")
    created_by_kind = Column(String(32), nullable=False, default="system")
    created_by_id = Column(String(128), nullable=False, default="migration")
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountTag(Base):
    __tablename__ = "account_tags"
    __table_args__ = (UniqueConstraint("name", name="uq_account_tags_name"),)

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(64), nullable=False)
    color = Column(String(20), nullable=False, default="#8b7cf6")
    description = Column(String(500), nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountTagAssignment(Base):
    __tablename__ = "account_tag_assignments"
    __table_args__ = (
        UniqueConstraint("account_id", "tag_id", name="uq_account_tag_assignments"),
        Index("ix_account_tag_assignments_tag", "tag_id", "account_id"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    account_id = Column(String(36), nullable=False)
    tag_id = Column(Integer, nullable=False)
    assigned_by_kind = Column(String(32), nullable=False)
    assigned_by_id = Column(String(128), nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class AccountWallet(Base):
    __tablename__ = "account_wallets"
    __table_args__ = (
        UniqueConstraint("account_id", "balance_type", name="uq_account_wallets_account_type"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    account_id = Column(String(36), nullable=False)
    balance_type = Column(String(32), nullable=False)
    balance = Column(Integer, nullable=False, default=0)
    revision = Column(Integer, nullable=False, default=1)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountLedgerEntry(Base):
    __tablename__ = "account_ledger_entries"
    __table_args__ = (
        Index("ix_account_ledger_account_created", "account_id", "created_at"),
        UniqueConstraint("scope", "idempotency_key", name="uq_account_ledger_scope_key"),
        UniqueConstraint("source_transaction_id", name="uq_account_ledger_source_transaction"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    source_transaction_id = Column(BigInteger, nullable=True)
    account_id = Column(String(36), nullable=False)
    legacy_tg = Column(BigInteger, nullable=True)
    balance_type = Column(String(32), nullable=False)
    amount = Column(Integer, nullable=False)
    balance_after = Column(Integer, nullable=False)
    reason = Column(String(255), nullable=False)
    scope = Column(String(100), nullable=False)
    idempotency_key = Column(String(128), nullable=True)
    actor_kind = Column(String(32), nullable=False)
    actor_id = Column(String(128), nullable=False)
    metadata_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
