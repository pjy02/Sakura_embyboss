from typing import Optional
from urllib.parse import quote

from fastapi import APIRouter, Depends, Header, HTTPException, Request, Response
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.api.auth import set_auth_cookies
from backend.dependencies import (
    client_ip,
    csrf_protected_identity,
    current_identity,
    get_auth_service,
)
from backend.settings import WebSettings, get_settings
from bot.application import RegistrationService
from bot.application.auth_service import WebIdentity
from bot.domain import Actor, secret_fingerprint


router = APIRouter(prefix="/registration", tags=["registration"])
registration_service = RegistrationService()


class TelegramRegistrationStartRequest(BaseModel):
    tg: Optional[int] = Field(default=None, gt=0)


class TokenRequest(BaseModel):
    token: str = Field(min_length=20, max_length=256)


class RegistrationRequest(BaseModel):
    username: str = Field(min_length=2, max_length=32)
    safety_code: str = Field(min_length=4, max_length=6)
    registration_code: Optional[str] = Field(default=None, min_length=3, max_length=255)


def _raise_registration_error(status: str) -> None:
    status_map = {
        "user_not_found": (404, "Telegram 用户记录不存在，请重新完成身份验证"),
        "account_already_bound": (409, "当前 Telegram 已经绑定 Emby 账号"),
        "duplicate": (409, "已有注册任务正在排队或处理"),
        "username_taken": (409, "该用户名已经被使用"),
        "slot_full": (409, "当前注册名额已满"),
        "queue_full": (429, "注册队列已满，请稍后再试"),
        "registration_code_required": (409, "当前未开放注册，请填写有效注册码"),
        "invalid_code": (422, "注册码无效"),
        "used_code": (422, "注册码已被使用"),
        "forbidden_code": (403, "该注册码不属于当前用户"),
        "no_qualification": (409, "当前没有可用注册资格"),
        "invalid_username": (422, "用户名格式不正确"),
        "invalid_safety_code": (422, "安全码必须为 4 至 6 位数字"),
    }
    code, detail = status_map.get(status, (400, "注册请求未能提交"))
    raise HTTPException(status_code=code, detail=detail)


@router.get("/status")
async def public_registration_status():
    return await run_in_threadpool(registration_service.status)


@router.post("/telegram/start", status_code=201)
async def telegram_registration_start(
    payload: TelegramRegistrationStartRequest,
    request: Request,
    settings: WebSettings = Depends(get_settings),
):
    result = await run_in_threadpool(
        get_auth_service().create_telegram_login,
        ip_address=client_ip(request),
        requested_tg=payload.tg,
        purpose="registration",
    )
    if result.status == "rate_limited":
        raise HTTPException(status_code=429, detail="请求过于频繁，请稍后再试")
    token = result.data["request_token"]
    return {
        "request_id": result.data["request_id"],
        "request_token": token,
        "expires_at": result.data["expires_at"],
        "deep_link": f"https://t.me/{settings.bot_username}?start=webreg_{quote(token)}",
        "poll_after_seconds": 2,
    }


@router.post("/telegram/status")
async def telegram_registration_status(payload: TokenRequest):
    result = await run_in_threadpool(
        get_auth_service().telegram_login_status,
        payload.token,
    )
    if not result.ok or result.data.get("purpose") != "registration":
        raise HTTPException(status_code=404, detail="注册验证请求不存在")
    return result.data


@router.post("/telegram/exchange")
async def telegram_registration_exchange(
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
        expected_purpose="registration",
    )
    if not result.ok:
        status_map = {
            "not_approved": 409,
            "already_consumed": 409,
            "expired": 410,
            "invalid_request": 404,
            "purpose_mismatch": 404,
        }
        raise HTTPException(
            status_code=status_map.get(result.status, 400),
            detail="注册验证尚未确认或已经失效",
        )
    set_auth_cookies(
        response,
        settings=settings,
        session_token=result.data["session_token"],
        csrf_token=result.data["csrf_token"],
        max_age=min(settings.session_ttl_hours * 3600, 15 * 60),
    )
    return {
        "tg": result.data["tg"],
        "auth_method": result.data["auth_method"],
        "expires_at": result.data["expires_at"],
    }


@router.get("/me")
async def my_registration_status(identity: WebIdentity = Depends(current_identity)):
    return await run_in_threadpool(registration_service.status, identity.tg)


@router.post("/submit", status_code=202)
async def submit_registration(
    payload: RegistrationRequest,
    identity: WebIdentity = Depends(csrf_protected_identity),
    idempotency_key: Optional[str] = Header(
        default=None,
        alias="Idempotency-Key",
        min_length=8,
        max_length=128,
    ),
):
    if not idempotency_key:
        raise HTTPException(status_code=400, detail="缺少 Idempotency-Key")
    if identity.purpose != "registration":
        raise HTTPException(
            status_code=403,
            detail="请先通过注册中心完成 Telegram 注册身份确认",
        )
    key = "registration:" + secret_fingerprint(f"{identity.tg}:{idempotency_key}")
    try:
        result = await run_in_threadpool(
            registration_service.submit,
            tg=identity.tg,
            username=payload.username,
            safety_code=payload.safety_code,
            registration_code=payload.registration_code,
            actor=Actor.web(identity.tg),
            idempotency_key=key,
            channel="web",
        )
    except ValueError as error:
        raise HTTPException(status_code=422, detail=str(error)) from error
    if result.status == "duplicate" and result.data:
        return result.data
    if not result.ok:
        _raise_registration_error(result.status)
    return result.data


@router.get("/tasks/{task_id}")
async def registration_task(
    task_id: str,
    identity: WebIdentity = Depends(current_identity),
):
    task = await run_in_threadpool(registration_service.task, task_id, identity.tg)
    if not task:
        raise HTTPException(status_code=404, detail="注册任务不存在")
    return task


@router.post("/tasks/{task_id}/cancel")
async def cancel_registration_task(
    task_id: str,
    identity: WebIdentity = Depends(csrf_protected_identity),
):
    result = await run_in_threadpool(
        registration_service.cancel,
        task_id,
        identity.tg,
        Actor.web(identity.tg),
    )
    if not result.ok:
        status_code = 404 if result.status == "not_found" else 409
        raise HTTPException(status_code=status_code, detail="任务不存在或当前无法取消")
    return result.data
