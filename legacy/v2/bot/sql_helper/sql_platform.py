"""Platform integration, media and automation models.

The Bot, Web API and standalone worker share these tables.  Secret values are
stored only as authenticated ciphertext; public API keys are stored as hashes.
"""

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


class DeviceClientRule(Base):
    __tablename__ = "device_client_rules"
    __table_args__ = (
        UniqueConstraint("name", name="uq_device_client_rules_name"),
        Index("ix_device_client_rules_enabled_priority", "enabled", "priority"),
    )

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(120), nullable=False)
    pattern = Column(String(255), nullable=False)
    match_type = Column(String(20), nullable=False, default="contains")
    action = Column(String(20), nullable=False, default="allow")
    enabled = Column(Boolean, nullable=False, default=True)
    built_in = Column(Boolean, nullable=False, default=False)
    priority = Column(Integer, nullable=False, default=100)
    hit_count = Column(Integer, nullable=False, default=0)
    notes = Column(String(500), nullable=True)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class ManagedCredential(Base):
    __tablename__ = "managed_credentials"
    __table_args__ = (
        UniqueConstraint("provider", "name", name="uq_managed_credentials_provider_name"),
        Index("ix_managed_credentials_provider_active", "provider", "active"),
    )

    id = Column(String(36), primary_key=True)
    name = Column(String(120), nullable=False)
    provider = Column(String(64), nullable=False)
    credential_type = Column(String(32), nullable=False, default="api_token")
    ciphertext = Column(Text, nullable=False)
    fingerprint = Column(String(24), nullable=False)
    metadata_json = Column(Text, nullable=True)
    active = Column(Boolean, nullable=False, default=True)
    last_used_at = Column(DateTime, nullable=True)
    expires_at = Column(DateTime, nullable=True)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class EmbyInstance(Base):
    __tablename__ = "emby_instances"
    __table_args__ = (
        UniqueConstraint("name", name="uq_emby_instances_name"),
        UniqueConstraint("base_url", name="uq_emby_instances_base_url"),
        Index("ix_emby_instances_enabled_priority", "enabled", "priority"),
    )

    id = Column(String(36), primary_key=True)
    name = Column(String(120), nullable=False)
    base_url = Column(String(512), nullable=False)
    credential_id = Column(String(36), nullable=False)
    enabled = Column(Boolean, nullable=False, default=True)
    is_default = Column(Boolean, nullable=False, default=False)
    verify_tls = Column(Boolean, nullable=False, default=True)
    priority = Column(Integer, nullable=False, default=100)
    status = Column(String(20), nullable=False, default="unknown")
    last_error = Column(String(500), nullable=True)
    last_latency_ms = Column(Integer, nullable=True)
    last_checked_at = Column(DateTime, nullable=True)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AccountEmbyBinding(Base):
    __tablename__ = "account_emby_bindings"
    __table_args__ = (
        UniqueConstraint("instance_id", "emby_user_id", name="uq_binding_instance_user"),
        UniqueConstraint("account_id", "instance_id", name="uq_binding_account_instance"),
        Index("ix_account_emby_bindings_account", "account_id", "status"),
    )

    id = Column(String(36), primary_key=True)
    account_id = Column(String(36), nullable=False)
    instance_id = Column(String(36), nullable=False)
    emby_user_id = Column(String(128), nullable=False)
    emby_username = Column(String(255), nullable=False)
    status = Column(String(20), nullable=False, default="active")
    is_primary = Column(Boolean, nullable=False, default=False)
    expires_at = Column(DateTime, nullable=True)
    last_synced_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class MediaCatalogItem(Base):
    __tablename__ = "media_catalog_items"
    __table_args__ = (
        UniqueConstraint("provider", "media_type", "provider_id", name="uq_media_catalog_provider_item"),
        Index("ix_media_catalog_title_year", "title", "year"),
        Index("ix_media_catalog_cached_until", "cached_until"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    provider = Column(String(32), nullable=False, default="tmdb")
    media_type = Column(String(20), nullable=False)
    provider_id = Column(String(64), nullable=False)
    title = Column(String(255), nullable=False)
    original_title = Column(String(255), nullable=True)
    year = Column(Integer, nullable=True)
    overview = Column(Text, nullable=True)
    poster_path = Column(String(512), nullable=True)
    backdrop_path = Column(String(512), nullable=True)
    vote_average = Column(String(16), nullable=True)
    payload_json = Column(Text, nullable=True)
    cached_until = Column(DateTime, nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AutomationRule(Base):
    __tablename__ = "automation_rules"
    __table_args__ = (Index("ix_automation_rules_trigger_enabled", "trigger_type", "enabled"),)

    id = Column(String(36), primary_key=True)
    name = Column(String(120), nullable=False)
    description = Column(String(500), nullable=True)
    trigger_type = Column(String(32), nullable=False, default="event")
    trigger_value = Column(String(255), nullable=False)
    conditions_json = Column(Text, nullable=True)
    actions_json = Column(Text, nullable=False)
    enabled = Column(Boolean, nullable=False, default=True)
    cooldown_seconds = Column(Integer, nullable=False, default=0)
    last_cursor = Column(BigInteger, nullable=False, default=0)
    last_run_at = Column(DateTime, nullable=True)
    revision = Column(Integer, nullable=False, default=1)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class AutomationRun(Base):
    __tablename__ = "automation_runs"
    __table_args__ = (
        UniqueConstraint("rule_id", "event_id", name="uq_automation_run_rule_event"),
        Index("ix_automation_runs_rule_created", "rule_id", "created_at"),
    )

    id = Column(String(36), primary_key=True)
    rule_id = Column(String(36), nullable=False)
    event_id = Column(BigInteger, nullable=True)
    status = Column(String(20), nullable=False, default="running")
    action_results_json = Column(Text, nullable=True)
    error_message = Column(String(1000), nullable=True)
    started_at = Column(DateTime, nullable=False, default=utcnow)
    finished_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class ApiClient(Base):
    __tablename__ = "api_clients"
    __table_args__ = (
        UniqueConstraint("key_prefix", name="uq_api_clients_key_prefix"),
        Index("ix_api_clients_active_expires", "active", "expires_at"),
    )

    id = Column(String(36), primary_key=True)
    name = Column(String(120), nullable=False)
    key_prefix = Column(String(16), nullable=False)
    key_hash = Column(String(128), nullable=False)
    scopes_json = Column(Text, nullable=False)
    active = Column(Boolean, nullable=False, default=True)
    expires_at = Column(DateTime, nullable=True)
    last_used_at = Column(DateTime, nullable=True)
    last_ip = Column(String(64), nullable=True)
    created_by = Column(String(128), nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)
