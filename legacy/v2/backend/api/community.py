from typing import Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import csrf_protected_identity, current_identity, require_permission
from bot.application import NotificationService, ReviewService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor


admin_router = APIRouter(prefix="/admin", tags=["community-administration"])
me_router = APIRouter(prefix="/me", tags=["community"])
reviews = ReviewService()
notifications = NotificationService()


class ReviewCreatePayload(BaseModel):
    media_key: str = Field(min_length=1, max_length=255)
    media_title: str = Field(min_length=1, max_length=255)
    media_year: Optional[int] = Field(None, ge=1888, le=2200)
    rating: int = Field(ge=1, le=10)
    content: str = Field(min_length=10, max_length=5000)
    spoiler: bool = False


class ReviewUpdatePayload(BaseModel):
    rating: Optional[int] = Field(None, ge=1, le=10)
    content: Optional[str] = Field(None, min_length=10, max_length=5000)
    spoiler: Optional[bool] = None


class ReactionPayload(BaseModel):
    enabled: bool


class ReviewReportPayload(BaseModel):
    reason: Literal["spam", "abuse", "spoiler", "irrelevant", "other"]
    detail: Optional[str] = Field(None, max_length=500)


class ReviewModerationPayload(BaseModel):
    status: Literal["published", "rejected", "hidden"]
    admin_note: Optional[str] = Field(None, max_length=1000)


class NotificationPreferencePayload(BaseModel):
    category: Literal["system", "billing", "ticket", "request", "review"]
    web_enabled: bool


class BroadcastPayload(BaseModel):
    target_tg: Optional[int] = Field(None, ge=1)
    category: Literal["system", "billing", "ticket", "request", "review"] = "system"
    title: str = Field(min_length=2, max_length=200)
    body: str = Field(min_length=2, max_length=2000)
    severity: Literal["info", "success", "warning", "danger"] = "info"
    action_url: Optional[str] = Field(None, max_length=500)


@me_router.get("/reviews")
async def published_reviews(
    search: Optional[str] = Query(None, max_length=255),
    minimum_rating: Optional[int] = Query(None, ge=1, le=10),
    limit: int = Query(30, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(
        reviews.list_published,
        viewer_tg=identity.tg,
        search=search,
        minimum_rating=minimum_rating,
        limit=limit,
        offset=offset,
    )


@me_router.get("/reviews/mine")
async def my_reviews(
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(reviews.list_mine, tg=identity.tg, limit=limit, offset=offset)


@me_router.post("/reviews", status_code=201)
async def create_review(
    payload: ReviewCreatePayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    try:
        return await run_in_threadpool(
            reviews.create,
            tg=identity.tg,
            **payload.model_dump(),
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@me_router.patch("/reviews/{review_id}")
async def update_review(
    review_id: str,
    payload: ReviewUpdatePayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    if not payload.model_fields_set:
        raise HTTPException(status_code=400, detail="没有可更新的影评字段")
    try:
        result = await run_in_threadpool(
            reviews.update_mine,
            review_id,
            tg=identity.tg,
            data=payload.model_dump(exclude_unset=True),
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="影评不存在")
    return result


@me_router.delete("/reviews/{review_id}", status_code=204)
async def delete_review(
    review_id: str,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    deleted = await run_in_threadpool(
        reviews.delete_mine,
        review_id,
        tg=identity.tg,
        actor=Actor.web(identity.tg),
    )
    if not deleted:
        raise HTTPException(status_code=404, detail="影评不存在")


@me_router.put("/reviews/{review_id}/reaction")
async def react_to_review(
    review_id: str,
    payload: ReactionPayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    result = await run_in_threadpool(reviews.react, review_id, tg=identity.tg, enabled=payload.enabled)
    if result is None:
        raise HTTPException(status_code=404, detail="影评不存在或尚未发布")
    return result


@me_router.post("/reviews/{review_id}/report")
async def report_review(
    review_id: str,
    payload: ReviewReportPayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    try:
        result = await run_in_threadpool(
            reviews.report,
            review_id,
            tg=identity.tg,
            reason=payload.reason,
            detail=payload.detail,
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    if result is None:
        raise HTTPException(status_code=404, detail="影评不存在或尚未发布")
    return result


@admin_router.get("/reviews")
async def admin_reviews(
    status: Optional[str] = Query(None, max_length=32),
    search: Optional[str] = Query(None, max_length=255),
    minimum_rating: Optional[int] = Query(None, ge=1, le=10),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("reviews:read", telegram_only=True)),
):
    return await run_in_threadpool(
        reviews.list_admin,
        status=status,
        search=search,
        minimum_rating=minimum_rating,
        limit=limit,
        offset=offset,
    )


@admin_router.get("/reviews/{review_id}")
async def admin_review_detail(
    review_id: str,
    _identity: WebIdentity = Depends(require_permission("reviews:read", telegram_only=True)),
):
    result = await run_in_threadpool(reviews.detail_admin, review_id)
    if result is None:
        raise HTTPException(status_code=404, detail="影评不存在")
    return result


@admin_router.patch("/reviews/{review_id}")
async def moderate_review(
    review_id: str,
    payload: ReviewModerationPayload,
    identity: WebIdentity = Depends(require_permission("reviews:update", csrf=True, telegram_only=True)),
):
    result = await run_in_threadpool(
        reviews.moderate,
        review_id,
        status=payload.status,
        admin_note=payload.admin_note,
        actor=Actor.web(identity.tg),
    )
    if result is None:
        raise HTTPException(status_code=404, detail="影评不存在")
    return result


@me_router.get("/notifications")
async def my_notifications(
    category: Optional[str] = Query(None, max_length=32),
    unread_only: bool = False,
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return await run_in_threadpool(
        notifications.list,
        tg=identity.tg,
        category=category,
        unread_only=unread_only,
        limit=limit,
        offset=offset,
    )


@me_router.get("/notifications/unread-count")
async def unread_count(identity: WebIdentity = Depends(current_identity)):
    return {"count": await run_in_threadpool(notifications.unread_count, identity.tg)}


@me_router.post("/notifications/read-all")
async def read_all_notifications(identity: WebIdentity = Depends(csrf_protected_identity)):
    return {"updated": await run_in_threadpool(notifications.mark_all_read, identity.tg)}


@me_router.post("/notifications/{notification_id}/read")
async def read_notification(
    notification_id: str,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    result = await run_in_threadpool(notifications.mark_read, notification_id, tg=identity.tg)
    if result is None:
        raise HTTPException(status_code=404, detail="通知不存在")
    return result


@me_router.get("/notification-preferences")
async def notification_preferences(identity: WebIdentity = Depends(current_identity)):
    return {"items": await run_in_threadpool(notifications.preferences, identity.tg)}


@me_router.put("/notification-preferences")
async def update_notification_preference(
    payload: NotificationPreferencePayload,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    return await run_in_threadpool(
        notifications.update_preference,
        tg=identity.tg,
        **payload.model_dump(),
    )


@admin_router.get("/notifications")
async def admin_notifications(
    category: Optional[str] = Query(None, max_length=32),
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _identity: WebIdentity = Depends(require_permission("notifications:read", telegram_only=True)),
):
    return await run_in_threadpool(
        notifications.list,
        category=category,
        limit=limit,
        offset=offset,
    )


@admin_router.post("/notifications/broadcast")
async def broadcast_notification(
    payload: BroadcastPayload,
    identity: WebIdentity = Depends(require_permission("notifications:send", csrf=True, telegram_only=True)),
):
    try:
        return await run_in_threadpool(
            notifications.broadcast,
            **payload.model_dump(),
            actor=Actor.web(identity.tg),
        )
    except RuntimeError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
