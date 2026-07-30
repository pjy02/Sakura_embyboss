"""Shared application infrastructure models.

These tables are intentionally independent from Telegram handlers.  They are
the durable foundation used by the Bot, Web API, workers and schedulers.
"""

from datetime import datetime, timezone

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


def utcnow() -> datetime:
    return datetime.now(timezone.utc).replace(tzinfo=None)


class IdempotencyRecord(Base):
    __tablename__ = "idempotency_records"
    __table_args__ = (
        UniqueConstraint("scope", "idempotency_key", name="uq_idempotency_scope_key"),
        Index("ix_idempotency_expires_at", "expires_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    scope = Column(String(100), nullable=False)
    idempotency_key = Column(String(128), nullable=False)
    result_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    expires_at = Column(DateTime, nullable=True)


class AuditLog(Base):
    __tablename__ = "audit_logs"
    __table_args__ = (
        Index("ix_audit_logs_actor", "actor_kind", "actor_id"),
        Index("ix_audit_logs_resource", "resource_type", "resource_id"),
        Index("ix_audit_logs_created_at", "created_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    request_id = Column(String(64), nullable=True)
    actor_kind = Column(String(32), nullable=False)
    actor_id = Column(String(128), nullable=False)
    actor_name = Column(String(255), nullable=True)
    action = Column(String(100), nullable=False)
    resource_type = Column(String(64), nullable=False)
    resource_id = Column(String(255), nullable=True)
    outcome = Column(String(32), nullable=False, default="success")
    detail_json = Column(Text, nullable=True)
    ip_address = Column(String(64), nullable=True)
    user_agent = Column(String(512), nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class PointTransaction(Base):
    __tablename__ = "point_transactions"
    __table_args__ = (
        Index("ix_point_transactions_tg_created", "tg", "created_at"),
        Index("ix_point_transactions_actor", "actor_kind", "actor_id"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    tg = Column(BigInteger, nullable=False)
    balance_type = Column(String(32), nullable=False, default="coins")
    amount = Column(Integer, nullable=False)
    balance_after = Column(Integer, nullable=False)
    reason = Column(String(255), nullable=False)
    actor_kind = Column(String(32), nullable=False)
    actor_id = Column(String(128), nullable=False)
    idempotency_key = Column(String(128), nullable=True)
    metadata_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class OperationTask(Base):
    __tablename__ = "operation_tasks"
    __table_args__ = (
        UniqueConstraint("idempotency_key", name="uq_operation_tasks_idempotency_key"),
        Index("ix_operation_tasks_status_created", "status", "created_at"),
        Index("ix_operation_tasks_owner", "owner_kind", "owner_id"),
    )

    id = Column(String(36), primary_key=True)
    task_type = Column(String(100), nullable=False)
    status = Column(String(32), nullable=False, default="pending")
    progress = Column(Integer, nullable=False, default=0)
    owner_kind = Column(String(32), nullable=False)
    owner_id = Column(String(128), nullable=False)
    idempotency_key = Column(String(128), nullable=True)
    input_json = Column(Text, nullable=True)
    result_json = Column(Text, nullable=True)
    error_message = Column(Text, nullable=True)
    retry_count = Column(Integer, nullable=False, default=0)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    started_at = Column(DateTime, nullable=True)
    finished_at = Column(DateTime, nullable=True)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class SystemEvent(Base):
    __tablename__ = "system_events"
    __table_args__ = (
        Index("ix_system_events_publish", "published_at", "created_at"),
        Index("ix_system_events_type", "event_type"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    event_type = Column(String(100), nullable=False)
    aggregate_type = Column(String(64), nullable=False)
    aggregate_id = Column(String(255), nullable=True)
    payload_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    published_at = Column(DateTime, nullable=True)


class JobRun(Base):
    __tablename__ = "job_runs"
    __table_args__ = (Index("ix_job_runs_job_started", "job_name", "started_at"),)

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    job_name = Column(String(100), nullable=False)
    trigger_kind = Column(String(32), nullable=False, default="scheduler")
    status = Column(String(32), nullable=False, default="running")
    summary_json = Column(Text, nullable=True)
    error_message = Column(Text, nullable=True)
    started_at = Column(DateTime, nullable=False, default=utcnow)
    finished_at = Column(DateTime, nullable=True)


class SecurityEvent(Base):
    __tablename__ = "security_events"
    __table_args__ = (
        Index("ix_security_events_type_created", "event_type", "created_at"),
        Index("ix_security_events_subject", "subject_kind", "subject_id"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    event_type = Column(String(100), nullable=False)
    severity = Column(String(20), nullable=False, default="info")
    subject_kind = Column(String(32), nullable=True)
    subject_id = Column(String(128), nullable=True)
    ip_address = Column(String(64), nullable=True)
    detail_json = Column(Text, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class DynamicSetting(Base):
    __tablename__ = "dynamic_settings"

    setting_key = Column(String(128), primary_key=True)
    value_json = Column(Text, nullable=False)
    value_type = Column(String(32), nullable=False, default="json")
    is_secret = Column(Boolean, nullable=False, default=False)
    revision = Column(Integer, nullable=False, default=1)
    updated_by_kind = Column(String(32), nullable=False)
    updated_by_id = Column(String(128), nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class ConfigRevision(Base):
    __tablename__ = "config_revisions"
    __table_args__ = (Index("ix_config_revisions_key_revision", "setting_key", "revision"),)

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    setting_key = Column(String(128), nullable=False)
    revision = Column(Integer, nullable=False)
    old_value_json = Column(Text, nullable=True)
    new_value_json = Column(Text, nullable=False)
    actor_kind = Column(String(32), nullable=False)
    actor_id = Column(String(128), nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class WebSession(Base):
    __tablename__ = "web_sessions"
    __table_args__ = (
        UniqueConstraint("token_hash", name="uq_web_sessions_token_hash"),
        Index("ix_web_sessions_tg_expires", "tg", "expires_at"),
    )

    id = Column(String(36), primary_key=True)
    tg = Column(BigInteger, nullable=False)
    token_hash = Column(String(128), nullable=False)
    csrf_hash = Column(String(128), nullable=False)
    auth_method = Column(String(32), nullable=False, default="telegram")
    ip_address = Column(String(64), nullable=True)
    user_agent = Column(String(512), nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    last_seen_at = Column(DateTime, nullable=False, default=utcnow)
    expires_at = Column(DateTime, nullable=False)
    revoked_at = Column(DateTime, nullable=True)


class WebLoginRequest(Base):
    __tablename__ = "web_login_requests"
    __table_args__ = (
        UniqueConstraint("request_token_hash", name="uq_web_login_requests_token"),
        Index("ix_web_login_requests_status_expires", "status", "expires_at"),
    )

    id = Column(String(36), primary_key=True)
    request_token_hash = Column(String(128), nullable=False)
    status = Column(String(32), nullable=False, default="pending")
    requested_tg = Column(BigInteger, nullable=True)
    approved_tg = Column(BigInteger, nullable=True)
    ip_address = Column(String(64), nullable=True)
    claimed_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    expires_at = Column(DateTime, nullable=False)
    approved_at = Column(DateTime, nullable=True)
    consumed_at = Column(DateTime, nullable=True)


class WebRole(Base):
    __tablename__ = "web_roles"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(64), nullable=False, unique=True)
    permissions_json = Column(Text, nullable=False)
    is_system = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class WebRoleMember(Base):
    __tablename__ = "web_role_members"
    __table_args__ = (
        UniqueConstraint("role_id", "tg", name="uq_web_role_members_role_tg"),
        Index("ix_web_role_members_tg", "tg"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    role_id = Column(Integer, nullable=False)
    tg = Column(BigInteger, nullable=False)
    created_by = Column(BigInteger, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
