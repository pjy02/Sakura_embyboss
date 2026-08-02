from datetime import timedelta
from functools import lru_cache
from typing import Callable

from fastapi import Depends, HTTPException, Request, status
from starlette.concurrency import run_in_threadpool

from backend.settings import WebSettings, get_settings
from bot.application import TokenCodec, WebAuthService
from bot.application.auth_service import WebIdentity


@lru_cache(maxsize=1)
def get_auth_service() -> WebAuthService:
    settings = get_settings()
    return WebAuthService(
        token_codec=TokenCodec(settings.session_secret),
        owner_tg=settings.owner_tg,
        admin_tg_ids=settings.admin_tg_ids,
        session_ttl=timedelta(hours=settings.session_ttl_hours),
        login_ttl=timedelta(seconds=settings.login_ttl_seconds),
    )


def client_ip(request: Request) -> str:
    return request.client.host if request.client else "unknown"


async def current_identity(
    request: Request,
    settings: WebSettings = Depends(get_settings),
) -> WebIdentity:
    raw_session = request.cookies.get(settings.cookie_name)
    if not raw_session:
        raise HTTPException(status_code=401, detail="未登录或会话已失效")
    identity = await run_in_threadpool(get_auth_service().authenticate, raw_session)
    if not identity:
        raise HTTPException(status_code=401, detail="未登录或会话已失效")
    request.state.identity = identity
    return identity


async def csrf_protected_identity(
    request: Request,
    identity: WebIdentity = Depends(current_identity),
) -> WebIdentity:
    csrf_token = request.headers.get("X-CSRF-Token", "")
    if not get_auth_service().verify_csrf(identity, csrf_token):
        raise HTTPException(status_code=403, detail="CSRF 校验失败")
    return identity


def require_permission(
    permission: str,
    *,
    csrf: bool = False,
    telegram_only: bool = False,
) -> Callable:
    dependency = csrf_protected_identity if csrf else current_identity

    async def checker(identity: WebIdentity = Depends(dependency)) -> WebIdentity:
        if telegram_only and identity.auth_method not in {"telegram", "local"}:
            raise HTTPException(
                status_code=403,
                detail="管理操作必须使用 Web 本地账号或 Telegram 强身份登录",
            )
        if not identity.has_permission(permission):
            raise HTTPException(status_code=403, detail="权限不足")
        return identity

    return checker


async def owner_identity(
    identity: WebIdentity = Depends(csrf_protected_identity),
) -> WebIdentity:
    if identity.auth_method not in {"telegram", "local"} or not identity.is_owner:
        raise HTTPException(status_code=403, detail="仅所有者可执行此操作")
    return identity
