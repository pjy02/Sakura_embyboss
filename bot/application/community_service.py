import json
from typing import Optional
from uuid import uuid4

from sqlalchemy.exc import IntegrityError

from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import utcnow
from bot.sql_helper.sql_community import (
    MediaReview,
    NotificationPreference,
    ReviewReaction,
    ReviewReport,
    UserNotification,
)


NOTIFICATION_CATEGORIES = (
    ("system", "系统公告"),
    ("billing", "充值与账单"),
    ("ticket", "服务工单"),
    ("request", "求片进度"),
    ("review", "影评状态"),
)
NOTIFICATION_CATEGORY_NAMES = {category for category, _label in NOTIFICATION_CATEGORIES}
NOTIFICATION_SEVERITIES = {"info", "success", "warning", "danger"}
REVIEW_REPORT_REASONS = {"spam", "abuse", "spoiler", "irrelevant", "other"}
REVIEW_MODERATION_STATUSES = {"published", "rejected", "hidden"}


def _json(value):
    return (
        json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str)
        if value is not None
        else None
    )


def serialize_review(row: MediaReview, *, liked=False) -> dict:
    return {
        "id": row.id,
        "tg": row.tg,
        "media_key": row.media_key,
        "media_title": row.media_title,
        "media_year": row.media_year,
        "rating": row.rating,
        "content": row.content,
        "spoiler": bool(row.spoiler),
        "status": row.status,
        "like_count": row.like_count,
        "report_count": row.report_count,
        "liked": bool(liked),
        "admin_note": row.admin_note,
        "moderated_by": row.moderated_by,
        "moderated_at": row.moderated_at,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


def serialize_report(row: ReviewReport) -> dict:
    return {
        "id": row.id,
        "review_id": row.review_id,
        "tg": row.tg,
        "reason": row.reason,
        "detail": row.detail,
        "created_at": row.created_at,
    }


def serialize_notification(row: UserNotification) -> dict:
    metadata = None
    if row.metadata_json:
        try:
            metadata = json.loads(row.metadata_json)
        except (TypeError, ValueError):
            pass
    return {
        "id": row.id,
        "tg": row.tg,
        "category": row.category,
        "title": row.title,
        "body": row.body,
        "severity": row.severity,
        "action_url": row.action_url,
        "metadata": metadata,
        "read_at": row.read_at,
        "created_at": row.created_at,
    }


def add_notification(
    uow,
    *,
    tg: int,
    category: str,
    title: str,
    body: str,
    severity: str = "info",
    action_url: Optional[str] = None,
    metadata=None,
):
    if category not in NOTIFICATION_CATEGORY_NAMES:
        raise RuntimeError("未知通知分类")
    if severity not in NOTIFICATION_SEVERITIES:
        raise RuntimeError("未知通知级别")
    normalized_title = title.strip()
    normalized_body = body.strip()
    if not normalized_title or not normalized_body:
        raise RuntimeError("通知标题和正文不能为空")
    if not uow.community.notification_enabled(tg, category, "web"):
        return None
    safe_action_url = (
        action_url
        if action_url and action_url.startswith("/") and not action_url.startswith("//")
        else None
    )
    row = UserNotification(
        id=str(uuid4()),
        tg=tg,
        category=category,
        title=normalized_title[:200],
        body=normalized_body[:2000],
        severity=severity,
        action_url=safe_action_url,
        metadata_json=_json(metadata),
        created_at=utcnow(),
    )
    uow.community.add_notification(row)
    uow.operations.event(
        "notification.created",
        "user",
        str(tg),
        {"resource_type": "notification", "resource_id": row.id, "tg": tg},
    )
    return row


class ReviewService:
    EDITABLE_STATUSES = {"pending", "rejected"}

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def create(
        self,
        *,
        tg: int,
        media_key: str,
        media_title: str,
        media_year: Optional[int],
        rating: int,
        content: str,
        spoiler: bool,
        actor: Actor,
    ) -> dict:
        now = utcnow()
        normalized_key = media_key.strip().lower()
        normalized_title = media_title.strip()
        normalized_content = content.strip()
        if (
            not normalized_key
            or len(normalized_key) > 255
            or not normalized_title
            or len(normalized_title) > 255
            or not 1 <= rating <= 10
            or media_year is not None
            and not 1888 <= media_year <= 2200
            or not 10 <= len(normalized_content) <= 5000
        ):
            raise RuntimeError("作品信息或影评内容不完整")
        with self._uow_factory() as uow:
            if uow.community.review_by_user_media(tg, normalized_key):
                raise RuntimeError("你已经评价过这部作品")
            row = MediaReview(
                id=str(uuid4()),
                tg=tg,
                media_key=normalized_key,
                media_title=normalized_title,
                media_year=media_year,
                rating=rating,
                content=normalized_content,
                spoiler=spoiler,
                status="pending",
                created_at=now,
                updated_at=now,
            )
            uow.community.add_review(row)
            uow.operations.audit(
                actor=actor,
                action="review.create",
                resource_type="media_review",
                resource_id=row.id,
                detail={"media_key": normalized_key, "rating": rating},
            )
            uow.operations.event(
                "review.created",
                "user",
                str(tg),
                {"resource_type": "media_review", "resource_id": row.id, "tg": tg},
            )
            try:
                uow.flush()
            except IntegrityError as exc:
                raise RuntimeError("你已经评价过这部作品") from exc
            return serialize_review(row)

    def list_published(
        self,
        *,
        viewer_tg: int,
        search=None,
        minimum_rating=None,
        limit=50,
        offset=0,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.community.list_reviews(
                status="published",
                search=search,
                minimum_rating=minimum_rating,
                limit=limit,
                offset=offset,
            )
            items = [
                serialize_review(
                    row,
                    liked=bool(uow.community.reaction(row.id, viewer_tg)),
                )
                for row in rows
            ]
            return {"items": items, "total": total, "limit": limit, "offset": offset}

    def list_mine(self, *, tg: int, limit=50, offset=0) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.community.list_reviews(
                tg=tg,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_review(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def list_admin(
        self,
        *,
        status=None,
        search=None,
        minimum_rating=None,
        limit=50,
        offset=0,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.community.list_reviews(
                status=status,
                search=search,
                minimum_rating=minimum_rating,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_review(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def detail_admin(self, review_id: str) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id)
            if row is None:
                return None
            reports = uow.community.review_reports(review_id)
            return {
                **serialize_review(row),
                "reports": [serialize_report(item) for item in reports],
            }

    def update_mine(
        self,
        review_id: str,
        *,
        tg: int,
        data: dict,
        actor: Actor,
    ) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id, tg=tg, for_update=True)
            if row is None:
                return None
            if row.status not in self.EDITABLE_STATUSES:
                raise RuntimeError("当前审核状态不能修改")
            for field in ("rating", "content", "spoiler"):
                if field in data:
                    value = data[field]
                    if value is None:
                        raise RuntimeError("影评字段不能设置为空")
                    if field == "rating" and not 1 <= value <= 10:
                        raise RuntimeError("评分必须在 1 到 10 之间")
                    if field == "content":
                        value = value.strip()
                        if not 10 <= len(value) <= 5000:
                            raise RuntimeError("影评内容需要 10 到 5000 个字符")
                    setattr(row, field, value)
            row.status = "pending"
            row.admin_note = None
            row.moderated_at = None
            row.moderated_by = None
            row.updated_at = utcnow()
            uow.operations.audit(
                actor=actor,
                action="review.update",
                resource_type="media_review",
                resource_id=review_id,
                detail=data,
            )
            uow.operations.event(
                "review.updated",
                "user",
                str(tg),
                {"resource_type": "media_review", "resource_id": review_id, "tg": tg, "status": "pending"},
            )
            uow.flush()
            return serialize_review(row)

    def delete_mine(self, review_id: str, *, tg: int, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id, tg=tg, for_update=True)
            if row is None:
                return False
            uow.community.delete_review(row)
            uow.operations.audit(
                actor=actor,
                action="review.delete",
                resource_type="media_review",
                resource_id=review_id,
            )
            uow.operations.event(
                "review.deleted",
                "user",
                str(tg),
                {"resource_type": "media_review", "resource_id": review_id, "tg": tg},
            )
            return True

    def react(self, review_id: str, *, tg: int, enabled: bool) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id, for_update=True)
            if row is None or row.status != "published":
                return None
            exists = uow.community.reaction(review_id, tg)
            if enabled and not exists:
                uow.community.add_reaction(ReviewReaction(review_id=review_id, tg=tg))
                row.like_count += 1
            elif not enabled and exists:
                uow.community.remove_reaction(review_id, tg)
                row.like_count = max(0, row.like_count - 1)
            row.updated_at = utcnow()
            uow.flush()
            return serialize_review(row, liked=enabled)

    def report(
        self,
        review_id: str,
        *,
        tg: int,
        reason: str,
        detail: Optional[str],
        actor: Actor,
    ) -> Optional[dict]:
        if reason not in REVIEW_REPORT_REASONS:
            raise RuntimeError("未知举报原因")
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id, for_update=True)
            if row is None or row.status != "published":
                return None
            if row.tg == tg:
                raise RuntimeError("不能举报自己的影评")
            if uow.community.report(review_id, tg):
                raise RuntimeError("你已经举报过这条影评")
            uow.community.add_report(
                ReviewReport(
                    review_id=review_id,
                    tg=tg,
                    reason=reason,
                    detail=(detail or "").strip() or None,
                )
            )
            row.report_count += 1
            uow.operations.audit(
                actor=actor,
                action="review.report",
                resource_type="media_review",
                resource_id=review_id,
                detail={"reason": reason},
            )
            uow.operations.event(
                "review.reported",
                "media_review",
                review_id,
                {"report_count": row.report_count},
            )
            uow.flush()
            return serialize_review(row)

    def moderate(
        self,
        review_id: str,
        *,
        status: str,
        admin_note: Optional[str],
        actor: Actor,
    ) -> Optional[dict]:
        if status not in REVIEW_MODERATION_STATUSES:
            raise RuntimeError("未知审核状态")
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.community.get_review(review_id, for_update=True)
            if row is None:
                return None
            row.status = status
            row.admin_note = (admin_note or "").strip() or None
            row.moderated_by = int(actor.identifier) if actor.identifier.isdigit() else None
            row.moderated_at = now
            row.updated_at = now
            labels = {"published": "已通过", "rejected": "未通过", "hidden": "已隐藏"}
            add_notification(
                uow,
                tg=row.tg,
                category="review",
                title=f"影评审核{labels.get(status, '已更新')}",
                body=f"《{row.media_title}》的影评状态已更新。" + (f" 备注：{row.admin_note}" if row.admin_note else ""),
                severity="success" if status == "published" else "warning",
                action_url="/reviews",
                metadata={"review_id": row.id, "status": status},
            )
            uow.operations.audit(
                actor=actor,
                action="review.moderate",
                resource_type="media_review",
                resource_id=review_id,
                detail={"status": status, "admin_note": row.admin_note},
            )
            uow.operations.event(
                "review.updated",
                "user",
                str(row.tg),
                {"resource_type": "media_review", "resource_id": review_id, "tg": row.tg, "status": status},
            )
            uow.flush()
            return serialize_review(row)


class NotificationService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(self, *, tg=None, category=None, unread_only=False, limit=50, offset=0):
        with self._uow_factory() as uow:
            rows, total = uow.community.list_notifications(
                tg=tg,
                category=category,
                unread_only=unread_only,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_notification(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def unread_count(self, tg: int) -> int:
        with self._uow_factory() as uow:
            return int(uow.community.unread_count(tg))

    def mark_read(self, notification_id: str, *, tg: int) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.community.get_notification(notification_id, tg=tg, for_update=True)
            if row is None:
                return None
            row.read_at = row.read_at or utcnow()
            uow.flush()
            return serialize_notification(row)

    def mark_all_read(self, tg: int) -> int:
        with self._uow_factory() as uow:
            return int(uow.community.mark_all_read(tg, utcnow()))

    def preferences(self, tg: int) -> list[dict]:
        with self._uow_factory() as uow:
            saved = {row.category: row for row in uow.community.list_preferences(tg)}
            return [
                {
                    "category": category,
                    "label": label,
                    "web_enabled": bool(saved[category].web_enabled) if category in saved else True,
                    "telegram_enabled": bool(saved[category].telegram_enabled) if category in saved else True,
                }
                for category, label in NOTIFICATION_CATEGORIES
            ]

    def update_preference(
        self,
        *,
        tg: int,
        category: str,
        web_enabled: bool,
        telegram_enabled: Optional[bool] = None,
    ) -> dict:
        if category not in NOTIFICATION_CATEGORY_NAMES:
            raise RuntimeError("未知通知分类")
        with self._uow_factory() as uow:
            row = uow.community.get_preference(tg, category, for_update=True)
            if row is None:
                row = NotificationPreference(tg=tg, category=category)
                uow.community.add_preference(row)
            row.web_enabled = web_enabled
            if telegram_enabled is not None:
                row.telegram_enabled = telegram_enabled
            row.updated_at = utcnow()
            uow.flush()
            return {
                "category": category,
                "web_enabled": bool(row.web_enabled),
                "telegram_enabled": bool(row.telegram_enabled),
            }

    def broadcast(
        self,
        *,
        target_tg: Optional[int],
        category: str,
        title: str,
        body: str,
        severity: str,
        action_url: Optional[str],
        actor: Actor,
    ) -> dict:
        if category not in NOTIFICATION_CATEGORY_NAMES:
            raise RuntimeError("未知通知分类")
        if severity not in NOTIFICATION_SEVERITIES:
            raise RuntimeError("未知通知级别")
        with self._uow_factory() as uow:
            if target_tg is not None and uow.users.get(target_tg) is None:
                raise RuntimeError("目标用户不存在")
            recipients = [target_tg] if target_tg is not None else uow.community.user_ids()
            created = 0
            for tg in recipients:
                if add_notification(
                    uow,
                    tg=tg,
                    category=category,
                    title=title,
                    body=body,
                    severity=severity,
                    action_url=action_url,
                    metadata={"broadcast": True},
                ):
                    created += 1
            uow.operations.audit(
                actor=actor,
                action="notification.broadcast",
                resource_type="notification",
                resource_id=None,
                detail={"target_tg": target_tg, "category": category, "created": created},
            )
            return {"created": created, "recipients": len(recipients)}
