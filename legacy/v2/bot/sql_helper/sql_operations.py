"""Persistent models for the Web operations console."""

from sqlalchemy import (
    BigInteger,
    Boolean,
    Column,
    DateTime,
    Float,
    Index,
    Integer,
    String,
    Text,
)

from bot.sql_helper import Base
from bot.sql_helper.sql_application import utcnow


class LineEndpoint(Base):
    __tablename__ = "line_endpoints"
    __table_args__ = (
        Index("ix_line_endpoints_enabled_sort", "enabled", "sort_order"),
        Index("ix_line_endpoints_status", "last_status", "last_checked_at"),
    )

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(100), nullable=False)
    base_url = Column(String(512), nullable=False, unique=True)
    region = Column(String(100), nullable=True)
    carrier = Column(String(100), nullable=True)
    audience = Column(String(32), nullable=False, default="all")
    weight = Column(Integer, nullable=False, default=100)
    sort_order = Column(Integer, nullable=False, default=0)
    enabled = Column(Boolean, nullable=False, default=True)
    maintenance = Column(Boolean, nullable=False, default=False)
    revision = Column(Integer, nullable=False, default=1)
    last_status = Column(String(32), nullable=False, default="unknown")
    last_latency_ms = Column(Integer, nullable=True)
    last_error = Column(String(512), nullable=True)
    last_checked_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class LineHealthSample(Base):
    __tablename__ = "line_health_samples"
    __table_args__ = (
        Index("ix_line_health_line_checked", "line_id", "checked_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    line_id = Column(Integer, nullable=False)
    success = Column(Boolean, nullable=False)
    status_code = Column(Integer, nullable=True)
    latency_ms = Column(Integer, nullable=True)
    error_message = Column(String(512), nullable=True)
    checked_at = Column(DateTime, nullable=False, default=utcnow)


class PlaybackSession(Base):
    __tablename__ = "playback_sessions"
    __table_args__ = (
        Index("ix_playback_sessions_active", "ended_at", "last_seen_at"),
        Index("ix_playback_sessions_session_active", "session_id", "ended_at"),
        Index("ix_playback_sessions_user_started", "emby_user_id", "started_at"),
        Index("ix_playback_sessions_device_started", "device_key", "started_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    session_id = Column(String(128), nullable=False)
    emby_user_id = Column(String(128), nullable=True)
    emby_user_name = Column(String(255), nullable=True)
    tg = Column(BigInteger, nullable=True)
    item_id = Column(String(128), nullable=True)
    item_name = Column(String(512), nullable=True)
    series_name = Column(String(512), nullable=True)
    item_type = Column(String(64), nullable=True)
    client_name = Column(String(255), nullable=True)
    app_version = Column(String(64), nullable=True)
    device_key = Column(String(255), nullable=True)
    device_name = Column(String(255), nullable=True)
    remote_address = Column(String(128), nullable=True)
    position_ticks = Column(BigInteger, nullable=False, default=0)
    runtime_ticks = Column(BigInteger, nullable=False, default=0)
    progress_percent = Column(Float, nullable=False, default=0)
    is_paused = Column(Boolean, nullable=False, default=False)
    is_transcoding = Column(Boolean, nullable=False, default=False)
    started_at = Column(DateTime, nullable=False, default=utcnow)
    last_seen_at = Column(DateTime, nullable=False, default=utcnow)
    ended_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class KnownDevice(Base):
    __tablename__ = "known_devices"
    __table_args__ = (
        Index("ix_known_devices_user_seen", "emby_user_id", "last_seen_at"),
        Index("ix_known_devices_risk_seen", "risk_level", "last_seen_at"),
    )

    device_key = Column(String(255), primary_key=True)
    emby_user_id = Column(String(128), nullable=True)
    emby_user_name = Column(String(255), nullable=True)
    tg = Column(BigInteger, nullable=True)
    device_name = Column(String(255), nullable=True)
    client_name = Column(String(255), nullable=True)
    app_version = Column(String(64), nullable=True)
    last_ip = Column(String(128), nullable=True)
    trusted = Column(Boolean, nullable=False, default=False)
    banned = Column(Boolean, nullable=False, default=False)
    risk_level = Column(String(20), nullable=False, default="normal")
    notes = Column(Text, nullable=True)
    playback_count = Column(Integer, nullable=False, default=0)
    first_seen_at = Column(DateTime, nullable=False, default=utcnow)
    last_seen_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)
