from __future__ import annotations

import base64
import fnmatch
import hashlib
import hmac
import json
import os
import re
import secrets
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Optional
from uuid import uuid4

import aiohttp
from cryptography.fernet import Fernet, InvalidToken
from sqlalchemy import or_
from sqlalchemy.exc import IntegrityError

from bot.application.results import ServiceResult
from bot.application.task_service import TaskService
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import SystemEvent, utcnow
from bot.sql_helper.sql_accounts import Account
from bot.sql_helper.sql_emby import Emby as LegacyEmby
from bot.sql_helper.sql_platform import (
    AccountEmbyBinding,
    ApiClient,
    AutomationRule,
    AutomationRun,
    DeviceClientRule,
    EmbyInstance,
    ManagedCredential,
    MediaCatalogItem,
)


def _load_json(value: Optional[str], fallback: Any = None) -> Any:
    if not value:
        return fallback
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return fallback


def _dump_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str)


def _naive_utc(value: Optional[datetime]) -> Optional[datetime]:
    if value is None or value.tzinfo is None:
        return value
    return value.astimezone(timezone.utc).replace(tzinfo=None)


def _credential_public(row: ManagedCredential) -> dict[str, Any]:
    return {
        "id": row.id,
        "name": row.name,
        "provider": row.provider,
        "credential_type": row.credential_type,
        "fingerprint": row.fingerprint,
        "metadata": _load_json(row.metadata_json, {}),
        "active": bool(row.active),
        "last_used_at": row.last_used_at,
        "expires_at": row.expires_at,
        "revision": row.revision,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


class CredentialService:
    """Write-only credential vault backed by Fernet authenticated encryption."""

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    @staticmethod
    def _cipher() -> Fernet:
        secret = os.getenv("SAKURA_CREDENTIAL_MASTER_KEY") or os.getenv("SAKURA_WEB_SESSION_SECRET")
        if not secret or len(secret.encode("utf-8")) < 32:
            raise RuntimeError(
                "请设置至少 32 字节的 SAKURA_CREDENTIAL_MASTER_KEY；"
                "凭据中心不会把明文写入 config.json"
            )
        key = base64.urlsafe_b64encode(hashlib.sha256(secret.encode("utf-8")).digest())
        return Fernet(key)

    @classmethod
    def _encrypt(cls, value: str) -> str:
        return cls._cipher().encrypt(value.encode("utf-8")).decode("ascii")

    @classmethod
    def _decrypt(cls, value: str) -> str:
        try:
            return cls._cipher().decrypt(value.encode("ascii")).decode("utf-8")
        except InvalidToken as exc:
            raise RuntimeError("凭据无法解密，请确认主密钥未被更换") from exc

    def list(self, provider: Optional[str] = None) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            query = uow.session.query(ManagedCredential)
            if provider:
                query = query.filter(ManagedCredential.provider == provider.strip().lower())
            return [_credential_public(row) for row in query.order_by(ManagedCredential.provider, ManagedCredential.name).all()]

    def save(self, data: dict[str, Any], actor: Actor, credential_id: Optional[str] = None) -> dict[str, Any]:
        now = utcnow()
        provider = str(data.get("provider") or "").strip().lower()
        name = str(data.get("name") or "").strip()
        secret_value = str(data.get("secret") or "")
        if not provider or not name:
            raise ValueError("凭据名称和提供方不能为空")
        with self._uow_factory() as uow:
            row = uow.session.query(ManagedCredential).filter(ManagedCredential.id == credential_id).with_for_update().first() if credential_id else None
            if credential_id and row is None:
                raise LookupError("凭据不存在")
            if row is None:
                if not secret_value:
                    raise ValueError("新凭据必须填写密钥内容")
                row = ManagedCredential(id=str(uuid4()), created_at=now)
                uow.session.add(row)
            elif data.get("revision") is not None and int(data["revision"]) != int(row.revision):
                raise RuntimeError("凭据已被其他管理员修改，请刷新后重试")
            row.name = name
            row.provider = provider
            if provider != "emby" and uow.session.query(EmbyInstance.id).filter(
                EmbyInstance.credential_id == row.id
            ).first():
                raise RuntimeError("该凭据正被 Emby 实例使用，提供方必须保持为 emby")
            row.credential_type = str(data.get("credential_type") or row.credential_type or "api_token")
            if secret_value:
                row.ciphertext = self._encrypt(secret_value)
                row.fingerprint = hashlib.sha256(secret_value.encode("utf-8")).hexdigest()[:12]
            row.metadata_json = _dump_json(data.get("metadata") or {})
            row.active = bool(data.get("active", True))
            row.expires_at = _naive_utc(data.get("expires_at"))
            row.revision = int(row.revision or 1) + (1 if credential_id else 0)
            row.updated_at = now
            uow.operations.audit(actor=actor, action="credential.update" if credential_id else "credential.create", resource_type="managed_credential", resource_id=row.id, detail={"provider": provider, "name": name, "secret_rotated": bool(secret_value)})
            uow.flush()
            return _credential_public(row)

    def delete(self, credential_id: str, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.session.query(ManagedCredential).filter(ManagedCredential.id == credential_id).with_for_update().first()
            if row is None:
                return False
            if uow.session.query(EmbyInstance.id).filter(EmbyInstance.credential_id == row.id).first():
                raise RuntimeError("该凭据仍被 Emby 实例使用")
            uow.operations.audit(actor=actor, action="credential.delete", resource_type="managed_credential", resource_id=row.id, detail={"provider": row.provider, "name": row.name})
            uow.session.delete(row)
            return True

    def reveal(self, *, credential_id: Optional[str] = None, provider: Optional[str] = None) -> Optional[str]:
        with self._uow_factory() as uow:
            query = uow.session.query(ManagedCredential).filter(ManagedCredential.active.is_(True))
            if credential_id:
                query = query.filter(ManagedCredential.id == credential_id)
            elif provider:
                query = query.filter(ManagedCredential.provider == provider.strip().lower())
            else:
                return None
            row = query.order_by(ManagedCredential.updated_at.desc()).first()
            if row is None or (row.expires_at and row.expires_at <= utcnow()):
                return None
            secret_value = self._decrypt(row.ciphertext)
            row.last_used_at = utcnow()
            return secret_value


def _device_rule_public(row: DeviceClientRule) -> dict[str, Any]:
    return {column: getattr(row, column) for column in (
        "id", "name", "pattern", "match_type", "action", "enabled", "built_in",
        "priority", "hit_count", "notes", "revision", "created_at", "updated_at",
    )}


class DeviceRuleService:
    ALLOWED_ACTIONS = {"allow", "block", "observe"}
    ALLOWED_MATCH_TYPES = {"exact", "contains", "glob", "regex"}

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(self) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            rows = uow.session.query(DeviceClientRule).order_by(DeviceClientRule.priority, DeviceClientRule.id).all()
            return [_device_rule_public(row) for row in rows]

    def save(self, data: dict[str, Any], actor: Actor, rule_id: Optional[int] = None) -> dict[str, Any]:
        action = str(data.get("action") or "allow")
        match_type = str(data.get("match_type") or "contains")
        pattern = str(data.get("pattern") or "").strip()
        if action not in self.ALLOWED_ACTIONS or match_type not in self.ALLOWED_MATCH_TYPES:
            raise ValueError("无效的规则动作或匹配方式")
        if not pattern or len(pattern) > 255:
            raise ValueError("客户端匹配内容长度必须为 1-255")
        if match_type == "regex":
            re.compile(pattern)
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.session.query(DeviceClientRule).filter(DeviceClientRule.id == rule_id).with_for_update().first() if rule_id else None
            if rule_id and row is None:
                raise LookupError("设备规则不存在")
            if row is None:
                row = DeviceClientRule(created_at=now, built_in=False, hit_count=0)
                uow.session.add(row)
            elif data.get("revision") is not None and int(data["revision"]) != int(row.revision):
                raise RuntimeError("规则已发生变化，请刷新后重试")
            row.name = str(data.get("name") or pattern).strip()[:120]
            row.pattern = pattern
            row.match_type = match_type
            row.action = action
            row.enabled = bool(data.get("enabled", True))
            row.priority = int(data.get("priority", 100))
            row.notes = str(data.get("notes") or "").strip()[:500] or None
            row.revision = int(row.revision or 1) + (1 if rule_id else 0)
            row.updated_at = now
            uow.operations.audit(actor=actor, action="device_rule.update" if rule_id else "device_rule.create", resource_type="device_client_rule", resource_id=str(row.id or "new"), detail={"action": action, "pattern": pattern})
            uow.flush()
            return _device_rule_public(row)

    def delete(self, rule_id: int, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.session.query(DeviceClientRule).filter(DeviceClientRule.id == rule_id).with_for_update().first()
            if row is None:
                return False
            if row.built_in:
                raise RuntimeError("内置规则只能停用，不能删除")
            uow.operations.audit(actor=actor, action="device_rule.delete", resource_type="device_client_rule", resource_id=str(rule_id))
            uow.session.delete(row)
            return True

    @staticmethod
    def _matches(rule: DeviceClientRule, client_name: str) -> bool:
        value = client_name.casefold()
        pattern = rule.pattern.casefold()
        if rule.match_type == "exact":
            return value == pattern
        if rule.match_type == "glob":
            return fnmatch.fnmatch(value, pattern)
        if rule.match_type == "regex":
            return re.search(rule.pattern, client_name, flags=re.IGNORECASE) is not None
        return pattern in value

    def evaluate(self, client_name: str, *, record_hit: bool = False) -> dict[str, Any]:
        normalized = str(client_name or "")[:255]
        with self._uow_factory() as uow:
            rows = uow.session.query(DeviceClientRule).filter(DeviceClientRule.enabled.is_(True)).order_by(DeviceClientRule.priority, DeviceClientRule.id).all()
            matches = [row for row in rows if self._matches(row, normalized)]
            selected = matches[0] if matches else None
            if selected and record_hit:
                selected.hit_count = int(selected.hit_count or 0) + 1
                selected.updated_at = utcnow()
            return {"client_name": normalized, "action": selected.action if selected else "observe", "matched": _device_rule_public(selected) if selected else None}


def _emby_public(row: EmbyInstance, bindings: int = 0) -> dict[str, Any]:
    return {**{column: getattr(row, column) for column in (
        "id", "name", "base_url", "credential_id", "enabled", "is_default", "verify_tls",
        "priority", "status", "last_error", "last_latency_ms", "last_checked_at", "revision",
        "created_at", "updated_at",
    )}, "binding_count": bindings}


class MultiEmbyService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory
        self._credentials = CredentialService(uow_factory)

    def list(self) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            rows = uow.session.query(EmbyInstance).order_by(EmbyInstance.priority, EmbyInstance.name).all()
            counts = dict(uow.session.query(AccountEmbyBinding.instance_id, __import__("sqlalchemy").func.count(AccountEmbyBinding.id)).group_by(AccountEmbyBinding.instance_id).all())
            return [_emby_public(row, int(counts.get(row.id, 0))) for row in rows]

    def has_enabled_instances(self) -> bool:
        with self._uow_factory() as uow:
            return bool(uow.session.query(EmbyInstance.id).filter(EmbyInstance.enabled.is_(True)).first())

    def feature_enabled(self) -> bool:
        """Return the runtime switch without making administration impossible.

        Instance CRUD and manual probes remain available while disabled, but
        registration and playback aggregation use this switch before routing
        production traffic to managed instances.
        """
        try:
            from bot.application.governance_service import DynamicSettingsService

            return bool(
                DynamicSettingsService(self._uow_factory)
                .get("integrations.multi_emby_enabled")["value"]
            )
        except Exception:
            return True

    def save(self, data: dict[str, Any], actor: Actor, instance_id: Optional[str] = None) -> dict[str, Any]:
        name = str(data.get("name") or "").strip()
        base_url = str(data.get("base_url") or "").strip().rstrip("/")
        credential_id = str(data.get("credential_id") or "")
        if not name or not re.match(r"^https?://", base_url, flags=re.IGNORECASE):
            raise ValueError("实例名称和 http(s) 地址不能为空")
        if not credential_id:
            raise ValueError("请选择 Emby API 凭据")
        now = utcnow()
        with self._uow_factory() as uow:
            if not uow.session.query(ManagedCredential.id).filter(
                ManagedCredential.id == credential_id,
                ManagedCredential.provider == "emby",
                ManagedCredential.active.is_(True),
            ).first():
                raise ValueError("Emby API 凭据不存在或已停用")
            row = uow.session.query(EmbyInstance).filter(EmbyInstance.id == instance_id).with_for_update().first() if instance_id else None
            if instance_id and row is None:
                raise LookupError("Emby 实例不存在")
            if row is None:
                row = EmbyInstance(id=str(uuid4()), created_at=now, status="unknown")
                uow.session.add(row)
            elif data.get("revision") is not None and int(data["revision"]) != int(row.revision):
                raise RuntimeError("Emby 实例已被其他管理员修改，请刷新后重试")
            if data.get("is_default"):
                uow.session.query(EmbyInstance).filter(EmbyInstance.id != row.id).update({EmbyInstance.is_default: False}, synchronize_session=False)
            row.name = name
            row.base_url = base_url
            row.credential_id = credential_id
            row.enabled = bool(data.get("enabled", True))
            row.is_default = bool(data.get("is_default", False))
            row.verify_tls = bool(data.get("verify_tls", True))
            row.priority = int(data.get("priority", 100))
            row.revision = int(row.revision or 1) + (1 if instance_id else 0)
            row.updated_at = now
            uow.operations.audit(actor=actor, action="emby_instance.update" if instance_id else "emby_instance.create", resource_type="emby_instance", resource_id=row.id, detail={"name": name, "base_url": base_url, "is_default": row.is_default})
            uow.flush()
            return _emby_public(row)

    def delete(self, instance_id: str, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.session.query(EmbyInstance).filter(EmbyInstance.id == instance_id).with_for_update().first()
            if row is None:
                return False
            if uow.session.query(AccountEmbyBinding.id).filter(AccountEmbyBinding.instance_id == instance_id).first():
                raise RuntimeError("实例已有账号绑定，只能停用，不能删除")
            uow.operations.audit(actor=actor, action="emby_instance.delete", resource_type="emby_instance", resource_id=instance_id)
            uow.session.delete(row)
            return True

    def _instance(self, instance_id: Optional[str] = None, *, require_enabled: bool = True) -> Optional[EmbyInstance]:
        with self._uow_factory() as uow:
            query = uow.session.query(EmbyInstance)
            if require_enabled:
                query = query.filter(EmbyInstance.enabled.is_(True))
            if instance_id:
                return query.filter(EmbyInstance.id == instance_id).first()
            return query.order_by(EmbyInstance.is_default.desc(), EmbyInstance.priority, EmbyInstance.id).first()

    async def _request(self, instance: EmbyInstance, method: str, path: str, json_body: Optional[dict[str, Any]] = None) -> tuple[int, Any, int]:
        token = self._credentials.reveal(credential_id=instance.credential_id)
        if not token:
            raise RuntimeError("Emby API 凭据不可用")
        headers = {"X-Emby-Token": token, "Accept": "application/json", "Content-Type": "application/json"}
        started = time.monotonic()
        timeout = aiohttp.ClientTimeout(total=15)
        async with aiohttp.ClientSession(timeout=timeout, headers=headers) as session:
            async with session.request(method, f"{instance.base_url}{path}", json=json_body, ssl=instance.verify_tls) as response:
                latency = int((time.monotonic() - started) * 1000)
                body = await response.json(content_type=None) if response.status != 204 and response.content_length != 0 else None
                return response.status, body, latency

    async def probe(self, instance_id: str) -> dict[str, Any]:
        instance = self._instance(instance_id)
        if instance is None:
            raise LookupError("Emby 实例不存在或已停用")
        status = "unhealthy"
        error = None
        latency = None
        try:
            code, body, latency = await self._request(instance, "GET", "/emby/System/Info")
            if code == 200 and isinstance(body, dict):
                status = "healthy"
            else:
                error = f"HTTP {code}"
        except Exception as exc:
            error = str(exc)[:500]
        with self._uow_factory() as uow:
            row = uow.session.query(EmbyInstance).filter(EmbyInstance.id == instance.id).with_for_update().first()
            row.status = status
            row.last_error = error
            row.last_latency_ms = latency
            row.last_checked_at = utcnow()
            row.updated_at = row.last_checked_at
            uow.operations.event("emby.probed", "emby_instance", row.id, {"status": status, "latency_ms": latency, "message": error})
            uow.flush()
            return _emby_public(row)

    async def probe_all(self) -> dict[str, Any]:
        results = []
        for item in self.list():
            if item["enabled"]:
                try:
                    results.append(await self.probe(item["id"]))
                except Exception as exc:
                    results.append({**item, "status": "unhealthy", "last_error": str(exc)})
        return {"items": results, "healthy": sum(item.get("status") == "healthy" for item in results), "total": len(results)}

    async def create_user(self, *, account_id: str, username: str, days: int, instance_id: Optional[str] = None) -> Optional[tuple[str, str, datetime, str]]:
        instance = self._instance(instance_id)
        if instance is None:
            return None
        expires_at = utcnow() + timedelta(days=days)
        password = secrets.token_urlsafe(9)
        code, user, _latency = await self._request(instance, "POST", "/emby/Users/New", {"Name": username})
        if code not in (200, 201) or not isinstance(user, dict) or not user.get("Id"):
            raise RuntimeError(f"Emby 创建账号失败：HTTP {code}")
        emby_id = str(user["Id"])
        try:
            code, _body, _latency = await self._request(instance, "POST", f"/emby/Users/{emby_id}/Password", {"Id": emby_id, "NewPw": password})
            if code not in (200, 204):
                raise RuntimeError(f"Emby 设置密码失败：HTTP {code}")
            policy = {"IsAdministrator": False, "IsDisabled": False, "EnableMediaPlayback": True, "EnableRemoteAccess": True, "EnableAllDevices": True, "EnableContentDownloading": False, "SimultaneousStreamLimit": 2}
            code, _body, _latency = await self._request(instance, "POST", f"/emby/Users/{emby_id}/Policy", policy)
            if code not in (200, 204):
                raise RuntimeError(f"Emby 设置策略失败：HTTP {code}")
        except Exception:
            try:
                await self._request(instance, "DELETE", f"/emby/Users/{emby_id}")
            finally:
                raise
        with self._uow_factory() as uow:
            has_primary = uow.session.query(AccountEmbyBinding.id).filter(
                AccountEmbyBinding.account_id == account_id,
                AccountEmbyBinding.is_primary.is_(True),
                AccountEmbyBinding.status != "deleted",
            ).first()
            binding = AccountEmbyBinding(
                id=str(uuid4()), account_id=account_id, instance_id=instance.id,
                emby_user_id=emby_id, emby_username=username, status="active",
                is_primary=not bool(has_primary), expires_at=expires_at,
                last_synced_at=utcnow(), created_at=utcnow(), updated_at=utcnow(),
            )
            uow.session.add(binding)
            uow.operations.event("emby.binding_created", "account", account_id, {"instance_id": instance.id, "emby_user_id": emby_id, "username": username})
        return emby_id, password, expires_at, instance.id

    async def delete_user(self, *, instance_id: str, emby_user_id: str) -> bool:
        instance = self._instance(instance_id, require_enabled=False)
        if instance is None:
            return False
        code, _body, _latency = await self._request(instance, "DELETE", f"/emby/Users/{emby_user_id}")
        return code in (200, 204)

    def bindings_for_account(self, account_id: str) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            rows = uow.session.query(AccountEmbyBinding).filter(
                AccountEmbyBinding.account_id == account_id,
                AccountEmbyBinding.status.in_(("active", "suspended")),
            ).all()
            return [{
                "id": row.id, "account_id": row.account_id, "instance_id": row.instance_id,
                "emby_user_id": row.emby_user_id, "emby_username": row.emby_username,
                "status": row.status, "is_primary": bool(row.is_primary), "expires_at": row.expires_at,
            } for row in rows]

    async def set_user_disabled(self, *, instance_id: str, emby_user_id: str, disabled: bool) -> bool:
        instance = self._instance(instance_id, require_enabled=False)
        if instance is None:
            return False
        code, user, _latency = await self._request(instance, "GET", f"/emby/Users/{emby_user_id}")
        if code != 200 or not isinstance(user, dict):
            return False
        policy = dict(user.get("Policy") or {})
        policy["IsDisabled"] = bool(disabled)
        code, _body, _latency = await self._request(instance, "POST", f"/emby/Users/{emby_user_id}/Policy", policy)
        if code not in (200, 204):
            return False
        with self._uow_factory() as uow:
            row = uow.session.query(AccountEmbyBinding).filter(
                AccountEmbyBinding.instance_id == instance_id,
                AccountEmbyBinding.emby_user_id == emby_user_id,
            ).with_for_update().first()
            if row:
                row.status = "suspended" if disabled else "active"
                row.last_synced_at = utcnow()
                row.updated_at = row.last_synced_at
        return True

    def adopt_legacy_bindings(self, instance_id: str, actor: Actor) -> dict[str, Any]:
        with self._uow_factory() as uow:
            instance = uow.session.query(EmbyInstance).filter(EmbyInstance.id == instance_id).first()
            if instance is None:
                raise LookupError("Emby 实例不存在")
            rows = uow.session.query(Account, LegacyEmby).join(LegacyEmby, LegacyEmby.tg == Account.legacy_tg).filter(LegacyEmby.embyid.isnot(None)).all()
            created = 0
            skipped = 0
            for account, legacy in rows:
                exists = uow.session.query(AccountEmbyBinding.id).filter(
                    AccountEmbyBinding.account_id == account.id,
                    AccountEmbyBinding.instance_id == instance_id,
                ).first()
                if exists:
                    skipped += 1
                    continue
                has_binding = uow.session.query(AccountEmbyBinding.id).filter(AccountEmbyBinding.account_id == account.id).first()
                uow.session.add(AccountEmbyBinding(
                    id=str(uuid4()), account_id=account.id, instance_id=instance_id,
                    emby_user_id=str(legacy.embyid), emby_username=str(legacy.name or legacy.embyid),
                    status="suspended" if legacy.lv == "c" else "active", is_primary=not bool(has_binding),
                    expires_at=legacy.ex, last_synced_at=utcnow(), created_at=utcnow(), updated_at=utcnow(),
                ))
                created += 1
            uow.operations.audit(actor=actor, action="emby_instance.adopt_legacy", resource_type="emby_instance", resource_id=instance_id, detail={"created": created, "skipped": skipped})
            return {"created": created, "skipped": skipped, "total": len(rows)}


class _AggregateResult:
    def __init__(self, success: bool, data: Any = None, error: Optional[str] = None):
        self.success = success
        self.data = data
        self.error = error


class MultiEmbyAggregateClient:
    """Core operations adapter that merges live sessions from all instances."""

    def __init__(self, service: Optional[MultiEmbyService] = None):
        self.service = service or MultiEmbyService()

    async def _request(self, method: str, path: str) -> _AggregateResult:
        if method.upper() != "GET" or path != "/emby/Sessions":
            return _AggregateResult(False, error="Unsupported aggregate request")
        sessions: list[dict[str, Any]] = []
        errors = []
        for item in self.service.list():
            if not item["enabled"]:
                continue
            instance = self.service._instance(item["id"])
            if instance is None:
                continue
            try:
                code, payload, _latency = await self.service._request(instance, "GET", "/emby/Sessions")
                if code != 200 or not isinstance(payload, list):
                    errors.append(f"{item['name']}: HTTP {code}")
                    continue
                for original in payload:
                    session = dict(original)
                    session["Id"] = f"{item['id']}:{original.get('Id', '')}"
                    session["UserId"] = f"{item['id']}:{original.get('UserId', '')}"
                    session["DeviceId"] = f"{item['id']}:{original.get('DeviceId', '')}"
                    session["ServerName"] = item["name"]
                    sessions.append(session)
            except Exception as exc:
                errors.append(f"{item['name']}: {exc}")
        return _AggregateResult(bool(sessions) or not errors, sessions, "; ".join(errors) or None)

    async def terminate_session(self, session_id: str, reason: str) -> bool:
        if ":" not in session_id:
            return False
        instance_id, original_id = session_id.split(":", 1)
        instance = self.service._instance(instance_id, require_enabled=False)
        if instance is None:
            return False
        stop_code, _body, _latency = await self.service._request(instance, "POST", f"/emby/Sessions/{original_id}/Playing/Stop")
        message_code, _body, _latency = await self.service._request(instance, "POST", f"/emby/Sessions/{original_id}/Message", {"Header": "Sakura 安全提醒", "Text": reason, "TimeoutMs": 10000})
        return stop_code in (200, 204) or message_code in (200, 204)


def _media_public(row: MediaCatalogItem) -> dict[str, Any]:
    payload = _load_json(row.payload_json, {})
    image_base = "https://image.tmdb.org/t/p/"
    return {
        "provider": row.provider, "media_type": row.media_type, "provider_id": row.provider_id,
        "external_ref": f"{row.provider}:{row.media_type}:{row.provider_id}", "title": row.title,
        "original_title": row.original_title, "year": row.year, "overview": row.overview,
        "poster_url": f"{image_base}w500{row.poster_path}" if row.poster_path else None,
        "backdrop_url": f"{image_base}w1280{row.backdrop_path}" if row.backdrop_path else None,
        "vote_average": float(row.vote_average or 0), "genres": payload.get("genres", []),
        "cached_until": row.cached_until,
    }


class MediaCatalogService:
    TMDB_BASE = "https://api.themoviedb.org/3"

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory
        self._credentials = CredentialService(uow_factory)

    def _options(self) -> dict[str, Any]:
        try:
            from bot.application.governance_service import DynamicSettingsService
            settings = DynamicSettingsService(self._uow_factory)
            return {
                "enabled": bool(settings.get("tmdb.enabled")["value"]),
                "language": str(settings.get("tmdb.language")["value"]),
                "region": str(settings.get("tmdb.region")["value"]),
                "include_adult": bool(settings.get("tmdb.include_adult")["value"]),
                "cache_minutes": int(settings.get("tmdb.cache_minutes")["value"]),
            }
        except Exception:
            return {"enabled": False, "language": "zh-CN", "region": "CN", "include_adult": False, "cache_minutes": 60}

    def _cached_search(self, query: str, media_type: Optional[str], limit: int) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            stmt = uow.session.query(MediaCatalogItem).filter(MediaCatalogItem.cached_until > utcnow(), or_(MediaCatalogItem.title.ilike(f"%{query}%"), MediaCatalogItem.original_title.ilike(f"%{query}%")))
            if media_type in {"movie", "tv"}:
                stmt = stmt.filter(MediaCatalogItem.media_type == media_type)
            return [_media_public(row) for row in stmt.order_by(MediaCatalogItem.updated_at.desc()).limit(limit).all()]

    async def search(self, query: str, media_type: Optional[str] = None, limit: int = 20) -> dict[str, Any]:
        query = str(query or "").strip()
        if len(query) < 1:
            return {"items": [], "source": "cache"}
        options = self._options()
        cached = self._cached_search(query, media_type, limit)
        token = self._credentials.reveal(provider="tmdb") if options["enabled"] else None
        if not token:
            return {"items": cached, "source": "cache", "configured": False}
        endpoint = f"/search/{media_type}" if media_type in {"movie", "tv"} else "/search/multi"
        params = {"query": query, "language": options["language"], "region": options["region"], "include_adult": str(options["include_adult"]).lower()}
        headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
        try:
            async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=15), headers=headers) as session:
                async with session.get(f"{self.TMDB_BASE}{endpoint}", params=params) as response:
                    if response.status != 200:
                        raise RuntimeError(f"TMDB HTTP {response.status}")
                    payload = await response.json()
            items = [item for item in payload.get("results", []) if (item.get("media_type") or media_type) in {"movie", "tv"}][:limit]
            result = self._cache_items(items, options["cache_minutes"], media_type)
            return {"items": result, "source": "tmdb", "configured": True}
        except Exception as exc:
            return {"items": cached, "source": "cache", "configured": True, "warning": str(exc)}

    def _cache_items(self, items: list[dict[str, Any]], cache_minutes: int, forced_type: Optional[str] = None) -> list[dict[str, Any]]:
        now = utcnow()
        expires = now + timedelta(minutes=cache_minutes)
        output = []
        with self._uow_factory() as uow:
            for item in items:
                media_type = str(item.get("media_type") or forced_type or "movie")
                provider_id = str(item.get("id"))
                row = uow.session.query(MediaCatalogItem).filter(MediaCatalogItem.provider == "tmdb", MediaCatalogItem.media_type == media_type, MediaCatalogItem.provider_id == provider_id).first()
                if row is None:
                    row = MediaCatalogItem(provider="tmdb", media_type=media_type, provider_id=provider_id, created_at=now)
                    uow.session.add(row)
                date_value = item.get("release_date") or item.get("first_air_date") or ""
                row.title = str(item.get("title") or item.get("name") or item.get("original_title") or item.get("original_name") or provider_id)[:255]
                row.original_title = str(item.get("original_title") or item.get("original_name") or "")[:255] or None
                row.year = int(date_value[:4]) if str(date_value)[:4].isdigit() else None
                row.overview = item.get("overview")
                row.poster_path = item.get("poster_path")
                row.backdrop_path = item.get("backdrop_path")
                row.vote_average = str(item.get("vote_average") or 0)
                row.payload_json = _dump_json(item)
                row.cached_until = expires
                row.updated_at = now
                uow.flush()
                output.append(_media_public(row))
        return output

    async def trending(self, limit: int = 20) -> dict[str, Any]:
        options = self._options()
        token = self._credentials.reveal(provider="tmdb") if options["enabled"] else None
        if not token:
            with self._uow_factory() as uow:
                rows = uow.session.query(MediaCatalogItem).order_by(MediaCatalogItem.updated_at.desc()).limit(limit).all()
                return {"items": [_media_public(row) for row in rows], "source": "cache", "configured": False}
        headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
        try:
            async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=15), headers=headers) as session:
                async with session.get(f"{self.TMDB_BASE}/trending/all/week", params={"language": options["language"]}) as response:
                    if response.status != 200:
                        raise RuntimeError(f"TMDB HTTP {response.status}")
                    payload = await response.json()
            return {"items": self._cache_items(payload.get("results", [])[:limit], options["cache_minutes"]), "source": "tmdb", "configured": True}
        except Exception as exc:
            with self._uow_factory() as uow:
                rows = (
                    uow.session.query(MediaCatalogItem)
                    .order_by(MediaCatalogItem.updated_at.desc())
                    .limit(limit)
                    .all()
                )
                cached = [_media_public(row) for row in rows]
            return {"items": cached, "source": "cache", "configured": True, "warning": str(exc)}


class MoviePilotGateway:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory
        self._credentials = CredentialService(uow_factory)

    def _base_url(self) -> str:
        try:
            from bot.application.governance_service import DynamicSettingsService
            return str(
                DynamicSettingsService(self._uow_factory)
                .get("integrations.moviepilot_url")["value"]
            ).rstrip("/")
        except Exception:
            try:
                import bot
                return str(bot.moviepilot.url).rstrip("/")
            except Exception:
                return ""

    def _auth(self) -> tuple[str, dict[str, str]]:
        base = self._base_url()
        token = self._credentials.reveal(provider="moviepilot")
        if token:
            return base, {"Authorization": token if token.lower().startswith("bearer ") else f"Bearer {token}"}
        try:
            import bot
            legacy = str(bot.moviepilot.access_token or "")
            return base, {"Authorization": legacy} if legacy else {}
        except Exception:
            return base, {}

    async def search(self, title: str) -> dict[str, Any]:
        base, headers = self._auth()
        if not base:
            return {"items": [], "configured": False}
        async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=30), headers=headers) as session:
            async with session.get(f"{base}/api/v1/search/title", params={"keyword": title}) as response:
                payload = await response.json(content_type=None)
                if response.status >= 400:
                    raise RuntimeError(f"MoviePilot HTTP {response.status}")
        return {"items": payload.get("data", []) if isinstance(payload, dict) else [], "configured": True}

    async def submit(self, request_item: dict[str, Any], resource: dict[str, Any]) -> dict[str, Any]:
        base, headers = self._auth()
        if not base:
            raise RuntimeError("MoviePilot 尚未配置")
        headers = {**headers, "Content-Type": "application/json"}
        torrent_info = resource.get("torrent_info") if isinstance(resource.get("torrent_info"), dict) else resource
        download_payload = {**torrent_info, "torrent_in": torrent_info}
        async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=30), headers=headers) as session:
            async with session.post(f"{base}/api/v1/download/add", json=download_payload) as response:
                payload = await response.json(content_type=None)
                if response.status >= 400 or not isinstance(payload, dict) or not payload.get("success"):
                    raise RuntimeError(f"MoviePilot 提交失败：HTTP {response.status}")
        download_id = str((payload.get("data") or {}).get("download_id") or "")
        if not download_id:
            raise RuntimeError("MoviePilot 未返回下载任务 ID")
        from bot.application.commerce_service import MediaRequestService
        updated = MediaRequestService(self._uow_factory).update(
            request_item["id"],
            data={"status": "downloading", "download_id": download_id, "progress": 0},
            actor=Actor.system("moviepilot-gateway"),
        )
        return updated or request_item


def _automation_public(row: AutomationRule) -> dict[str, Any]:
    return {
        "id": row.id, "name": row.name, "description": row.description, "trigger_type": row.trigger_type,
        "trigger_value": row.trigger_value, "conditions": _load_json(row.conditions_json, {}),
        "actions": _load_json(row.actions_json, []), "enabled": bool(row.enabled),
        "cooldown_seconds": row.cooldown_seconds, "last_cursor": row.last_cursor,
        "last_run_at": row.last_run_at, "revision": row.revision,
        "created_at": row.created_at, "updated_at": row.updated_at,
    }


class AutomationService:
    ALLOWED_ACTIONS = {"enqueue_task", "telegram_alert", "create_risk_event"}

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(self) -> dict[str, Any]:
        with self._uow_factory() as uow:
            rules = uow.session.query(AutomationRule).order_by(AutomationRule.created_at.desc()).all()
            runs = uow.session.query(AutomationRun).order_by(AutomationRun.created_at.desc()).limit(100).all()
            return {"items": [_automation_public(row) for row in rules], "runs": [{
                "id": row.id, "rule_id": row.rule_id, "event_id": row.event_id, "status": row.status,
                "action_results": _load_json(row.action_results_json, []), "error_message": row.error_message,
                "started_at": row.started_at, "finished_at": row.finished_at, "created_at": row.created_at,
            } for row in runs]}

    def save(self, data: dict[str, Any], actor: Actor, rule_id: Optional[str] = None) -> dict[str, Any]:
        actions = data.get("actions") or []
        if not isinstance(actions, list) or not actions:
            raise ValueError("自动化至少需要一个动作")
        if any(not isinstance(item, dict) or item.get("type") not in self.ALLOWED_ACTIONS for item in actions):
            raise ValueError("自动化包含不受支持的动作")
        trigger_type = str(data.get("trigger_type") or "event")
        if trigger_type not in {"event", "interval"}:
            raise ValueError("触发方式仅支持 event 或 interval")
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.session.query(AutomationRule).filter(AutomationRule.id == rule_id).with_for_update().first() if rule_id else None
            if rule_id and row is None:
                raise LookupError("自动化规则不存在")
            if row is None:
                row = AutomationRule(id=str(uuid4()), last_cursor=0, created_at=now)
                uow.session.add(row)
            elif data.get("revision") is not None and int(data["revision"]) != int(row.revision):
                raise RuntimeError("自动化规则已被其他管理员修改，请刷新后重试")
            row.name = str(data.get("name") or "未命名自动化").strip()[:120]
            row.description = str(data.get("description") or "").strip()[:500] or None
            row.trigger_type = trigger_type
            row.trigger_value = str(data.get("trigger_value") or "*").strip()[:255]
            row.conditions_json = _dump_json(data.get("conditions") or {})
            row.actions_json = _dump_json(actions)
            row.enabled = bool(data.get("enabled", True))
            row.cooldown_seconds = max(0, min(int(data.get("cooldown_seconds", 0)), 604800))
            row.revision = int(row.revision or 1) + (1 if rule_id else 0)
            row.updated_at = now
            uow.operations.audit(actor=actor, action="automation.update" if rule_id else "automation.create", resource_type="automation_rule", resource_id=row.id, detail={"trigger_type": trigger_type, "trigger_value": row.trigger_value})
            uow.flush()
            return _automation_public(row)

    def delete(self, rule_id: str, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.session.query(AutomationRule).filter(AutomationRule.id == rule_id).first()
            if row is None:
                return False
            uow.operations.audit(actor=actor, action="automation.delete", resource_type="automation_rule", resource_id=rule_id)
            uow.session.delete(row)
            return True

    @staticmethod
    def _event_matches(pattern: str, event_type: str) -> bool:
        return fnmatch.fnmatch(event_type, pattern)

    @staticmethod
    def _conditions_match(conditions: dict[str, Any], event: SystemEvent) -> bool:
        if not conditions:
            return True
        payload = _load_json(event.payload_json, {})
        for key, expected in conditions.items():
            actual = payload.get(key) if key not in {"aggregate_type", "aggregate_id"} else getattr(event, key)
            if isinstance(expected, list):
                if actual not in expected:
                    return False
            elif actual != expected:
                return False
        return True

    async def evaluate(self) -> dict[str, Any]:
        processed = 0
        created = 0
        with self._uow_factory() as uow:
            rules = uow.session.query(AutomationRule).filter(AutomationRule.enabled.is_(True)).all()
            snapshots = [_automation_public(row) for row in rules]
        for rule in snapshots:
            if rule["trigger_type"] == "interval":
                seconds = max(60, int(rule["trigger_value"] or 60))
                if rule["last_run_at"] and rule["last_run_at"] > utcnow() - timedelta(seconds=seconds):
                    continue
                events = [None]
            else:
                with self._uow_factory() as uow:
                    events = uow.session.query(SystemEvent).filter(SystemEvent.id > int(rule["last_cursor"] or 0)).order_by(SystemEvent.id).limit(500).all()
            for event in events:
                processed += 1
                if event is not None and (not self._event_matches(rule["trigger_value"], event.event_type) or not self._conditions_match(rule["conditions"], event)):
                    continue
                if rule["last_run_at"] and rule["cooldown_seconds"] and rule["last_run_at"] > utcnow() - timedelta(seconds=rule["cooldown_seconds"]):
                    continue
                if await self._run(rule, event):
                    created += 1
            with self._uow_factory() as uow:
                row = uow.session.query(AutomationRule).filter(AutomationRule.id == rule["id"]).with_for_update().first()
                if row and events and events[-1] is not None:
                    row.last_cursor = max(row.last_cursor, int(events[-1].id))
                    row.updated_at = utcnow()
        return {"processed": processed, "runs_created": created}

    async def _run(self, rule: dict[str, Any], event: Optional[SystemEvent]) -> bool:
        event_id = int(event.id) if event is not None else None
        run_id = str(uuid4())
        try:
            with self._uow_factory() as uow:
                if event_id is not None and uow.session.query(AutomationRun.id).filter(AutomationRun.rule_id == rule["id"], AutomationRun.event_id == event_id).first():
                    return False
                run = AutomationRun(id=run_id, rule_id=rule["id"], event_id=event_id, status="running", started_at=utcnow(), created_at=utcnow())
                uow.session.add(run)
                uow.flush()
        except IntegrityError:
            # Another worker already reserved this rule/event pair.
            return False
        results = []
        try:
            for action in rule["actions"]:
                results.append(await self._execute_action(action, event))
            status, error = "succeeded", None
        except Exception as exc:
            status, error = "failed", str(exc)[:1000]
        with self._uow_factory() as uow:
            run = uow.session.query(AutomationRun).filter(AutomationRun.id == run_id).first()
            if run is None:
                return False
            run.status = status
            run.action_results_json = _dump_json(results)
            run.error_message = error
            run.finished_at = utcnow()
            row = uow.session.query(AutomationRule).filter(AutomationRule.id == rule["id"]).with_for_update().first()
            row.last_run_at = utcnow()
            row.updated_at = row.last_run_at
        return True

    async def _execute_action(self, action: dict[str, Any], event: Optional[SystemEvent]) -> dict[str, Any]:
        action_type = action["type"]
        payload = _load_json(event.payload_json, {}) if event is not None else {}
        if action_type == "enqueue_task":
            task_type = str(action.get("task_type") or "")
            result = TaskService(self._uow_factory).enqueue(task_type=task_type, payload={**(action.get("payload") or {}), "source_event_id": getattr(event, "id", None)}, actor=Actor.system("automation"), idempotency_key=f"automation:{action.get('id', task_type)}:{getattr(event, 'id', utcnow().strftime('%Y%m%d%H%M'))}")
            if not result.ok:
                raise RuntimeError(f"任务入队失败：{result.status}")
            return {"type": action_type, "task_id": result.data["id"]}
        if action_type == "telegram_alert":
            recipients = action.get("recipients") or [item for item in os.getenv("SAKURA_ALERT_TELEGRAM_IDS", "").split(",") if item.strip()]
            tasks = []
            for recipient in recipients:
                result = TaskService(self._uow_factory).enqueue(task_type="notification.telegram", payload={"tg": int(recipient), "title": str(action.get("title") or "自动化告警"), "body": str(action.get("body") or payload or "系统事件触发")}, actor=Actor.system("automation"), idempotency_key=f"automation-alert:{rule_key(action)}:{getattr(event, 'id', 'interval')}:{recipient}")
                if result.ok:
                    tasks.append(result.data["id"])
            return {"type": action_type, "task_ids": tasks}
        if action_type == "create_risk_event":
            with self._uow_factory() as uow:
                item = uow.operations.security_event(
                    event_type=str(action.get("event_type") or "automation.risk"),
                    severity=str(action.get("severity") or "warning"),
                    subject_kind=getattr(event, "aggregate_type", None),
                    subject_id=getattr(event, "aggregate_id", None),
                    detail={"automation": True, "source_event": getattr(event, "event_type", None), **payload},
                )
                uow.flush()
                return {"type": action_type, "event_id": item.id}
        raise ValueError("不受支持的自动化动作")


def rule_key(action: dict[str, Any]) -> str:
    return hashlib.sha256(_dump_json(action).encode("utf-8")).hexdigest()[:12]


def _api_client_public(row: ApiClient) -> dict[str, Any]:
    return {"id": row.id, "name": row.name, "key_prefix": row.key_prefix, "scopes": _load_json(row.scopes_json, []), "active": bool(row.active), "expires_at": row.expires_at, "last_used_at": row.last_used_at, "last_ip": row.last_ip, "created_by": row.created_by, "created_at": row.created_at, "updated_at": row.updated_at}


class ApiClientService:
    ALLOWED_SCOPES = {"health:read", "media:read", "requests:read", "requests:create", "events:write"}

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(self) -> list[dict[str, Any]]:
        with self._uow_factory() as uow:
            return [_api_client_public(row) for row in uow.session.query(ApiClient).order_by(ApiClient.created_at.desc()).all()]

    def create(self, *, name: str, scopes: list[str], expires_at: Optional[datetime], actor: Actor) -> dict[str, Any]:
        normalized_scopes = sorted(set(scopes))
        if not normalized_scopes or not set(normalized_scopes).issubset(self.ALLOWED_SCOPES):
            raise ValueError("开放 API 权限范围无效")
        raw_key = f"sk_sakura_{secrets.token_urlsafe(32)}"
        prefix = raw_key[:15]
        now = utcnow()
        row = ApiClient(id=str(uuid4()), name=name.strip()[:120], key_prefix=prefix, key_hash=hashlib.sha256(raw_key.encode("utf-8")).hexdigest(), scopes_json=_dump_json(normalized_scopes), active=True, expires_at=_naive_utc(expires_at), created_by=actor.identifier, created_at=now, updated_at=now)
        with self._uow_factory() as uow:
            uow.session.add(row)
            uow.operations.audit(actor=actor, action="api_client.create", resource_type="api_client", resource_id=row.id, detail={"name": row.name, "scopes": normalized_scopes})
            uow.flush()
        return {**_api_client_public(row), "api_key": raw_key}

    def revoke(self, client_id: str, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.session.query(ApiClient).filter(ApiClient.id == client_id).with_for_update().first()
            if row is None:
                return False
            row.active = False
            row.updated_at = utcnow()
            uow.operations.audit(actor=actor, action="api_client.revoke", resource_type="api_client", resource_id=row.id)
            return True

    def authenticate(self, raw_key: str, required_scope: str, ip_address: Optional[str]) -> Optional[dict[str, Any]]:
        if not raw_key.startswith("sk_sakura_"):
            return None
        prefix = raw_key[:15]
        digest = hashlib.sha256(raw_key.encode("utf-8")).hexdigest()
        with self._uow_factory() as uow:
            row = uow.session.query(ApiClient).filter(ApiClient.key_prefix == prefix, ApiClient.active.is_(True)).with_for_update().first()
            if row is None or not hmac.compare_digest(row.key_hash, digest) or (row.expires_at and row.expires_at <= utcnow()):
                return None
            scopes = set(_load_json(row.scopes_json, []))
            if required_scope not in scopes:
                return None
            row.last_used_at = utcnow()
            row.last_ip = ip_address
            return _api_client_public(row)


class BackupService:
    def __init__(self, backup_dir: Optional[str] = None):
        if backup_dir is None:
            try:
                import bot
                backup_dir = bot.db_backup_dir
            except Exception:
                backup_dir = "./db_backup"
        self.backup_dir = Path(backup_dir).resolve()

    def list(self) -> dict[str, Any]:
        self.backup_dir.mkdir(parents=True, exist_ok=True)
        items = []
        for path in sorted(self.backup_dir.glob("*.sql"), key=lambda item: item.stat().st_mtime, reverse=True):
            stat = path.stat()
            items.append({"name": path.name, "size": stat.st_size, "created_at": datetime.fromtimestamp(stat.st_mtime), "sha256": self._hash(path)})
        return {"items": items, "directory": str(self.backup_dir), "total_size": sum(item["size"] for item in items)}

    @staticmethod
    def _hash(path: Path) -> str:
        digest = hashlib.sha256()
        with path.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
        return digest.hexdigest()

    def resolve(self, name: str) -> Path:
        if Path(name).name != name or not name.endswith(".sql"):
            raise ValueError("备份文件名无效")
        path = (self.backup_dir / name).resolve()
        if path.parent != self.backup_dir or not path.is_file():
            raise LookupError("备份文件不存在")
        return path
