from typing import Optional
from urllib.parse import quote

from fastapi import APIRouter, Depends, HTTPException, Request, Response
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import (
    client_ip,
    csrf_protected_identity,
    current_identity,
    get_auth_service,
)
from backend.settings import WebSettings, get_settings
from bot.application.auth_service import WebIdentity
from bot.domain import Actor, secret_fingerprint
from bot.func_helper.emby import emby
from bot.sql_helper.sql_emby import sql_get_emby


router = APIRouter(prefix="/auth", tags=["authentication"])


class TelegramStartRequest(BaseModel):
    tg: Optional[int] = Field(default=None, gt=0)


class TokenRequest(BaseModel):
    token: str = Field(min_length=20, max_length=256)


class EmbyLoginRequest(BaseModel):
    username: str = Field(min_length=1, max_length=255)
    password: str = Field(min_length=1, max_length=255)


def _set_auth_cookies(
    response: Response,
    *,
    settings: WebSettings,
    session_token: str,
    csrf_token: str,
    max_age: int,
) -> None:
    response.set_cookie(
        key=settings.cookie_name,
        value=session_token,
        max_age=max_age,
        httponly=True,
        secure=settings.cookie_secure,
        samesite="lax",
        path="/",
    )
    response.set_cookie(
        key=settings.csrf_cookie_name,
        value=csrf_token,
        max_age=max_age,
        httponly=False,
        secure=settings.cookie_secure,
        samesite="strict",
        path="/",
    )


@router.post("/telegram/start", status_code=201)
async def telegram_start(
    payload: TelegramStartRequest,
    request: Request,
    settings: WebSettings = Depends(get_settings),
):
    result = await run_in_threadpool(
        get_auth_service().create_telegram_login,
        ip_address=client_ip(request),
        requested_tg=payload.tg,
    )
    if result.status == "rate_limited":
        raise HTTPException(status_code=429, detail="请求过于频繁，请稍后再试")
    token = result.data["request_token"]
    return {
        "request_id": result.data["request_id"],
        "request_token": token,
        "expires_at": result.data["expires_at"],
        "deep_link": f"https://t.me/{settings.bot_username}?start=web_{quote(token)}",
        "poll_after_seconds": 2,
    }


@router.post("/telegram/status")
async def telegram_status(payload: TokenRequest):
    result = await run_in_threadpool(
        get_auth_service().telegram_login_status,
        payload.token,
    )
    if not result.ok:
        raise HTTPException(status_code=404, detail="登录请求不存在")
    return result.data


@router.post("/telegram/exchange")
async def telegram_exchange(
    payload: TokenRequest,
    request: Request,
    response: Response,
    settings: WebSettings = Depends(get_settings),
):
    result = await run_in_threadpool(
        get_auth_service().exchange_telegram_login,
        raw_token=payload.token,
        user_agent=request.headers.get("user-agent"),
        ip_address=client_ip(request),
    )
    if not result.ok:
        status_map = {
            "not_approved": 409,
            "already_consumed": 409,
            "expired": 410,
            "invalid_request": 404,
        }
        raise HTTPException(
            status_code=status_map.get(result.status, 400),
            detail="登录请求尚未确认或已失效",
        )
    _set_auth_cookies(
        response,
        settings=settings,
        session_token=result.data["session_token"],
        csrf_token=result.data["csrf_token"],
        max_age=settings.session_ttl_hours * 3600,
    )
    return {
        "tg": result.data["tg"],
        "auth_method": result.data["auth_method"],
        "expires_at": result.data["expires_at"],
    }


@router.post("/emby")
async def emby_login(
    payload: EmbyLoginRequest,
    request: Request,
    response: Response,
    settings: WebSettings = Depends(get_settings),
):
    ip_address = client_ip(request)
    auth_service = get_auth_service()
    if not await run_in_threadpool(auth_service.emby_login_allowed, ip_address):
        raise HTTPException(status_code=429, detail="登录失败次数过多，请稍后再试")

    existing_user = await run_in_threadpool(sql_get_emby, payload.username)
    success = False
    embyid = None
    if existing_user:
        success, embyid = await emby.authority_account(
            tg_id=existing_user.tg,
            username=payload.username,
            password=payload.password,
        )
    if not success or not embyid:
        await run_in_threadpool(
            auth_service.record_emby_login_failure,
            ip_address=ip_address,
            username_fingerprint=secret_fingerprint(payload.username.lower()),
        )
        raise HTTPException(status_code=401, detail="用户名或密码错误")

    result = await run_in_threadpool(
        auth_service.create_emby_session,
        embyid=str(embyid),
        username=payload.username,
        user_agent=request.headers.get("user-agent"),
        ip_address=ip_address,
    )
    if not result.ok:
        raise HTTPException(status_code=401, detail="用户名或密码错误")
    _set_auth_cookies(
        response,
        settings=settings,
        session_token=result.data["session_token"],
        csrf_token=result.data["csrf_token"],
        max_age=settings.session_ttl_hours * 3600,
    )
    return {
        "tg": result.data["tg"],
        "auth_method": "emby",
        "expires_at": result.data["expires_at"],
    }


@router.get("/session")
async def session_info(identity: WebIdentity = Depends(current_identity)):
    return {
        "tg": identity.tg,
        "auth_method": identity.auth_method,
        "roles": identity.roles,
        "permissions": sorted(identity.permissions),
    }


@router.post("/logout", status_code=204)
async def logout(
    response: Response,
    identity: WebIdentity = Depends(csrf_protected_identity),
    settings: WebSettings = Depends(get_settings),
):
    await run_in_threadpool(
        get_auth_service().logout,
        identity.session_id,
        Actor.web(identity.tg),
    )
    response.delete_cookie(settings.cookie_name, path="/")
    response.delete_cookie(settings.csrf_cookie_name, path="/")
    response.status_code = 204
    return response


@router.post("/logout-all")
async def logout_all(
    response: Response,
    identity: WebIdentity = Depends(csrf_protected_identity),
    settings: WebSettings = Depends(get_settings),
):
    revoked = await run_in_threadpool(
        get_auth_service().logout_all,
        identity.tg,
        Actor.web(identity.tg),
    )
    response.delete_cookie(settings.cookie_name, path="/")
    response.delete_cookie(settings.csrf_cookie_name, path="/")
    return {"revoked_sessions": revoked}
