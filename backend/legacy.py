import hmac

from fastapi import APIRouter, Depends, Header, HTTPException, Query

from backend.settings import WebSettings, get_settings


async def verify_legacy_token(
    x_sakura_legacy_token: str | None = Header(None),
    token: str | None = Query(None),
    settings: WebSettings = Depends(get_settings),
):
    expected = settings.legacy_api_token or ""
    supplied = x_sakura_legacy_token or token or ""
    if not expected or not hmac.compare_digest(supplied, expected):
        raise HTTPException(status_code=401, detail="Legacy API token invalid")


def create_legacy_router() -> APIRouter:
    from bot.web.api.ban_playlist import route as ban_playlist_route
    from bot.web.api.login import router as login_router
    from bot.web.api.user_info import route as user_info_route
    from bot.web.api.webhook.client_filter import router as client_filter_router
    from bot.web.api.webhook.favorites import router as favorites_router
    from bot.web.api.webhook.line_report import router as line_report_router
    from bot.web.api.webhook.media import router as media_router

    root = APIRouter(dependencies=[Depends(verify_legacy_token)])
    emby_router = APIRouter(prefix="/emby", tags=["legacy-emby"])
    emby_router.include_router(ban_playlist_route)
    emby_router.include_router(favorites_router)
    emby_router.include_router(media_router)
    emby_router.include_router(client_filter_router)
    emby_router.include_router(line_report_router)

    user_router = APIRouter(prefix="/user", tags=["legacy-user"])
    user_router.include_router(user_info_route)

    auth_router = APIRouter(prefix="/auth", tags=["legacy-auth"])
    auth_router.include_router(login_router)

    root.include_router(emby_router)
    root.include_router(user_router)
    root.include_router(auth_router)
    return root
