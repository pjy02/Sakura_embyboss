"""Media reviews, notifications and user notification preferences."""

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


class MediaReview(Base):
    __tablename__ = "media_reviews"
    __table_args__ = (
        UniqueConstraint("tg", "media_key", name="uq_media_reviews_user_media"),
        Index("ix_media_reviews_status_created", "status", "created_at"),
        Index("ix_media_reviews_media", "media_key", "status"),
        Index("ix_media_reviews_tg_created", "tg", "created_at"),
    )

    id = Column(String(36), primary_key=True)
    tg = Column(BigInteger, nullable=False)
    media_key = Column(String(255), nullable=False)
    media_title = Column(String(255), nullable=False)
    media_year = Column(Integer, nullable=True)
    rating = Column(Integer, nullable=False)
    content = Column(Text, nullable=False)
    spoiler = Column(Boolean, nullable=False, default=False)
    status = Column(String(32), nullable=False, default="pending")
    like_count = Column(Integer, nullable=False, default=0)
    report_count = Column(Integer, nullable=False, default=0)
    admin_note = Column(String(1000), nullable=True)
    moderated_by = Column(BigInteger, nullable=True)
    moderated_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)


class ReviewReaction(Base):
    __tablename__ = "review_reactions"
    __table_args__ = (
        UniqueConstraint("review_id", "tg", name="uq_review_reactions_review_tg"),
        Index("ix_review_reactions_tg", "tg", "created_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    review_id = Column(String(36), nullable=False)
    tg = Column(BigInteger, nullable=False)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class ReviewReport(Base):
    __tablename__ = "review_reports"
    __table_args__ = (
        UniqueConstraint("review_id", "tg", name="uq_review_reports_review_tg"),
        Index("ix_review_reports_review", "review_id", "created_at"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    review_id = Column(String(36), nullable=False)
    tg = Column(BigInteger, nullable=False)
    reason = Column(String(32), nullable=False)
    detail = Column(String(500), nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class UserNotification(Base):
    __tablename__ = "user_notifications"
    __table_args__ = (
        Index("ix_user_notifications_tg_created", "tg", "created_at"),
        Index("ix_user_notifications_tg_read", "tg", "read_at"),
        Index("ix_user_notifications_category", "category", "created_at"),
    )

    id = Column(String(36), primary_key=True)
    tg = Column(BigInteger, nullable=False)
    category = Column(String(32), nullable=False)
    title = Column(String(200), nullable=False)
    body = Column(String(2000), nullable=False)
    severity = Column(String(20), nullable=False, default="info")
    action_url = Column(String(500), nullable=True)
    metadata_json = Column(Text, nullable=True)
    read_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, nullable=False, default=utcnow)


class NotificationPreference(Base):
    __tablename__ = "notification_preferences"
    __table_args__ = (
        UniqueConstraint("tg", "category", name="uq_notification_preferences_tg_category"),
        Index("ix_notification_preferences_tg", "tg"),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    tg = Column(BigInteger, nullable=False)
    category = Column(String(32), nullable=False)
    web_enabled = Column(Boolean, nullable=False, default=True)
    telegram_enabled = Column(Boolean, nullable=False, default=True)
    updated_at = Column(DateTime, nullable=False, default=utcnow, onupdate=utcnow)
