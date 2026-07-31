import hmac
import json
import re
import secrets
from dataclasses import dataclass
from datetime import datetime, timedelta
from hashlib import sha256
from typing import Iterable, Optional
from uuid import uuid4

from bot.application.results import ServiceResult
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import WebLoginRequest, WebRole, WebSession, utcnow
from bot.sql_helper.sql_emby import Emby


DEFAULT_ROLE_PERMISSIONS = {
    "owner": {"*"},
    "admin": {
        "users:*",
        "codes:*",
        "partitions:*",
        "tasks:*",
        "audit:read",
        "security:read",
        "settings:read",
        "roles:read",
        "dashboard:read",
        "playback:*",
        "devices:*",
        "lines:*",
        "billing:*",
        "tickets:*",
        "requests:*",
        "reviews:*",
        "notifications:*",
        "roles:manage",
        "audit:export",
        "security:manage",
        "settings:manage",
    },
    "operator": {
        "users:read",
        "users:update",
        "codes:*",
        "partitions:*",
        "tasks:read",
        "dashboard:read",
        "playback:read",
        "playback:stop",
        "devices:read",
        "devices:update",
        "lines:read",
        "billing:read",
        "billing:update",
        "tickets:*",
        "requests:*",
        "reviews:*",
        "notifications:read",
        "notifications:send",
        "security:read",
    },
    "auditor": {
        "users:read",
        "tasks:read",
        "audit:read",
        "security:read",
        "dashboard:read",
        "playback:read",
        "devices:read",
        "lines:read",
        "billing:read",
        "tickets:read",
        "requests:read",
        "reviews:read",
        "notifications:read",
    },
    "user": {"self:*"},
}

PERMISSION_CATALOG = {
    "用户运营": {
        "users:*": "用户全部权限",
        "users:read": "查看用户",
        "users:update": "调整用户",
        "playback:*": "播放全部权限",
        "playback:read": "查看播放",
        "playback:stop": "终止播放",
        "devices:*": "设备全部权限",
        "devices:read": "查看设备",
        "devices:update": "管理设备",
    },
    "业务运营": {
        "codes:*": "兑换码管理",
        "partitions:*": "分区管理",
        "billing:*": "交易全部权限",
        "billing:read": "查看交易",
        "billing:update": "处理交易",
        "tickets:*": "工单全部权限",
        "tickets:read": "查看工单",
        "tickets:update": "处理工单",
        "requests:*": "求片全部权限",
        "requests:read": "查看求片",
        "requests:update": "处理求片",
        "reviews:*": "影评全部权限",
        "reviews:read": "查看影评",
        "reviews:update": "审核影评",
        "notifications:*": "通知全部权限",
        "notifications:read": "查看通知",
        "notifications:send": "发送通知",
    },
    "系统与安全": {
        "dashboard:read": "查看仪表盘",
        "lines:*": "线路全部权限",
        "lines:read": "查看线路",
        "lines:update": "管理线路",
        "tasks:read": "查看任务",
        "tasks:update": "执行任务",
        "tasks:*": "管理任务",
        "audit:read": "查看审计",
        "audit:export": "导出审计",
        "security:read": "查看安全事件",
        "security:manage": "处置安全事件",
        "roles:read": "查看角色",
        "roles:manage": "管理角色",
        "settings:read": "查看设置",
        "settings:manage": "修改设置",
    },
}
KNOWN_PERMISSIONS = {
    permission
    for permissions in PERMISSION_CATALOG.values()
    for permission in permissions
}


class TokenCodec:
    def __init__(self, secret: str):
        if len(secret) < 24:
            raise ValueError("Web session secret must contain at least 24 characters")
        self._secret = secret.encode("utf-8")

    def generate(self, length: int = 32) -> str:
        return secrets.token_urlsafe(length)

    def digest(self, raw_value: str) -> str:
        return hmac.new(self._secret, raw_value.encode("utf-8"), sha256).hexdigest()

    def verify(self, raw_value: str, expected_digest: str) -> bool:
        return hmac.compare_digest(self.digest(raw_value), expected_digest)


@dataclass(frozen=True)
class WebIdentity:
    session_id: str
    tg: int
    auth_method: str
    roles: tuple[str, ...]
    permissions: frozenset[str]
    csrf_hash: str

    @property
    def is_owner(self) -> bool:
        return "owner" in self.roles

    def has_permission(self, required: str) -> bool:
        if "*" in self.permissions or required in self.permissions:
            return True
        namespace = required.split(":", 1)[0]
        return f"{namespace}:*" in self.permissions


class WebAuthService:
    def __init__(
        self,
        *,
        token_codec: TokenCodec,
        owner_tg: int,
        admin_tg_ids: Iterable[int],
        session_ttl: timedelta = timedelta(days=7),
        login_ttl: timedelta = timedelta(minutes=5),
        uow_factory=SqlAlchemyUnitOfWork,
    ):
        self.codec = token_codec
        self.owner_tg = int(owner_tg)
        self.admin_tg_ids = {int(item) for item in admin_tg_ids}
        self.session_ttl = session_ttl
        self.login_ttl = login_ttl
        self._uow_factory = uow_factory

    def create_telegram_login(
        self,
        *,
        ip_address: str,
        requested_tg: Optional[int] = None,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            recent_count = uow.auth.recent_login_request_count(
                ip_address,
                now - timedelta(minutes=10),
            )
            if recent_count >= 10:
                uow.operations.security_event(
                    event_type="auth.telegram.rate_limited",
                    severity="warning",
                    subject_kind="ip",
                    subject_id=ip_address,
                    ip_address=ip_address,
                )
                return ServiceResult("rate_limited")

            raw_token = self.codec.generate()
            request_id = str(uuid4())
            expires_at = now + self.login_ttl
            uow.auth.add_login_request(
                WebLoginRequest(
                    id=request_id,
                    request_token_hash=self.codec.digest(raw_token),
                    status="pending",
                    requested_tg=requested_tg,
                    ip_address=ip_address,
                    created_at=now,
                    expires_at=expires_at,
                )
            )
            return ServiceResult(
                "ok",
                {
                    "request_id": request_id,
                    "request_token": raw_token,
                    "expires_at": expires_at,
                },
            )

    def claim_telegram_login(
        self,
        *,
        raw_token: str,
        tg: int,
        display_name: Optional[str] = None,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            request = uow.auth.get_login_request_by_hash(
                self.codec.digest(raw_token),
                for_update=True,
            )
            if not request:
                return ServiceResult("invalid_request")
            if request.expires_at <= now:
                request.status = "expired"
                return ServiceResult("expired")
            if request.status not in {"pending", "claimed"}:
                return ServiceResult(request.status)
            if request.requested_tg and request.requested_tg != tg:
                uow.operations.security_event(
                    event_type="auth.telegram.identity_mismatch",
                    severity="warning",
                    subject_kind="telegram",
                    subject_id=str(tg),
                    ip_address=request.ip_address,
                    detail={"request_id": request.id},
                )
                return ServiceResult("identity_mismatch")

            user = uow.users.get(tg)
            if not user and tg != self.owner_tg and tg not in self.admin_tg_ids:
                return ServiceResult("user_not_found")

            request.requested_tg = tg
            request.status = "claimed"
            request.claimed_at = now
            uow.operations.audit(
                actor=Actor.telegram(tg, display_name),
                action="auth.telegram.claim",
                resource_type="web_login_request",
                resource_id=request.id,
            )
            return ServiceResult("ok", {"request_id": request.id})

    def decide_telegram_login(
        self,
        *,
        request_id: str,
        tg: int,
        approve: bool,
        display_name: Optional[str] = None,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            request = uow.auth.get_login_request_by_id(request_id, for_update=True)
            if not request:
                return ServiceResult("invalid_request")
            if request.expires_at <= now:
                request.status = "expired"
                return ServiceResult("expired")
            if request.requested_tg != tg:
                return ServiceResult("identity_mismatch")
            if request.status not in {"claimed", "pending"}:
                return ServiceResult(request.status)

            request.status = "approved" if approve else "rejected"
            request.approved_tg = tg if approve else None
            request.approved_at = now if approve else None
            action = "auth.telegram.approve" if approve else "auth.telegram.reject"
            uow.operations.audit(
                actor=Actor.telegram(tg, display_name),
                action=action,
                resource_type="web_login_request",
                resource_id=request.id,
            )
            return ServiceResult("ok", {"approved": approve})

    def telegram_login_status(self, raw_token: str) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            request = uow.auth.get_login_request_by_hash(self.codec.digest(raw_token))
            if not request:
                return ServiceResult("invalid_request")
            if request.expires_at <= now and request.status not in {"consumed", "rejected"}:
                request.status = "expired"
            return ServiceResult(
                "ok",
                {
                    "status": request.status,
                    "expires_at": request.expires_at,
                },
            )

    def exchange_telegram_login(
        self,
        *,
        raw_token: str,
        user_agent: Optional[str],
        ip_address: str,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            request = uow.auth.get_login_request_by_hash(
                self.codec.digest(raw_token),
                for_update=True,
            )
            if not request:
                return ServiceResult("invalid_request")
            if request.expires_at <= now:
                request.status = "expired"
                return ServiceResult("expired")
            if request.status == "consumed":
                return ServiceResult("already_consumed")
            if request.status != "approved" or not request.approved_tg:
                return ServiceResult("not_approved")

            result = self._create_session(
                uow=uow,
                tg=request.approved_tg,
                auth_method="telegram",
                user_agent=user_agent,
                ip_address=ip_address,
                now=now,
            )
            request.status = "consumed"
            request.consumed_at = now
            uow.operations.audit(
                actor=Actor.telegram(request.approved_tg),
                action="auth.session.create",
                resource_type="web_session",
                resource_id=result.data["session_id"],
                detail={"auth_method": "telegram"},
            )
            return result

    def create_emby_session(
        self,
        *,
        embyid: str,
        username: str,
        user_agent: Optional[str],
        ip_address: str,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            user = uow.users.session.query(Emby).filter_by(embyid=embyid).first()
            if not user or user.name != username:
                return ServiceResult("user_not_found")
            result = self._create_session(
                uow=uow,
                tg=user.tg,
                auth_method="emby",
                user_agent=user_agent,
                ip_address=ip_address,
                now=now,
            )
            uow.operations.audit(
                actor=Actor(kind="emby", identifier=str(user.tg), display_name=username),
                action="auth.session.create",
                resource_type="web_session",
                resource_id=result.data["session_id"],
                detail={"auth_method": "emby"},
            )
            return result

    def _create_session(
        self,
        *,
        uow,
        tg: int,
        auth_method: str,
        user_agent: Optional[str],
        ip_address: str,
        now: datetime,
    ) -> ServiceResult:
        raw_session = self.codec.generate(48)
        raw_csrf = self.codec.generate(24)
        session_id = str(uuid4())
        expires_at = now + self.session_ttl
        uow.auth.add_session(
            WebSession(
                id=session_id,
                tg=tg,
                token_hash=self.codec.digest(raw_session),
                csrf_hash=self.codec.digest(raw_csrf),
                auth_method=auth_method,
                ip_address=ip_address,
                user_agent=(user_agent or "")[:512],
                created_at=now,
                last_seen_at=now,
                expires_at=expires_at,
            )
        )
        return ServiceResult(
            "ok",
            {
                "session_id": session_id,
                "session_token": raw_session,
                "csrf_token": raw_csrf,
                "expires_at": expires_at,
                "tg": tg,
                "auth_method": auth_method,
            },
        )

    def record_emby_login_failure(
        self,
        *,
        ip_address: str,
        username_fingerprint: str,
    ) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            recent = uow.auth.recent_security_event_count(
                "auth.emby.failed",
                ip_address,
                now - timedelta(minutes=10),
            )
            if recent >= 5:
                return ServiceResult("rate_limited")
            uow.operations.security_event(
                event_type="auth.emby.failed",
                severity="warning",
                subject_kind="emby_username",
                subject_id=username_fingerprint,
                ip_address=ip_address,
            )
            return ServiceResult("ok")

    def emby_login_allowed(self, ip_address: str) -> bool:
        now = utcnow()
        with self._uow_factory() as uow:
            recent = uow.auth.recent_security_event_count(
                "auth.emby.failed",
                ip_address,
                now - timedelta(minutes=10),
            )
            return recent < 5

    def authenticate(self, raw_session: str) -> Optional[WebIdentity]:
        now = utcnow()
        with self._uow_factory() as uow:
            session = uow.auth.get_session_by_hash(
                self.codec.digest(raw_session),
                for_update=False,
            )
            if not session or session.revoked_at or session.expires_at <= now:
                return None
            if not session.last_seen_at or session.last_seen_at <= now - timedelta(minutes=5):
                session.last_seen_at = now
            roles, permissions = self._resolve_roles(uow, session.tg)
            return WebIdentity(
                session_id=session.id,
                tg=session.tg,
                auth_method=session.auth_method,
                roles=tuple(sorted(roles)),
                permissions=frozenset(permissions),
                csrf_hash=session.csrf_hash,
            )

    def verify_csrf(self, identity: WebIdentity, raw_csrf: str) -> bool:
        return bool(raw_csrf) and self.codec.verify(raw_csrf, identity.csrf_hash)

    def logout(self, session_id: str, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            session = uow.auth.get_session_by_id(session_id, for_update=True)
            if not session or session.revoked_at:
                return False
            session.revoked_at = utcnow()
            uow.operations.audit(
                actor=actor,
                action="auth.session.revoke",
                resource_type="web_session",
                resource_id=session_id,
            )
            return True

    def logout_all(self, tg: int, actor: Actor) -> int:
        with self._uow_factory() as uow:
            revoked = uow.auth.revoke_user_sessions(tg, utcnow())
            uow.operations.audit(
                actor=actor,
                action="auth.session.revoke_all",
                resource_type="user",
                resource_id=str(tg),
                detail={"revoked_sessions": revoked},
            )
            return revoked

    def list_roles(self) -> list[dict]:
        with self._uow_factory() as uow:
            result = []
            registered_users = {
                int(row[0])
                for row in uow.users.session.query(Emby.tg).all()
            }
            for role in uow.auth.list_roles():
                members = uow.auth.role_member_tgs(role.id)
                if role.name == "owner":
                    members.add(self.owner_tg)
                elif role.name == "admin":
                    members.update(self.admin_tg_ids)
                elif role.name == "user":
                    members.update(registered_users)
                    members.add(self.owner_tg)
                    members.update(self.admin_tg_ids)
                result.append(
                    {
                        "id": role.id,
                        "name": role.name,
                        "permissions": sorted(self._permissions_for_role(role)),
                        "is_system": role.is_system,
                        "member_count": len(members),
                    }
                )
            return result

    def permission_catalog(self) -> list[dict]:
        return [
            {
                "group": group,
                "items": [
                    {"permission": permission, "label": label}
                    for permission, label in permissions.items()
                ],
            }
            for group, permissions in PERMISSION_CATALOG.items()
        ]

    def create_role(
        self,
        *,
        name: str,
        permissions: list[str],
        actor_tg: int,
    ) -> ServiceResult:
        normalized = name.strip().lower()
        if not re.fullmatch(r"[a-z][a-z0-9_-]{2,31}", normalized):
            return ServiceResult("invalid_name")
        if not set(permissions).issubset(KNOWN_PERMISSIONS):
            return ServiceResult("invalid_permissions")
        with self._uow_factory() as uow:
            if uow.auth.get_role_by_name(normalized):
                return ServiceResult("role_exists")
            role = WebRole(
                name=normalized,
                permissions_json=json.dumps(list(dict.fromkeys(permissions))),
                is_system=False,
            )
            uow.auth.add_role(role)
            uow.flush()
            uow.operations.audit(
                actor=Actor.web(actor_tg),
                action="role.create",
                resource_type="web_role",
                resource_id=str(role.id),
                detail={"name": normalized, "permissions": permissions},
            )
            return ServiceResult("ok", {"id": role.id, "name": role.name})

    def update_role(
        self,
        *,
        role_id: int,
        permissions: list[str],
        actor_tg: int,
    ) -> ServiceResult:
        if not set(permissions).issubset(KNOWN_PERMISSIONS):
            return ServiceResult("invalid_permissions")
        with self._uow_factory() as uow:
            role = uow.auth.get_role(role_id)
            if role is None:
                return ServiceResult("role_not_found")
            if role.name in {"owner", "user"}:
                return ServiceResult("protected_role")
            role.permissions_json = json.dumps(list(dict.fromkeys(permissions)))
            role.updated_at = utcnow()
            uow.operations.audit(
                actor=Actor.web(actor_tg),
                action="role.update",
                resource_type="web_role",
                resource_id=str(role.id),
                detail={"name": role.name, "permissions": permissions},
            )
            return ServiceResult("ok", {"id": role.id, "name": role.name})

    def delete_role(self, *, role_id: int, actor_tg: int) -> ServiceResult:
        with self._uow_factory() as uow:
            role = uow.auth.get_role(role_id)
            if role is None:
                return ServiceResult("role_not_found")
            if role.is_system:
                return ServiceResult("protected_role")
            members = int(uow.auth.role_member_count(role.id))
            if members:
                return ServiceResult("role_in_use", {"member_count": members})
            name = role.name
            uow.auth.delete_role(role)
            uow.operations.audit(
                actor=Actor.web(actor_tg),
                action="role.delete",
                resource_type="web_role",
                resource_id=str(role_id),
                detail={"name": name},
            )
            return ServiceResult("ok", {"name": name})

    def roles_for_user(self, tg: int) -> list[str]:
        with self._uow_factory() as uow:
            roles, _permissions = self._resolve_roles(uow, tg)
            return sorted(roles)

    def set_role(
        self,
        *,
        target_tg: int,
        role_name: str,
        enabled: bool,
        actor_tg: int,
    ) -> ServiceResult:
        with self._uow_factory() as uow:
            if not uow.users.get(target_tg):
                return ServiceResult("user_not_found")
            role = uow.auth.get_role_by_name(role_name)
            if not role or role_name in {"owner", "user"}:
                return ServiceResult("invalid_role")
            exists = uow.auth.has_role(target_tg, role.id)
            if enabled and not exists:
                uow.auth.add_role_member(target_tg, role.id, actor_tg)
            elif not enabled and exists:
                uow.auth.remove_role_member(target_tg, role.id)
            uow.operations.audit(
                actor=Actor.web(actor_tg),
                action="role.assign" if enabled else "role.remove",
                resource_type="user",
                resource_id=str(target_tg),
                detail={"role": role_name},
            )
            return ServiceResult("ok", {"changed": exists != enabled})

    def _resolve_roles(self, uow, tg: int):
        roles = {"user"}
        permissions = set(DEFAULT_ROLE_PERMISSIONS["user"])
        if tg == self.owner_tg:
            roles.add("owner")
            permissions.add("*")
        assigned_roles = uow.auth.get_roles_for_user(tg)
        if tg in self.admin_tg_ids:
            roles.add("admin")
            admin_role = next(
                (role for role in assigned_roles if role.name == "admin"),
                None,
            ) or uow.auth.get_role_by_name("admin")
            permissions.update(
                self._permissions_for_role(admin_role)
                if admin_role is not None
                else DEFAULT_ROLE_PERMISSIONS["admin"]
            )
        for role in assigned_roles:
            roles.add(role.name)
            permissions.update(self._permissions_for_role(role))
        return roles, permissions

    @staticmethod
    def _permissions_for_role(role: WebRole) -> set[str]:
        try:
            value = json.loads(role.permissions_json)
        except (TypeError, ValueError):
            value = None
        if isinstance(value, list) and all(isinstance(item, str) for item in value):
            return set(value)
        return set(DEFAULT_ROLE_PERMISSIONS.get(role.name, set()))
