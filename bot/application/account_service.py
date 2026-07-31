from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import re
from datetime import timedelta
from typing import Optional
from uuid import NAMESPACE_URL, uuid4, uuid5

from sqlalchemy.exc import IntegrityError

from bot.application.results import ServiceResult
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_accounts import (
    Account,
    AccountIdentity,
    AccountMembership,
    AccountTag,
    AccountTagAssignment,
    AccountWallet,
    MembershipPlan,
)
from bot.sql_helper.sql_application import utcnow
from bot.sql_helper.sql_emby import Emby


LOCAL_USERNAME_PATTERN = re.compile(r"^[^\s/\\<>]{3,32}$")
TAG_COLOR_PATTERN = re.compile(r"^#[0-9a-fA-F]{6}$")


def _loads(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


class PasswordHasher:
    """Dependency-free scrypt password hashes with per-password salts."""

    algorithm = "scrypt"
    n = 2**14
    r = 8
    p = 1

    @classmethod
    def hash(cls, password: str) -> str:
        cls.validate(password)
        salt = os.urandom(16)
        digest = hashlib.scrypt(password.encode("utf-8"), salt=salt, n=cls.n, r=cls.r, p=cls.p, dklen=32)
        return "$".join((cls.algorithm, str(cls.n), str(cls.r), str(cls.p), base64.urlsafe_b64encode(salt).decode(), base64.urlsafe_b64encode(digest).decode()))

    @classmethod
    def verify(cls, password: str, encoded: str) -> bool:
        try:
            algorithm, n, r, p, salt, expected = encoded.split("$", 5)
            if algorithm != cls.algorithm:
                return False
            digest = hashlib.scrypt(
                password.encode("utf-8"),
                salt=base64.urlsafe_b64decode(salt.encode()),
                n=int(n),
                r=int(r),
                p=int(p),
                dklen=32,
            )
            return hmac.compare_digest(base64.urlsafe_b64encode(digest).decode(), expected)
        except (TypeError, ValueError):
            return False

    @staticmethod
    def validate(password: str) -> None:
        if len(password or "") < 10 or len(password) > 128:
            raise ValueError("密码长度必须为 10 到 128 个字符")
        if password.strip() != password:
            raise ValueError("密码首尾不能包含空格")


def serialize_plan(row: MembershipPlan) -> dict:
    try:
        entitlements = json.loads(row.entitlements_json or "{}")
    except (TypeError, ValueError):
        entitlements = {}
    return {
        "id": row.id,
        "code": row.code,
        "name": row.name,
        "description": row.description,
        "duration_days": row.duration_days,
        "legacy_level": row.legacy_level,
        "entitlements": entitlements,
        "enabled": bool(row.enabled),
        "is_default": bool(row.is_default),
        "sort_order": row.sort_order,
        "revision": row.revision,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


def serialize_tag(row: AccountTag) -> dict:
    return {"id": row.id, "name": row.name, "color": row.color, "description": row.description, "created_at": row.created_at, "updated_at": row.updated_at}


class AccountService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    @staticmethod
    def normalize_username(value: str) -> tuple[str, str]:
        username = (value or "").strip()
        if not LOCAL_USERNAME_PATTERN.fullmatch(username):
            raise ValueError("登录名需要 3-32 个字符，不能包含空格、斜杠或尖括号")
        return username, username.casefold()

    def ensure_telegram_account(self, tg: int, display_name: Optional[str] = None) -> dict:
        now = utcnow()
        account_id = str(uuid5(NAMESPACE_URL, f"sakura-account:{int(tg)}"))
        with self._uow_factory() as uow:
            row = uow.accounts.by_legacy_tg(int(tg), for_update=True)
            if row is None:
                row = Account(id=account_id, legacy_tg=int(tg), display_name=(display_name or f"TG {tg}")[:255], status="active", created_at=now, updated_at=now)
                uow.accounts.add_account(row)
                legacy, _created = uow.users.add_if_missing(int(tg))
                if display_name and not legacy.name:
                    legacy.name = display_name[:255]
                uow.accounts.add_identity(AccountIdentity(id=str(uuid5(NAMESPACE_URL, f"sakura-identity:telegram:{tg}")), account_id=account_id, provider="telegram", subject=str(tg), verified_at=now, created_at=now, updated_at=now))
                self._ensure_wallets(uow, row.id, legacy)
            elif display_name and not row.display_name:
                row.display_name = display_name[:255]
                row.updated_at = now
            return self._serialize_account(uow, row)

    def create_local(
        self,
        *,
        username: str,
        password: str,
        display_name: Optional[str],
        actor: Actor,
        ip_address: Optional[str] = None,
        hourly_limit: int = 5,
    ) -> ServiceResult:
        username, normalized = self.normalize_username(username)
        password_hash = PasswordHasher.hash(password)
        now = utcnow()
        try:
            with self._uow_factory() as uow:
                if ip_address and uow.auth.recent_security_event_count(
                    "registration.local.created",
                    ip_address,
                    now - timedelta(hours=1),
                ) >= max(1, int(hourly_limit)):
                    return ServiceResult("rate_limited")
                if uow.accounts.local_identity(normalized):
                    return ServiceResult("username_taken")
                legacy_tg = uow.accounts.next_local_legacy_key()
                account_id = str(uuid4())
                account = Account(id=account_id, legacy_tg=legacy_tg, display_name=(display_name or username).strip()[:255], status="active", created_at=now, updated_at=now)
                uow.accounts.add_account(account)
                uow.accounts.add_identity(AccountIdentity(id=str(uuid4()), account_id=account_id, provider="local", subject=normalized, username=username, username_normalized=normalized, password_hash=password_hash, verified_at=now, created_at=now, updated_at=now))
                legacy = Emby(tg=legacy_tg, lv="d", us=0, iv=0)
                uow.users.session.add(legacy)
                self._ensure_wallets(uow, account_id, legacy)
                uow.operations.audit(actor=actor, action="account.local.create", resource_type="account", resource_id=account_id, detail={"username": username})
                uow.operations.event("account.created", "account", account_id, {"account_id": account_id, "legacy_tg": legacy_tg, "provider": "local"})
                if ip_address:
                    uow.operations.security_event(
                        event_type="registration.local.created",
                        severity="info",
                        subject_kind="account",
                        subject_id=account_id,
                        ip_address=ip_address,
                    )
                uow.flush()
                return ServiceResult("ok", self._serialize_account(uow, account))
        except IntegrityError:
            return ServiceResult("username_taken")

    def bootstrap_owner(
        self,
        *,
        owner_tg: int,
        username: str,
        password: str,
    ) -> ServiceResult:
        """Create the first strong local owner identity exactly once."""
        username, normalized = self.normalize_username(username)
        password_hash = PasswordHasher.hash(password)
        self.ensure_telegram_account(int(owner_tg), "Sakura Owner")
        now = utcnow()
        try:
            with self._uow_factory() as uow:
                account = uow.accounts.by_legacy_tg(int(owner_tg), for_update=True)
                if account is None:
                    return ServiceResult("account_not_found")
                own = next(
                    (
                        item
                        for item in uow.accounts.identities(account.id)
                        if item.provider == "local"
                    ),
                    None,
                )
                if own:
                    return ServiceResult(
                        "already_configured",
                        {"account_id": account.id, "username": own.username},
                    )
                if uow.accounts.local_identity(normalized):
                    return ServiceResult("username_taken")
                uow.accounts.add_identity(
                    AccountIdentity(
                        id=str(uuid4()),
                        account_id=account.id,
                        provider="local",
                        subject=normalized,
                        username=username,
                        username_normalized=normalized,
                        password_hash=password_hash,
                        verified_at=now,
                        created_at=now,
                        updated_at=now,
                    )
                )
                uow.operations.audit(
                    actor=Actor.system("web-owner-bootstrap"),
                    action="account.owner.bootstrap",
                    resource_type="account",
                    resource_id=account.id,
                    detail={"username": username},
                )
                return ServiceResult(
                    "ok",
                    {"account_id": account.id, "username": username},
                )
        except IntegrityError:
            return ServiceResult("username_taken")

    def add_local_identity(self, *, account_id: str, username: str, password: str, actor: Actor) -> ServiceResult:
        username, normalized = self.normalize_username(username)
        password_hash = PasswordHasher.hash(password)
        now = utcnow()
        try:
            with self._uow_factory() as uow:
                account = uow.accounts.get(account_id, for_update=True)
                if not account:
                    return ServiceResult("account_not_found")
                existing = uow.accounts.local_identity(normalized, for_update=True)
                if existing and existing.account_id != account_id:
                    return ServiceResult("username_taken")
                own = next((item for item in uow.accounts.identities(account_id) if item.provider == "local"), None)
                if own:
                    own.subject = normalized
                    own.username = username
                    own.username_normalized = normalized
                    own.password_hash = password_hash
                    own.disabled = False
                    own.updated_at = now
                else:
                    uow.accounts.add_identity(AccountIdentity(id=str(uuid4()), account_id=account_id, provider="local", subject=normalized, username=username, username_normalized=normalized, password_hash=password_hash, verified_at=now, created_at=now, updated_at=now))
                uow.operations.audit(actor=actor, action="account.local_identity.update", resource_type="account", resource_id=account_id, detail={"username": username})
                return ServiceResult("ok", {"account_id": account_id, "username": username})
        except IntegrityError:
            return ServiceResult("username_taken")

    def authenticate_local(self, username: str, password: str) -> ServiceResult:
        try:
            _username, normalized = self.normalize_username(username)
        except ValueError:
            return ServiceResult("invalid_credentials")
        with self._uow_factory() as uow:
            identity = uow.accounts.local_identity(normalized, for_update=True)
            if not identity or identity.disabled or not identity.password_hash or not PasswordHasher.verify(password, identity.password_hash):
                return ServiceResult("invalid_credentials")
            account = uow.accounts.get(identity.account_id)
            if not account or account.status not in {"active", "suspended"}:
                return ServiceResult("account_disabled")
            identity.last_used_at = utcnow()
            return ServiceResult("ok", self._serialize_account(uow, account))

    def by_legacy_tg(self, tg: int) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.accounts.by_legacy_tg(tg)
            return self._serialize_account(uow, row) if row else None

    def get(self, account_id: str) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.accounts.get(account_id)
            return self._serialize_account(uow, row) if row else None

    def list_accounts(self, **filters) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.accounts.list_accounts(**filters)
            return {"items": [self._serialize_account(uow, row) for row in rows], "total": total}

    def ledger(self, account_id: str, *, limit: int = 100, offset: int = 0) -> Optional[dict]:
        with self._uow_factory() as uow:
            if not uow.accounts.get(account_id):
                return None
            rows = uow.accounts.list_ledger(account_id, limit=limit, offset=offset)
            return {
                "items": [
                    {
                        "id": row.id,
                        "source_transaction_id": row.source_transaction_id,
                        "account_id": row.account_id,
                        "tg": row.legacy_tg,
                        "balance_type": row.balance_type,
                        "amount": row.amount,
                        "balance_after": row.balance_after,
                        "reason": row.reason,
                        "actor_kind": row.actor_kind,
                        "actor_id": row.actor_id,
                        "metadata": _loads(row.metadata_json),
                        "created_at": row.created_at,
                    }
                    for row in rows
                ],
                "limit": limit,
                "offset": offset,
            }

    def list_plans(self, *, enabled_only: bool = False) -> dict:
        with self._uow_factory() as uow:
            rows = uow.accounts.list_plans(enabled_only=enabled_only)
            return {"items": [serialize_plan(row) for row in rows], "total": len(rows)}

    def create_plan(self, data: dict, actor: Actor) -> dict:
        now = utcnow()
        with self._uow_factory() as uow:
            row = MembershipPlan(
                code=str(data["code"]).strip().lower(), name=str(data["name"]).strip(), description=(data.get("description") or "").strip() or None,
                duration_days=int(data.get("duration_days", 30)), legacy_level=str(data.get("legacy_level", "b")), entitlements_json=json.dumps(data.get("entitlements") or {}, ensure_ascii=False),
                enabled=bool(data.get("enabled", True)), is_default=bool(data.get("is_default", False)), sort_order=int(data.get("sort_order", 0)), created_at=now, updated_at=now,
            )
            if row.is_default:
                row.enabled = True
            if row.is_default:
                for current in uow.accounts.list_plans():
                    current.is_default = False
            uow.accounts.add_plan(row)
            uow.flush()
            uow.operations.audit(actor=actor, action="membership.plan.create", resource_type="membership_plan", resource_id=str(row.id), detail={"code": row.code})
            return serialize_plan(row)

    def update_plan(self, plan_id: int, data: dict, actor: Actor) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.accounts.get_plan(plan_id)
            if not row:
                return None
            expected = data.pop("revision", None)
            if expected is not None and int(expected) != int(row.revision):
                raise RuntimeError("会员方案已被其他管理员修改，请刷新后重试")
            if row.is_default and data.get("enabled") is False:
                raise RuntimeError("默认会员方案不能停用，请先设置新的默认方案")
            for key in ("name", "description", "duration_days", "legacy_level", "enabled", "is_default", "sort_order"):
                if key in data:
                    setattr(row, key, data[key])
            if "entitlements" in data:
                row.entitlements_json = json.dumps(data["entitlements"] or {}, ensure_ascii=False)
            if row.is_default:
                row.enabled = True
                for current in uow.accounts.list_plans():
                    if current.id != row.id:
                        current.is_default = False
            row.revision += 1
            row.updated_at = utcnow()
            uow.operations.audit(actor=actor, action="membership.plan.update", resource_type="membership_plan", resource_id=str(row.id), detail=data)
            return serialize_plan(row)

    def assign_plan(self, *, account_id: str, plan_id: int, duration_days: Optional[int], actor: Actor) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            account = uow.accounts.get(account_id, for_update=True)
            plan = uow.accounts.get_plan(plan_id)
            if not account or not plan or not plan.enabled:
                return ServiceResult("not_found")
            current = uow.accounts.active_membership(account_id, for_update=True)
            if current:
                current.status = "replaced"
                current.updated_at = now
            days = int(duration_days or plan.duration_days)
            membership = AccountMembership(id=str(uuid4()), account_id=account_id, plan_id=plan.id, status="active", starts_at=now, expires_at=now + timedelta(days=days) if days > 0 else None, source="admin", created_by_kind=actor.kind, created_by_id=actor.identifier, created_at=now, updated_at=now)
            uow.accounts.add_membership(membership)
            legacy = uow.users.get_for_update(account.legacy_tg)
            if legacy:
                legacy.lv = plan.legacy_level
                legacy.ex = membership.expires_at
            uow.operations.audit(actor=actor, action="membership.assign", resource_type="account", resource_id=account_id, detail={"plan_id": plan.id, "days": days})
            uow.operations.event("membership.updated", "account", account_id, {"account_id": account_id, "plan_id": plan.id, "expires_at": membership.expires_at})
            return ServiceResult("ok", {"account_id": account_id, "membership_id": membership.id, "plan": serialize_plan(plan), "expires_at": membership.expires_at})

    def assign_default_plan(self, *, account_id: str, duration_days: Optional[int], actor: Actor) -> ServiceResult:
        with self._uow_factory() as uow:
            plan = uow.accounts.default_plan()
            plan_id = plan.id if plan else None
        if plan_id is None:
            return ServiceResult("not_found")
        return self.assign_plan(account_id=account_id, plan_id=plan_id, duration_days=duration_days, actor=actor)

    def list_tags(self) -> dict:
        with self._uow_factory() as uow:
            rows = uow.accounts.list_tags()
            return {"items": [serialize_tag(row) for row in rows], "total": len(rows)}

    def create_tag(self, *, name: str, color: str, description: Optional[str], actor: Actor) -> ServiceResult:
        name = (name or "").strip()
        if not name or len(name) > 64 or not TAG_COLOR_PATTERN.fullmatch(color or ""):
            return ServiceResult("invalid")
        with self._uow_factory() as uow:
            if uow.accounts.get_tag_by_name(name):
                return ServiceResult("exists")
            row = AccountTag(name=name, color=color, description=(description or "").strip() or None, created_at=utcnow(), updated_at=utcnow())
            uow.accounts.add_tag(row)
            uow.flush()
            uow.operations.audit(actor=actor, action="account.tag.create", resource_type="account_tag", resource_id=str(row.id), detail={"name": name})
            return ServiceResult("ok", serialize_tag(row))

    def delete_tag(self, tag_id: int, actor: Actor) -> bool:
        with self._uow_factory() as uow:
            row = uow.accounts.get_tag(tag_id)
            if not row:
                return False
            uow.accounts.delete_tag(row)
            uow.operations.audit(actor=actor, action="account.tag.delete", resource_type="account_tag", resource_id=str(tag_id), detail={"name": row.name})
            return True

    def assign_tags(self, *, account_ids: list[str], tag_ids: list[int], mode: str, actor: Actor) -> dict:
        if mode not in {"add", "remove", "replace"}:
            raise ValueError("Unsupported tag assignment mode")
        changed = 0
        with self._uow_factory() as uow:
            valid_tags = {row.id for row in uow.accounts.list_tags() if row.id in set(tag_ids)}
            for account_id in dict.fromkeys(account_ids):
                if not uow.accounts.get(account_id):
                    continue
                current = {row.id for row in uow.accounts.tags_for_account(account_id)}
                remove = current if mode == "replace" else (valid_tags if mode == "remove" else set())
                add = valid_tags if mode in {"add", "replace"} else set()
                for tag_id in remove:
                    changed += uow.accounts.remove_assignment(account_id, tag_id)
                for tag_id in add:
                    if not uow.accounts.assignment(account_id, tag_id):
                        uow.accounts.add_assignment(AccountTagAssignment(account_id=account_id, tag_id=tag_id, assigned_by_kind=actor.kind, assigned_by_id=actor.identifier, created_at=utcnow()))
                        changed += 1
            uow.operations.audit(actor=actor, action="account.tags.batch", resource_type="account", resource_id=None, detail={"account_count": len(account_ids), "tag_ids": sorted(valid_tags), "mode": mode, "changed": changed})
            uow.operations.event("account.tags.updated", "account", None, {"account_ids": account_ids, "tag_ids": sorted(valid_tags), "mode": mode})
        return {"changed": changed}

    @staticmethod
    def _ensure_wallets(uow, account_id: str, legacy: Emby) -> None:
        for balance_type, value in (("coins", legacy.iv), ("registration_days", legacy.us)):
            if not uow.accounts.wallet(account_id, balance_type):
                uow.accounts.add_wallet(AccountWallet(account_id=account_id, balance_type=balance_type, balance=int(value or 0), revision=1, updated_at=utcnow()))

    @staticmethod
    def _serialize_account(uow, row: Account) -> dict:
        identities = uow.accounts.identities(row.id)
        membership = uow.accounts.active_membership(row.id)
        plan = uow.accounts.get_plan(membership.plan_id) if membership else None
        wallets = {}
        for balance_type in ("coins", "registration_days"):
            wallet = uow.accounts.wallet(row.id, balance_type)
            wallets[balance_type] = int(wallet.balance) if wallet else 0
        legacy = uow.users.get(row.legacy_tg)
        return {
            "id": row.id,
            "account_id": row.id,
            "legacy_tg": row.legacy_tg,
            "tg": next((int(item.subject) for item in identities if item.provider == "telegram"), None),
            "display_name": row.display_name,
            "status": row.status,
            "identities": [{"provider": item.provider, "subject": item.subject if item.provider != "local" else None, "username": item.username, "verified_at": item.verified_at, "last_used_at": item.last_used_at} for item in identities],
            "membership": ({"id": membership.id, "status": membership.status, "starts_at": membership.starts_at, "expires_at": membership.expires_at, "plan": serialize_plan(plan)} if membership and plan else None),
            "tags": [serialize_tag(tag) for tag in uow.accounts.tags_for_account(row.id)],
            "wallets": wallets,
            "emby": ({"id": legacy.embyid, "username": legacy.name, "level": legacy.lv, "expires_at": legacy.ex} if legacy else None),
            "created_at": row.created_at,
            "updated_at": row.updated_at,
        }
