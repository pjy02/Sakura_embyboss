from sqlalchemy import func, or_
from sqlalchemy.orm import Session

from bot.sql_helper.sql_community import (
    MediaReview,
    NotificationPreference,
    ReviewReaction,
    ReviewReport,
    UserNotification,
)
from bot.sql_helper.sql_emby import Emby


class CommunityRepository:
    def __init__(self, session: Session):
        self.session = session

    def add_review(self, row: MediaReview):
        self.session.add(row)

    def delete_review(self, row: MediaReview):
        self.session.query(ReviewReaction).filter(
            ReviewReaction.review_id == row.id
        ).delete(synchronize_session=False)
        self.session.query(ReviewReport).filter(
            ReviewReport.review_id == row.id
        ).delete(synchronize_session=False)
        self.session.delete(row)

    def get_review(self, review_id: str, *, tg=None, for_update=False):
        query = self.session.query(MediaReview).filter(MediaReview.id == review_id)
        if tg is not None:
            query = query.filter(MediaReview.tg == tg)
        return query.with_for_update().first() if for_update else query.first()

    def review_by_user_media(self, tg: int, media_key: str):
        return (
            self.session.query(MediaReview)
            .filter(MediaReview.tg == tg, MediaReview.media_key == media_key)
            .first()
        )

    def list_reviews(
        self,
        *,
        tg=None,
        status=None,
        search=None,
        minimum_rating=None,
        limit=50,
        offset=0,
    ):
        query = self.session.query(MediaReview)
        if tg is not None:
            query = query.filter(MediaReview.tg == tg)
        if status:
            query = query.filter(MediaReview.status == status)
        if search:
            pattern = f"%{search.strip()}%"
            conditions = [
                MediaReview.media_title.like(pattern),
                MediaReview.content.like(pattern),
                MediaReview.media_key.like(pattern),
            ]
            if search.strip().isdigit():
                conditions.append(MediaReview.tg == int(search))
            query = query.filter(or_(*conditions))
        if minimum_rating is not None:
            query = query.filter(MediaReview.rating >= minimum_rating)
        total = query.count()
        rows = (
            query.order_by(MediaReview.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def reaction(self, review_id: str, tg: int):
        return (
            self.session.query(ReviewReaction)
            .filter(ReviewReaction.review_id == review_id, ReviewReaction.tg == tg)
            .first()
        )

    def add_reaction(self, row: ReviewReaction):
        self.session.add(row)

    def remove_reaction(self, review_id: str, tg: int):
        return (
            self.session.query(ReviewReaction)
            .filter(ReviewReaction.review_id == review_id, ReviewReaction.tg == tg)
            .delete(synchronize_session=False)
        )

    def report(self, review_id: str, tg: int):
        return (
            self.session.query(ReviewReport)
            .filter(ReviewReport.review_id == review_id, ReviewReport.tg == tg)
            .first()
        )

    def add_report(self, row: ReviewReport):
        self.session.add(row)

    def review_reports(self, review_id: str):
        return (
            self.session.query(ReviewReport)
            .filter(ReviewReport.review_id == review_id)
            .order_by(ReviewReport.created_at.desc())
            .all()
        )

    def add_notification(self, row: UserNotification):
        self.session.add(row)

    def get_notification(self, notification_id: str, *, tg=None, for_update=False):
        query = self.session.query(UserNotification).filter(UserNotification.id == notification_id)
        if tg is not None:
            query = query.filter(UserNotification.tg == tg)
        return query.with_for_update().first() if for_update else query.first()

    def list_notifications(
        self,
        *,
        tg=None,
        category=None,
        unread_only=False,
        limit=50,
        offset=0,
    ):
        query = self.session.query(UserNotification)
        if tg is not None:
            query = query.filter(UserNotification.tg == tg)
        if category:
            query = query.filter(UserNotification.category == category)
        if unread_only:
            query = query.filter(UserNotification.read_at.is_(None))
        total = query.count()
        rows = (
            query.order_by(UserNotification.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def unread_count(self, tg: int):
        return (
            self.session.query(func.count(UserNotification.id))
            .filter(UserNotification.tg == tg, UserNotification.read_at.is_(None))
            .scalar()
            or 0
        )

    def mark_all_read(self, tg: int, read_at):
        return (
            self.session.query(UserNotification)
            .filter(UserNotification.tg == tg, UserNotification.read_at.is_(None))
            .update({UserNotification.read_at: read_at}, synchronize_session=False)
        )

    def list_preferences(self, tg: int):
        return (
            self.session.query(NotificationPreference)
            .filter(NotificationPreference.tg == tg)
            .all()
        )

    def get_preference(self, tg: int, category: str, *, for_update=False):
        query = self.session.query(NotificationPreference).filter(
            NotificationPreference.tg == tg,
            NotificationPreference.category == category,
        )
        return query.with_for_update().first() if for_update else query.first()

    def add_preference(self, row: NotificationPreference):
        self.session.add(row)

    def notification_enabled(self, tg: int, category: str, channel: str = "web"):
        row = self.get_preference(tg, category)
        if row is None:
            return True
        return bool(row.web_enabled if channel == "web" else row.telegram_enabled)

    def user_ids(self):
        return [int(row[0]) for row in self.session.query(Emby.tg).all()]
