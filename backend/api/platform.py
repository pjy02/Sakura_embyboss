from __future__ import annotations

from datetime import datetime
from typing import Any, Callable, Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from backend.dependencies import current_identity, require_permission
from bot.application import (
    ApiClientService,
    AutomationService,
    BackupService,
    CredentialService,
    DeviceRuleService,
    DynamicSettingsService,
    MediaCatalogService,
    MediaRequestService,
    MoviePilotGateway,
    MultiEmbyService,
    TaskService,
)
from bot.application.auth_service import WebIdentity
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork


admin_router = APIRouter(prefix="/admin", tags=["platform-center"])
media_router = APIRouter(prefix="/media", tags=["media-center"])
open_router = APIRouter(prefix="/api/open/v1", tags=["open-api"])

device_rules = DeviceRuleService()
credentials = CredentialService()
emby_instances = MultiEmbyService()
media = MediaCatalogService()
moviepilot = MoviePilotGateway()
automations = AutomationService()
api_clients = ApiClientService()
backups = BackupService()
tasks = TaskService()
requests = MediaRequestService()
dynamic_settings = DynamicSettingsService()


class DeviceRulePayload(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    pattern: str = Field(min_length=1, max_length=255)
    match_type: Literal["exact", "contains", "glob", "regex"] = "contains"
    action: Literal["allow", "block", "observe"] = "allow"
    enabled: bool = True
    priority: int = Field(100, ge=0, le=100000)
    notes: Optional[str] = Field(None, max_length=500)
    revision: Optional[int] = Field(None, ge=1)


class CredentialPayload(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    provider: str = Field(min_length=1, max_length=64, pattern=r"^[a-zA-Z0-9_.-]+$")
    credential_type: Literal["api_token", "api_key", "password", "bearer"] = "api_token"
    secret: Optional[str] = Field(None, min_length=1, max_length=10000)
    metadata: dict[str, Any] = Field(default_factory=dict)
    active: bool = True
    expires_at: Optional[datetime] = None
    revision: Optional[int] = Field(None, ge=1)


class EmbyInstancePayload(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    base_url: str = Field(min_length=8, max_length=512)
    credential_id: str = Field(min_length=36, max_length=36)
    enabled: bool = True
    is_default: bool = False
    verify_tls: bool = True
    priority: int = Field(100, ge=0, le=100000)
    revision: Optional[int] = Field(None, ge=1)


class AutomationPayload(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    description: Optional[str] = Field(None, max_length=500)
    trigger_type: Literal["event", "interval"] = "event"
    trigger_value: str = Field(min_length=1, max_length=255)
    conditions: dict[str, Any] = Field(default_factory=dict)
    actions: list[dict[str, Any]] = Field(min_length=1, max_length=20)
    enabled: bool = True
    cooldown_seconds: int = Field(0, ge=0, le=604800)
    revision: Optional[int] = Field(None, ge=1)


class ApiClientPayload(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    scopes: list[str] = Field(min_length=1, max_length=20)
    expires_at: Optional[datetime] = None


class ClientEvaluationPayload(BaseModel):
    client_name: str = Field(min_length=1, max_length=255)


class MoviePilotSubmitPayload(BaseModel):
    resource: dict[str, Any]
    confirm: bool = False


def _translate_error(exc: Exception) -> HTTPException:
    if isinstance(exc, LookupError):
        return HTTPException(status_code=404, detail=str(exc))
    if isinstance(exc, RuntimeError):
        return HTTPException(status_code=409, detail=str(exc))
    return HTTPException(status_code=400, detail=str(exc))


@admin_router.get("/device-rules")
async def list_device_rules(_identity: WebIdentity = Depends(require_permission("devices:read", telegram_only=True))):
    return {"items": await run_in_threadpool(device_rules.list)}


@admin_router.post("/device-rules", status_code=201)
async def create_device_rule(payload: DeviceRulePayload, identity: WebIdentity = Depends(require_permission("devices:update", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(device_rules.save, payload.model_dump(exclude={"revision"}), Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.patch("/device-rules/{rule_id}")
async def update_device_rule(rule_id: int, payload: DeviceRulePayload, identity: WebIdentity = Depends(require_permission("devices:update", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(device_rules.save, payload.model_dump(), Actor.web(identity.tg), rule_id)
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.delete("/device-rules/{rule_id}", status_code=204)
async def delete_device_rule(rule_id: int, identity: WebIdentity = Depends(require_permission("devices:update", csrf=True, telegram_only=True))):
    try:
        if not await run_in_threadpool(device_rules.delete, rule_id, Actor.web(identity.tg)):
            raise HTTPException(status_code=404, detail="设备规则不存在")
    except HTTPException:
        raise
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.post("/device-rules/evaluate")
async def evaluate_device_rule(payload: ClientEvaluationPayload, _identity: WebIdentity = Depends(require_permission("devices:read", csrf=True, telegram_only=True))):
    return await run_in_threadpool(device_rules.evaluate, payload.client_name)


@admin_router.get("/credentials")
async def list_credentials(provider: Optional[str] = Query(None, max_length=64), _identity: WebIdentity = Depends(require_permission("integrations:read", telegram_only=True))):
    return {"items": await run_in_threadpool(credentials.list, provider)}


@admin_router.post("/credentials", status_code=201)
async def create_credential(payload: CredentialPayload, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(credentials.save, payload.model_dump(), Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.patch("/credentials/{credential_id}")
async def update_credential(credential_id: str, payload: CredentialPayload, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(credentials.save, payload.model_dump(), Actor.web(identity.tg), credential_id)
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.delete("/credentials/{credential_id}", status_code=204)
async def delete_credential(credential_id: str, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        if not await run_in_threadpool(credentials.delete, credential_id, Actor.web(identity.tg)):
            raise HTTPException(status_code=404, detail="凭据不存在")
    except HTTPException:
        raise
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.get("/emby-instances")
async def list_emby_instances(_identity: WebIdentity = Depends(require_permission("integrations:read", telegram_only=True))):
    return {"items": await run_in_threadpool(emby_instances.list)}


@admin_router.post("/emby-instances", status_code=201)
async def create_emby_instance(payload: EmbyInstancePayload, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(emby_instances.save, payload.model_dump(exclude={"revision"}), Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.patch("/emby-instances/{instance_id}")
async def update_emby_instance(instance_id: str, payload: EmbyInstancePayload, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(emby_instances.save, payload.model_dump(), Actor.web(identity.tg), instance_id)
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.delete("/emby-instances/{instance_id}", status_code=204)
async def delete_emby_instance(instance_id: str, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        if not await run_in_threadpool(emby_instances.delete, instance_id, Actor.web(identity.tg)):
            raise HTTPException(status_code=404, detail="Emby 实例不存在")
    except HTTPException:
        raise
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.post("/emby-instances/{instance_id}/probe")
async def probe_emby_instance(instance_id: str, _identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await emby_instances.probe(instance_id)
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.post("/emby-instances/{instance_id}/adopt-legacy")
async def adopt_legacy_emby_bindings(instance_id: str, identity: WebIdentity = Depends(require_permission("integrations:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(emby_instances.adopt_legacy_bindings, instance_id, Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@media_router.get("/search")
async def search_media(q: str = Query(min_length=1, max_length=120), media_type: Optional[Literal["movie", "tv"]] = None, _identity: WebIdentity = Depends(current_identity)):
    return await media.search(q, media_type)


@media_router.get("/trending")
async def trending_media(limit: int = Query(20, ge=1, le=50), _identity: WebIdentity = Depends(current_identity)):
    return await media.trending(limit)


@admin_router.get("/moviepilot/search")
async def search_moviepilot(q: str = Query(min_length=1, max_length=120), _identity: WebIdentity = Depends(require_permission("media:manage", telegram_only=True))):
    try:
        return await moviepilot.search(q)
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@admin_router.post("/moviepilot/requests/{request_id}/submit")
async def submit_moviepilot(request_id: str, payload: MoviePilotSubmitPayload, _identity: WebIdentity = Depends(require_permission("media:manage", csrf=True, telegram_only=True))):
    if not payload.confirm:
        raise HTTPException(status_code=409, detail="提交下载任务前必须明确确认")
    item = await run_in_threadpool(requests.get, request_id)
    if not item:
        raise HTTPException(status_code=404, detail="求片记录不存在")
    try:
        return await moviepilot.submit(item, payload.resource)
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@admin_router.get("/automations")
async def list_automations(_identity: WebIdentity = Depends(require_permission("automation:read", telegram_only=True))):
    return await run_in_threadpool(automations.list)


@admin_router.post("/automations", status_code=201)
async def create_automation(payload: AutomationPayload, identity: WebIdentity = Depends(require_permission("automation:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(automations.save, payload.model_dump(exclude={"revision"}), Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.patch("/automations/{rule_id}")
async def update_automation(rule_id: str, payload: AutomationPayload, identity: WebIdentity = Depends(require_permission("automation:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(automations.save, payload.model_dump(), Actor.web(identity.tg), rule_id)
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.delete("/automations/{rule_id}", status_code=204)
async def delete_automation(rule_id: str, identity: WebIdentity = Depends(require_permission("automation:manage", csrf=True, telegram_only=True))):
    if not await run_in_threadpool(automations.delete, rule_id, Actor.web(identity.tg)):
        raise HTTPException(status_code=404, detail="自动化规则不存在")


@admin_router.post("/automations/evaluate")
async def evaluate_automations(_identity: WebIdentity = Depends(require_permission("automation:manage", csrf=True, telegram_only=True))):
    return await automations.evaluate()


@admin_router.get("/api-clients")
async def list_api_clients(_identity: WebIdentity = Depends(require_permission("api:read", telegram_only=True))):
    return {"items": await run_in_threadpool(api_clients.list), "available_scopes": sorted(ApiClientService.ALLOWED_SCOPES)}


@admin_router.post("/api-clients", status_code=201)
async def create_api_client(payload: ApiClientPayload, identity: WebIdentity = Depends(require_permission("api:manage", csrf=True, telegram_only=True))):
    try:
        return await run_in_threadpool(api_clients.create, name=payload.name, scopes=payload.scopes, expires_at=payload.expires_at, actor=Actor.web(identity.tg))
    except Exception as exc:
        raise _translate_error(exc) from exc


@admin_router.post("/api-clients/{client_id}/revoke")
async def revoke_api_client(client_id: str, identity: WebIdentity = Depends(require_permission("api:manage", csrf=True, telegram_only=True))):
    if not await run_in_threadpool(api_clients.revoke, client_id, Actor.web(identity.tg)):
        raise HTTPException(status_code=404, detail="API 客户端不存在")
    return {"ok": True}


@admin_router.get("/backups")
async def list_backups(_identity: WebIdentity = Depends(require_permission("backups:read", telegram_only=True))):
    return await run_in_threadpool(backups.list)


@admin_router.post("/backups", status_code=202)
async def create_backup(identity: WebIdentity = Depends(require_permission("backups:manage", csrf=True, telegram_only=True))):
    result = await run_in_threadpool(tasks.enqueue, task_type="maintenance.backup_database", payload={}, actor=Actor.web(identity.tg), idempotency_key=f"manual-backup:{datetime.utcnow().strftime('%Y%m%d%H%M%S')}")
    if not result.ok:
        raise HTTPException(status_code=409, detail=result.status)
    return result.data


@admin_router.get("/backups/{name}/download")
async def download_backup(name: str, _identity: WebIdentity = Depends(require_permission("backups:read", telegram_only=True))):
    try:
        path = await run_in_threadpool(backups.resolve, name)
    except Exception as exc:
        raise _translate_error(exc) from exc
    return FileResponse(path, filename=path.name, media_type="application/sql")


def require_api_scope(scope: str) -> Callable:
    async def dependency(request: Request) -> dict[str, Any]:
        enabled = await run_in_threadpool(dynamic_settings.get, "integrations.open_api_enabled")
        if not enabled["value"]:
            raise HTTPException(status_code=404, detail="开放 API 未启用")
        authorization = request.headers.get("Authorization", "")
        raw_key = authorization[7:].strip() if authorization.lower().startswith("bearer ") else ""
        identity = await run_in_threadpool(api_clients.authenticate, raw_key, scope, request.client.host if request.client else None)
        if not identity:
            raise HTTPException(status_code=401, detail="API Key 无效、过期或缺少权限")
        return identity
    return dependency


@open_router.get("/health")
async def open_health(_client: dict[str, Any] = Depends(require_api_scope("health:read"))):
    return {"status": "ok", "checked_at": datetime.utcnow()}


@open_router.get("/media/search")
async def open_media_search(q: str = Query(min_length=1, max_length=120), media_type: Optional[Literal["movie", "tv"]] = None, _client: dict[str, Any] = Depends(require_api_scope("media:read"))):
    return await media.search(q, media_type)


class OpenRequestPayload(BaseModel):
    tg: int
    title: str = Field(min_length=1, max_length=255)
    year: Optional[int] = Field(None, ge=1888, le=2200)
    media_type: Literal["movie", "series", "anime", "documentary", "other"] = "other"
    description: Optional[str] = Field(None, max_length=2000)
    external_ref: Optional[str] = Field(None, max_length=255)


@open_router.post("/requests", status_code=201)
async def open_create_request(payload: OpenRequestPayload, client: dict[str, Any] = Depends(require_api_scope("requests:create"))):
    return await run_in_threadpool(requests.create, tg=payload.tg, title=payload.title, year=payload.year, media_type=payload.media_type, description=payload.description, external_ref=payload.external_ref, actor=Actor.system(f"open-api:{client['id']}"))


@open_router.get("/requests")
async def open_list_requests(tg: Optional[int] = None, status: Optional[str] = Query(None, max_length=32), client: dict[str, Any] = Depends(require_api_scope("requests:read"))):
    return await run_in_threadpool(requests.list, tg=tg, status=status, limit=100, offset=0)


class OpenEventPayload(BaseModel):
    event_type: str = Field(min_length=3, max_length=100)
    aggregate_type: str = Field(min_length=1, max_length=64)
    aggregate_id: Optional[str] = Field(None, max_length=255)
    payload: dict[str, Any] = Field(default_factory=dict)


@open_router.post("/events", status_code=202)
async def open_create_event(payload: OpenEventPayload, client: dict[str, Any] = Depends(require_api_scope("events:write"))):
    def write_event() -> dict[str, Any]:
        with SqlAlchemyUnitOfWork() as uow:
            row = uow.operations.event(payload.event_type, payload.aggregate_type, payload.aggregate_id, {**payload.payload, "api_client_id": client["id"]})
            uow.flush()
            return {"accepted": True, "event_id": row.id}
    return await run_in_threadpool(write_event)
