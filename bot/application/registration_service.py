from __future__ import annotations

import json
import re
from datetime import datetime
from typing import Optional
from uuid import uuid4

from sqlalchemy import func
from sqlalchemy.exc import IntegrityError

from bot.application.results import ServiceResult
from bot.application.task_service import serialize_task
from bot.application.user_service import UserService
from bot.domain import Actor, secret_fingerprint
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import OperationTask, RegistrationState, utcnow
from bot.sql_helper.sql_emby import Emby


REGISTRATION_TASK_TYPE = "registration.account"
ACTIVE_TASK_STATUSES = ("pending", "retrying", "running")
USERNAME_PATTERN = re.compile(r"^[^\s/\\<>]{2,32}$")
SAFETY_CODE_PATTERN = re.compile(r"^\d{4,6}$")


def _runtime_config():
    import bot

    return bot._open, bot.ranks


def _loads(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


class RegistrationService:
    """Canonical registration workflow shared by Telegram and Web."""

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork, emby_client=None):
        self._uow_factory = uow_factory
        self._users = UserService(uow_factory)
        self._emby_client = emby_client

    @staticmethod
    def validate_username(value: str) -> str:
        username = (value or "").strip()
        if not USERNAME_PATTERN.fullmatch(username):
            raise ValueError("用户名需为 2-32 个字符，不能包含空格、斜杠或尖括号")
        return username

    @staticmethod
    def validate_safety_code(value: str) -> str:
        safety_code = (value or "").strip()
        if not SAFETY_CODE_PATTERN.fullmatch(safety_code):
            raise ValueError("安全码必须是 4-6 位数字")
        return safety_code

    def status(self, tg: Optional[int] = None) -> dict:
        open_config, _ranks = _runtime_config()
        with self._uow_factory() as uow:
            session = uow.operations.session
            registered = int(
                session.query(func.count(Emby.tg))
                .filter(Emby.embyid.isnot(None))
                .scalar()
                or 0
            )
            active = int(
                session.query(func.count(OperationTask.id))
                .filter(
                    OperationTask.task_type == REGISTRATION_TASK_TYPE,
                    OperationTask.status.in_(ACTIVE_TASK_STATUSES),
                )
                .scalar()
                or 0
            )
            waiting = int(
                session.query(func.count(OperationTask.id))
                .filter(
                    OperationTask.task_type == REGISTRATION_TASK_TYPE,
                    OperationTask.status.in_(("pending", "retrying")),
                )
                .scalar()
                or 0
            )
            user = uow.users.get(tg) if tg else None
            active_task = self._active_task(uow, tg) if tg else None
            capacity = max(0, int(open_config.all_user) - registered - active)
            return {
                "enabled": bool(open_config.stat),
                "open_registration_days": int(open_config.open_us),
                "user_limit": int(open_config.all_user),
                "registered": registered,
                "reserved": active,
                "remaining": capacity,
                "queue_waiting": waiting,
                "queue_limit": int(
                    getattr(open_config, "register_queue_limit", 100) or 100
                ),
                "has_account": bool(user and user.embyid),
                "qualification_days": int(user.us or 0) if user else 0,
                "can_register": bool(
                    user
                    and not user.embyid
                    and capacity > 0
                    and (bool(open_config.stat) or int(user.us or 0) > 0)
                ),
                "active_task": (
                    self._public_task(active_task) if active_task else None
                ),
                "checked_at": utcnow(),
            }

    def submit(
        self,
        *,
        tg: int,
        username: str,
        safety_code: str,
        registration_code: Optional[str],
        actor: Actor,
        idempotency_key: str,
        channel: str = "web",
        notification_chat_id: Optional[int] = None,
        notification_message_id: Optional[int] = None,
    ) -> ServiceResult:
        username = self.validate_username(username)
        safety_code = self.validate_safety_code(safety_code)
        registration_code = (registration_code or "").strip() or None
        open_config, ranks = _runtime_config()
        now = utcnow()
        task_id = str(uuid4())
        try:
            with self._uow_factory() as uow:
                replay = uow.operations.get_task_by_idempotency_key(idempotency_key)
                if replay:
                    return ServiceResult("ok", self._public_task(replay))

                user = uow.users.get_for_update(tg)
                if not user:
                    return ServiceResult("user_not_found")
                if user.embyid:
                    return ServiceResult("account_already_bound")

                active_task = self._active_task(uow, tg)
                if active_task:
                    return ServiceResult(
                        "duplicate",
                        self._public_task(active_task),
                    )

                session = uow.operations.session
                self._lock_registration_state(session, now)
                if (
                    session.query(Emby)
                    .filter(func.lower(Emby.name) == username.lower())
                    .first()
                ):
                    return ServiceResult("username_taken")

                registered = int(
                    session.query(func.count(Emby.tg))
                    .filter(Emby.embyid.isnot(None))
                    .scalar()
                    or 0
                )
                active_count = int(
                    session.query(func.count(OperationTask.id))
                    .filter(
                        OperationTask.task_type == REGISTRATION_TASK_TYPE,
                        OperationTask.status.in_(ACTIVE_TASK_STATUSES),
                    )
                    .scalar()
                    or 0
                )
                if registered + active_count >= int(open_config.all_user):
                    return ServiceResult("slot_full")

                waiting = int(
                    session.query(func.count(OperationTask.id))
                    .filter(
                        OperationTask.task_type == REGISTRATION_TASK_TYPE,
                        OperationTask.status.in_(("pending", "retrying")),
                    )
                    .scalar()
                    or 0
                )
                queue_limit = max(
                    1,
                    int(getattr(open_config, "register_queue_limit", 100) or 100),
                )
                if waiting >= queue_limit:
                    return ServiceResult("queue_full")

                if not bool(open_config.stat) and int(user.us or 0) <= 0:
                    redeemed = self._redeem_code(
                        uow,
                        user=user,
                        code_value=registration_code,
                        logo=str(ranks.logo),
                        actor=actor,
                        now=now,
                    )
                    if redeemed != "ok":
                        return ServiceResult(redeemed)

                is_open = bool(open_config.stat)
                days = int(open_config.open_us if is_open else user.us or 0)
                if days <= 0:
                    return ServiceResult("no_qualification")

                payload = {
                    "tg": int(tg),
                    "username": username,
                    "safety_code": safety_code,
                    "days": days,
                    "consume_qualification": not is_open,
                    "channel": channel,
                    "notification_chat_id": notification_chat_id,
                    "notification_message_id": notification_message_id,
                }
                task = OperationTask(
                    id=task_id,
                    task_type=REGISTRATION_TASK_TYPE,
                    status="pending",
                    progress=0,
                    owner_kind=actor.kind,
                    owner_id=str(tg),
                    idempotency_key=idempotency_key,
                    input_json=json.dumps(payload, ensure_ascii=False),
                    retry_count=0,
                    max_retries=0,
                    next_run_at=now,
                    cancel_requested=False,
                    created_at=now,
                    updated_at=now,
                )
                uow.operations.add_task(task)
                uow.operations.audit(
                    actor=actor,
                    action="registration.submit",
                    resource_type="operation_task",
                    resource_id=task_id,
                    detail={
                        "username": username,
                        "days": days,
                        "channel": channel,
                    },
                )
                uow.operations.event(
                    "registration.submitted",
                    "user",
                    str(tg),
                    {
                        "task_id": task_id,
                        "username": username,
                        "status": "pending",
                    },
                )
                uow.operations.event(
                    "task.created",
                    "operation_task",
                    task_id,
                    {
                        "task_id": task_id,
                        "task_type": REGISTRATION_TASK_TYPE,
                        "status": "pending",
                    },
                )
                uow.flush()
                result = self._public_task(task)
                result["position"] = active_count + 1
                return ServiceResult("ok", result)
        except IntegrityError:
            with self._uow_factory() as uow:
                replay = uow.operations.get_task_by_idempotency_key(idempotency_key)
                if replay:
                    return ServiceResult("ok", self._public_task(replay))
                active_task = self._active_task(uow, tg)
                if active_task:
                    return ServiceResult("duplicate", self._public_task(active_task))
            raise

    def task(self, task_id: str, tg: int) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.operations.get_task(task_id)
            if (
                not row
                or row.task_type != REGISTRATION_TASK_TYPE
                or row.owner_id != str(tg)
            ):
                return None
            return self._public_task(row, include_credentials=True)

    def cancel(self, task_id: str, tg: int, actor: Actor) -> ServiceResult:
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.operations.get_task(task_id, for_update=True)
            if (
                not row
                or row.task_type != REGISTRATION_TASK_TYPE
                or row.owner_id != str(tg)
            ):
                return ServiceResult("not_found")
            if row.status not in {"pending", "retrying"}:
                return ServiceResult("not_cancelable", self._public_task(row))
            row.status = "canceled"
            row.cancel_requested = True
            row.progress = 100
            row.finished_at = now
            row.updated_at = now
            uow.operations.audit(
                actor=actor,
                action="registration.cancel",
                resource_type="operation_task",
                resource_id=task_id,
            )
            uow.operations.event(
                "registration.updated",
                "user",
                str(tg),
                {"task_id": task_id, "status": "canceled"},
            )
            uow.operations.event(
                "task.updated",
                "operation_task",
                task_id,
                {
                    "task_id": task_id,
                    "task_type": REGISTRATION_TASK_TYPE,
                    "status": "canceled",
                },
            )
            return ServiceResult("ok", self._public_task(row))

    async def execute(self, payload: dict) -> dict:
        tg = int(payload["tg"])
        username = self.validate_username(payload["username"])
        safety_code = self.validate_safety_code(payload["safety_code"])
        days = int(payload["days"])
        consume_qualification = bool(payload.get("consume_qualification"))

        with self._uow_factory() as uow:
            user = uow.users.get(tg)
            if not user:
                return await self._failure(tg, "user_not_found", "用户记录不存在")
            if user.embyid:
                return await self._failure(
                    tg,
                    "account_already_bound",
                    "当前 Telegram 已经绑定 Emby 账号",
                )

        if self._emby_client is None:
            from bot.func_helper.emby import emby

            emby_client = emby
        else:
            emby_client = self._emby_client

        created = await emby_client.emby_create(name=username, days=days)
        if not created:
            return await self._failure(
                tg,
                "emby_create_failed",
                "用户名已存在或 Emby 服务暂时不可用",
            )

        emby_id, emby_password, expires_at = created
        try:
            completed = self._users.complete_registration(
                tg=tg,
                embyid=str(emby_id),
                name=username,
                pwd=str(emby_password),
                pwd2=safety_code,
                expires_at=expires_at,
                consume_qualification=consume_qualification,
                actor=Actor.system("registration-worker"),
            )
        except Exception:
            try:
                await emby_client.emby_del(emby_id=str(emby_id))
            except Exception:
                pass
            raise
        if not completed.ok:
            try:
                await emby_client.emby_del(emby_id=str(emby_id))
            except Exception:
                pass
            return await self._failure(
                tg,
                completed.status,
                "账号状态已变化，本次创建已回滚",
            )

        try:
            from bot.func_helper.utils import tem_adduser

            tem_adduser()
        except Exception:
            pass

        result = {
            "ok": True,
            "code": "registered",
            "tg": tg,
            "username": username,
            "emby_id": str(emby_id),
            "emby_password": str(emby_password),
            "expires_at": expires_at,
        }
        with self._uow_factory() as uow:
            uow.operations.event(
                "registration.completed",
                "user",
                str(tg),
                {
                    "username": username,
                    "emby_id": str(emby_id),
                    "status": "succeeded",
                },
            )
        return result

    async def _failure(self, tg: int, code: str, message: str) -> dict:
        with self._uow_factory() as uow:
            uow.operations.event(
                "registration.failed",
                "user",
                str(tg),
                {"code": code, "message": message, "status": "failed"},
            )
        return {"ok": False, "code": code, "message": message, "tg": tg}

    @staticmethod
    def _active_task(uow, tg: int):
        return (
            uow.operations.session.query(OperationTask)
            .filter(
                OperationTask.task_type == REGISTRATION_TASK_TYPE,
                OperationTask.owner_id == str(tg),
                OperationTask.status.in_(ACTIVE_TASK_STATUSES),
            )
            .order_by(OperationTask.created_at.desc())
            .first()
        )

    @staticmethod
    def _lock_registration_state(session, now: datetime) -> None:
        state = (
            session.query(RegistrationState)
            .filter(RegistrationState.id == 1)
            .with_for_update()
            .first()
        )
        if state is None:
            state = RegistrationState(id=1, updated_at=now)
            session.add(state)
            session.flush()
        else:
            state.updated_at = now

    @staticmethod
    def _public_task(row: OperationTask, include_credentials: bool = False) -> dict:
        task = serialize_task(row, include_sensitive=include_credentials)
        if isinstance(task.get("input"), dict):
            task["username"] = task["input"].get("username")
            task["input"].pop("safety_code", None)
        return task

    @staticmethod
    def _redeem_code(
        uow,
        *,
        user,
        code_value: Optional[str],
        logo: str,
        actor: Actor,
        now: datetime,
    ) -> str:
        if not code_value:
            return "registration_code_required"
        code = uow.codes.get_for_update(code_value)
        if not code:
            return "invalid_code"
        if code.used is not None:
            return "used_code"
        prefix = code_value.split("-")[0]
        if prefix not in {logo, str(user.tg)}:
            return "forbidden_code"
        user.us = int(user.us or 0) + int(code.us or 0)
        code.used = user.tg
        code.usedtime = now
        fingerprint = secret_fingerprint(code_value)
        uow.operations.audit(
            actor=actor,
            action="code.redeem.registration",
            resource_type="registration_code",
            resource_id=fingerprint,
            detail={"tg": user.tg, "issuer_tg": code.tg},
        )
        uow.operations.event(
            "code.redeemed",
            "registration_code",
            fingerprint,
            {"tg": user.tg, "kind": "registration"},
        )
        return "ok"
