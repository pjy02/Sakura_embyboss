from __future__ import annotations

import asyncio
import json
import os
import re
from datetime import datetime, timedelta
from time import perf_counter
from typing import Optional
from uuid import uuid4

import aiohttp
from sqlalchemy import func, text

from bot.application.community_service import NotificationService
from bot.application.point_service import PointService
from bot.application.results import ServiceResult
from bot.application.task_service import TaskService
from bot.application.user_service import UserService
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import (
    AccountLifecycleEvent,
    AlertDelivery,
    OperationTask,
    RiskRule,
    SecurityEvent,
    ServiceProbe,
    utcnow,
)
from bot.sql_helper.sql_emby import Emby


SEVERITY_ORDER = {"info": 1, "warning": 2, "danger": 3}
LIFECYCLE_ACTIONS = {
    "suspend",
    "restore",
    "extend",
    "grant_coins",
    "grant_registration_days",
    "notify",
    "clear_account",
}


def _json(value) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str)


def _loads(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


def serialize_rule(row: RiskRule) -> dict:
    return {
        "id": row.id,
        "name": row.name,
        "event_pattern": row.event_pattern,
        "severity": row.severity,
        "threshold_count": row.threshold_count,
        "window_minutes": row.window_minutes,
        "cooldown_minutes": row.cooldown_minutes,
        "enabled": bool(row.enabled),
        "telegram_alert": bool(row.telegram_alert),
        "revision": row.revision,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


def serialize_probe(row: ServiceProbe) -> dict:
    return {
        "id": row.id,
        "service_name": row.service_name,
        "service_kind": row.service_kind,
        "status": row.status,
        "latency_ms": row.latency_ms,
        "status_code": row.status_code,
        "message": row.message,
        "detail": _loads(row.detail_json),
        "checked_at": row.checked_at,
    }


class RiskRuleService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(self) -> dict:
        with self._uow_factory() as uow:
            rows = uow.operations.session.query(RiskRule).order_by(RiskRule.id).all()
            return {"items": [serialize_rule(row) for row in rows]}

    def create(self, data: dict, actor: Actor) -> dict:
        self._validate(data)
        now = utcnow()
        with self._uow_factory() as uow:
            row = RiskRule(**data, revision=1, created_at=now, updated_at=now)
            uow.operations.session.add(row)
            uow.flush()
            uow.operations.audit(actor=actor, action="security.rule.create", resource_type="risk_rule", resource_id=str(row.id), detail={"name": row.name})
            uow.operations.event("security.rule.updated", "security", str(row.id), {"rule_id": row.id, "action": "created"})
            return serialize_rule(row)

    def update(self, rule_id: int, data: dict, expected_revision: int, actor: Actor) -> Optional[dict]:
        self._validate(data)
        with self._uow_factory() as uow:
            row = uow.operations.session.query(RiskRule).filter(RiskRule.id == rule_id).with_for_update().first()
            if row is None:
                return None
            if int(row.revision) != expected_revision:
                raise RuntimeError("规则已被其他管理员修改，请刷新后重试")
            before = serialize_rule(row)
            for key, value in data.items():
                setattr(row, key, value)
            row.revision += 1
            row.updated_at = utcnow()
            uow.operations.audit(actor=actor, action="security.rule.update", resource_type="risk_rule", resource_id=str(rule_id), detail={"before": before, "after": data})
            uow.operations.event("security.rule.updated", "security", str(rule_id), {"rule_id": rule_id, "action": "updated"})
            uow.flush()
            return serialize_rule(row)

    @staticmethod
    def _validate(data: dict) -> None:
        data["name"] = str(data.get("name") or "").strip()
        data["event_pattern"] = str(data.get("event_pattern") or "").strip()
        if len(data["name"]) < 2:
            raise ValueError("规则名称至少需要 2 个字符")
        if data.get("severity") not in SEVERITY_ORDER:
            raise ValueError("未知风险级别")
        if not data["event_pattern"]:
            raise ValueError("事件匹配规则不能为空")
        if not re.fullmatch(r"[A-Za-z0-9_.*-]+", data["event_pattern"]):
            raise ValueError("事件匹配只能包含字母、数字、点、横线、下划线和星号")
        for key, minimum, maximum in (
            ("threshold_count", 1, 100000),
            ("window_minutes", 1, 10080),
            ("cooldown_minutes", 1, 43200),
        ):
            value = int(data.get(key, 0))
            if value < minimum or value > maximum:
                raise ValueError(f"{key} 超出允许范围")


class AlertService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    @staticmethod
    def recipients() -> list[int]:
        configured = os.getenv("SAKURA_ALERT_TELEGRAM_IDS", "").strip()
        if configured:
            recipients = sorted({int(item.strip()) for item in configured.split(",") if item.strip().isdigit() and int(item.strip()) > 0})
            if recipients:
                return recipients
        import bot

        candidates = [getattr(bot, "owner", None), *(getattr(bot, "admins", None) or [])]
        return sorted({int(item) for item in candidates if item is not None and int(item) > 0})

    def queue(self, uow, event: SecurityEvent, rule_name: str) -> int:
        now = utcnow()
        created = 0
        for recipient in self.recipients():
            exists = uow.operations.session.query(AlertDelivery.id).filter(AlertDelivery.security_event_id == event.id, AlertDelivery.recipient_tg == recipient).first()
            if exists:
                continue
            alert_id = str(uuid4())
            uow.operations.session.add(AlertDelivery(id=alert_id, security_event_id=event.id, recipient_tg=recipient, status="pending", attempt_count=0, created_at=now, updated_at=now))
            uow.operations.add_task(OperationTask(
                id=str(uuid4()), task_type="alert.telegram", status="pending", progress=0,
                owner_kind="system", owner_id="risk-automation", idempotency_key=f"alert:{event.id}:{recipient}",
                input_json=_json({"alert_id": alert_id, "rule_name": rule_name}), retry_count=0, max_retries=3,
                next_run_at=now, cancel_requested=False, created_at=now, updated_at=now,
            ))
            created += 1
        return created

    def list(self, *, status: Optional[str] = None, limit: int = 100) -> dict:
        with self._uow_factory() as uow:
            query = uow.operations.session.query(AlertDelivery, SecurityEvent).outerjoin(
                SecurityEvent,
                SecurityEvent.id == AlertDelivery.security_event_id,
            )
            if status:
                query = query.filter(AlertDelivery.status == status)
            rows = query.order_by(AlertDelivery.created_at.desc()).limit(limit).all()
            return {
                "items": [
                    {
                        "id": delivery.id,
                        "security_event_id": delivery.security_event_id,
                        "recipient_tg": delivery.recipient_tg,
                        "status": delivery.status,
                        "attempt_count": delivery.attempt_count,
                        "error_message": delivery.error_message,
                        "event_type": event.event_type if event else None,
                        "severity": event.severity if event else None,
                        "created_at": delivery.created_at,
                        "sent_at": delivery.sent_at,
                        "updated_at": delivery.updated_at,
                    }
                    for delivery, event in rows
                ]
            }

    async def deliver(self, alert_id: str, sender=None) -> dict:
        with self._uow_factory() as uow:
            row = uow.operations.session.query(AlertDelivery).filter(AlertDelivery.id == alert_id).with_for_update().first()
            if row is None:
                return {"ok": False, "code": "not_found"}
            if row.status == "sent":
                return {"ok": True, "already_sent": True}
            event = uow.operations.session.get(SecurityEvent, row.security_event_id)
            if event is None:
                row.status = "failed"
                row.error_message = "风险事件不存在"
                return {"ok": False, "code": "event_not_found"}
            recipient = int(row.recipient_tg)
            detail = _loads(event.detail_json) or {}
            message = (
                "🚨 Sakura 系统告警\n\n"
                f"级别：{event.severity}\n"
                f"事件：{event.event_type}\n"
                f"对象：{event.subject_kind or 'system'} / {event.subject_id or '-'}\n"
                f"时间：{event.created_at}\n"
                f"详情：{json.dumps(detail, ensure_ascii=False, default=str)[:1200]}"
            )
            row.attempt_count = int(row.attempt_count or 0) + 1
            row.updated_at = utcnow()
        try:
            if sender is None:
                from bot.integrations.telegram_gateway import TelegramGateway

                sender = TelegramGateway()
            await sender.send_message(recipient, message, parse_mode=None)
        except Exception as error:
            with self._uow_factory() as uow:
                row = uow.operations.session.query(AlertDelivery).filter(AlertDelivery.id == alert_id).with_for_update().first()
                if row:
                    row.status = "failed"
                    row.error_message = f"{type(error).__name__}: {error}"[:1000]
                    row.updated_at = utcnow()
            raise
        with self._uow_factory() as uow:
            row = uow.operations.session.query(AlertDelivery).filter(AlertDelivery.id == alert_id).with_for_update().first()
            if row:
                row.status = "sent"
                row.sent_at = utcnow()
                row.error_message = None
                row.updated_at = row.sent_at
        return {"ok": True, "recipient_tg": recipient}


class RiskAutomationService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory
        self._alerts = AlertService(uow_factory)

    def evaluate(self) -> dict:
        now = utcnow()
        triggered = []
        alerts = 0
        with self._uow_factory() as uow:
            session = uow.operations.session
            rules = session.query(RiskRule).filter(RiskRule.enabled.is_(True)).all()
            for rule in rules:
                pattern = rule.event_pattern.replace("%", r"\%").replace("_", r"\_").replace("*", "%")
                count = int(session.query(func.count(SecurityEvent.id)).filter(
                    SecurityEvent.event_type.like(pattern, escape="\\"),
                    SecurityEvent.event_type != "risk.rule.triggered",
                    SecurityEvent.created_at >= now - timedelta(minutes=int(rule.window_minutes)),
                ).scalar() or 0)
                if count < int(rule.threshold_count):
                    continue
                recent = session.query(SecurityEvent).filter(
                    SecurityEvent.event_type == "risk.rule.triggered",
                    SecurityEvent.subject_kind == "risk_rule",
                    SecurityEvent.subject_id == str(rule.id),
                    SecurityEvent.created_at >= now - timedelta(minutes=int(rule.cooldown_minutes)),
                ).first()
                if recent:
                    continue
                event = uow.operations.security_event(
                    event_type="risk.rule.triggered", severity=rule.severity,
                    subject_kind="risk_rule", subject_id=str(rule.id),
                    detail={"rule_name": rule.name, "event_pattern": rule.event_pattern, "matched_count": count, "window_minutes": rule.window_minutes},
                )
                uow.flush()
                if rule.telegram_alert:
                    alerts += self._alerts.queue(uow, event, rule.name)
                triggered.append({"rule_id": rule.id, "event_id": event.id, "matched_count": count})
        return {"triggered": triggered, "alerts_queued": alerts, "checked_at": now}


class DiagnosticService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory
        self._automation = RiskAutomationService(uow_factory)

    async def run(self) -> dict:
        results = [await asyncio.to_thread(self._probe_database)]
        targets = self._targets()
        if targets:
            results.extend(await asyncio.gather(*(self._probe_http(**target) for target in targets)))
        try:
            self._persist(results)
        except Exception as error:
            await self._fallback_alert(results, error)
            raise
        automation = await asyncio.to_thread(self._automation.evaluate)
        return {"probes": results, "risk_automation": automation, "checked_at": utcnow()}

    @staticmethod
    async def _fallback_alert(results: list[dict], error: Exception) -> None:
        """Best-effort alert path used when the database cannot persist a probe."""
        try:
            from bot.integrations.telegram_gateway import TelegramGateway

            sender = TelegramGateway()

            failed = [item["service_name"] for item in results if item["status"] == "unhealthy"]
            message = (
                "🚨 Sakura 诊断无法写入数据库\n\n"
                f"异常探测：{', '.join(failed) if failed else '持久化层'}\n"
                f"错误：{type(error).__name__}: {str(error)[:500]}"
            )
            await asyncio.gather(
                *(sender.send_message(recipient, message, parse_mode=None) for recipient in AlertService.recipients()),
                return_exceptions=True,
            )
        except Exception:
            pass

    def summary(self, history_limit: int = 40) -> dict:
        with self._uow_factory() as uow:
            rows = uow.operations.session.query(ServiceProbe).order_by(ServiceProbe.checked_at.desc()).limit(max(history_limit, 20) * 4).all()
            latest = {}
            for row in rows:
                latest.setdefault(row.service_name, serialize_probe(row))
            history = [serialize_probe(row) for row in rows[:history_limit]]
            statuses = [item["status"] for item in latest.values()]
            return {
                "status": "healthy" if statuses and all(item == "healthy" for item in statuses) else "degraded",
                "services": list(latest.values()),
                "history": history,
                "checked_at": utcnow(),
            }

    def _probe_database(self) -> dict:
        start = perf_counter()
        try:
            with self._uow_factory() as uow:
                uow.operations.session.execute(text("SELECT 1"))
            return self._result("database", "database", "healthy", start, 200, "数据库连接正常")
        except Exception as error:
            return self._result("database", "database", "unhealthy", start, None, f"{type(error).__name__}: {error}")

    @staticmethod
    async def _probe_http(name: str, kind: str, url: str, headers: Optional[dict] = None, require_success: bool = True) -> dict:
        start = perf_counter()
        try:
            timeout = aiohttp.ClientTimeout(total=8)
            async with aiohttp.ClientSession(timeout=timeout) as session:
                async with session.get(url, headers=headers or {}, allow_redirects=True) as response:
                    await response.content.read(1024)
                    status = "healthy" if (response.status < 400 if require_success else response.status < 500) else "unhealthy"
                    return DiagnosticService._result(name, kind, status, start, response.status, f"HTTP {response.status}")
        except Exception as error:
            message = (
                f"{type(error).__name__}: Telegram API request failed"
                if name == "telegram"
                else f"{type(error).__name__}: {error}"
            )
            return DiagnosticService._result(name, kind, "unhealthy", start, None, message)

    @staticmethod
    def _result(name, kind, status, start, status_code, message):
        return {"service_name": name, "service_kind": kind, "status": status, "latency_ms": int((perf_counter() - start) * 1000), "status_code": status_code, "message": str(message)[:1000], "checked_at": utcnow()}

    @staticmethod
    def _targets() -> list[dict]:
        import bot

        targets = []
        emby_url = str(getattr(bot, "emby_url", "") or "").rstrip("/")
        if emby_url:
            targets.append({"name": "emby", "kind": "media", "url": f"{emby_url}/System/Info/Public"})
        bot_token = str(getattr(bot, "bot_token", "") or "")
        if bot_token:
            targets.append({"name": "telegram", "kind": "messaging", "url": f"https://api.telegram.org/bot{bot_token}/getMe", "require_success": True})
        moviepilot = getattr(bot, "moviepilot", None)
        if moviepilot and getattr(moviepilot, "status", False) and getattr(moviepilot, "url", None):
            headers = {"Authorization": str(moviepilot.access_token)} if getattr(moviepilot, "access_token", None) else None
            targets.append({"name": "moviepilot", "kind": "automation", "url": str(moviepilot.url), "headers": headers})
        return targets

    def _persist(self, results: list[dict]) -> None:
        with self._uow_factory() as uow:
            session = uow.operations.session
            for result in results:
                previous = session.query(ServiceProbe).filter(ServiceProbe.service_name == result["service_name"]).order_by(ServiceProbe.checked_at.desc()).first()
                session.add(ServiceProbe(**result))
                if result["status"] == "unhealthy" and (previous is None or previous.status != "unhealthy"):
                    uow.operations.security_event(event_type="service.probe.failed", severity="danger", subject_kind="service", subject_id=result["service_name"], detail={"message": result["message"], "latency_ms": result["latency_ms"], "status_code": result["status_code"]})
                elif result["status"] == "healthy" and previous is not None and previous.status == "unhealthy":
                    uow.operations.event("service.probe.recovered", "service", result["service_name"], {"service_name": result["service_name"], "status": "healthy"})
            try:
                retention_days = max(1, min(3650, int(os.getenv("SAKURA_PROBE_RETENTION_DAYS", "30"))))
            except ValueError:
                retention_days = 30
            session.query(ServiceProbe).filter(
                ServiceProbe.checked_at < utcnow() - timedelta(days=retention_days)
            ).delete(synchronize_session=False)


class AccountLifecycleService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork, emby_client=None):
        self._uow_factory = uow_factory
        self._tasks = TaskService(uow_factory)
        self._users = UserService(uow_factory)
        self._points = PointService(uow_factory)
        self._notifications = NotificationService(uow_factory)
        self._emby_client = emby_client

    def enqueue_batch(self, *, action: str, tg_ids: list[int], parameters: dict, actor: Actor, idempotency_key: str, account_ids: Optional[list[str]] = None) -> ServiceResult:
        if action not in LIFECYCLE_ACTIONS:
            return ServiceResult("unsupported_action")
        targets = {int(item) for item in tg_ids if int(item) > 0}
        with self._uow_factory() as uow:
            for account_id in account_ids or []:
                account = uow.accounts.get(str(account_id))
                if account:
                    targets.add(int(account.legacy_tg))
        targets = sorted(targets)
        if not targets or len(targets) > 500:
            return ServiceResult("invalid_targets")
        with self._uow_factory() as uow:
            existing = {int(item[0]) for item in uow.operations.session.query(Emby.tg).filter(Emby.tg.in_(targets)).all()}
        if not existing:
            return ServiceResult("invalid_targets")
        parameters = dict(parameters or {})
        try:
            self._validate_parameters(action, parameters)
        except (TypeError, ValueError):
            return ServiceResult("invalid_parameters")
        parameters.setdefault("batch_id", str(uuid4()))
        return self._tasks.enqueue(
            task_type="users.batch",
            payload={
                "action": action,
                "tg_ids": sorted(existing),
                "parameters": parameters,
                "requested_by": {"kind": actor.kind, "identifier": actor.identifier},
            },
            actor=actor,
            idempotency_key=idempotency_key,
        )

    @staticmethod
    def _validate_parameters(action: str, parameters: dict) -> None:
        if action == "extend":
            days = int(parameters.get("days", 0))
            if days < 1 or days > 3650:
                raise ValueError("延期天数无效")
        elif action in {"grant_coins", "grant_registration_days"}:
            amount = int(parameters.get("amount", 0))
            if amount == 0 or abs(amount) > 10_000_000:
                raise ValueError("权益调整数量无效")
        elif action == "notify":
            title = str(parameters.get("title") or "").strip()
            body = str(parameters.get("body") or "").strip()
            if not title or not body or len(title) > 200 or len(body) > 2000:
                raise ValueError("通知内容无效")
            if parameters.get("severity", "info") not in {"info", "success", "warning", "danger"}:
                raise ValueError("通知级别无效")

    async def execute_batch(self, payload: dict) -> dict:
        action = payload["action"]
        parameters = payload.get("parameters") or {}
        batch_id = str(parameters.get("batch_id") or "")
        if not batch_id:
            raise ValueError("批量任务缺少 batch_id")
        requested_by = payload.get("requested_by") or {}
        actor = Actor(
            kind=str(requested_by.get("kind") or "system")[:32],
            identifier=str(requested_by.get("identifier") or "account-lifecycle-worker")[:128],
        )
        results = []
        for tg in payload.get("tg_ids") or []:
            replay = self._existing(batch_id, int(tg), action)
            if replay is not None:
                results.append({"tg": int(tg), "ok": replay["status"] == "succeeded", "detail": replay["detail"], "replayed": True})
                continue
            try:
                detail = await self._apply(int(tg), action, parameters)
                self._record(batch_id, int(tg), action, "succeeded", detail, actor)
                results.append({"tg": int(tg), "ok": True, "detail": detail})
            except Exception as error:
                detail = {"error": f"{type(error).__name__}: {error}"}
                self._record(batch_id, int(tg), action, "failed", detail, actor)
                results.append({"tg": int(tg), "ok": False, "detail": detail})
        succeeded = sum(1 for item in results if item["ok"])
        return {"action": action, "total": len(results), "succeeded": succeeded, "failed": len(results) - succeeded, "items": results[:100]}

    async def _apply(self, tg: int, action: str, parameters: dict) -> dict:
        with self._uow_factory() as uow:
            user = uow.users.get(tg)
            if user is None:
                raise RuntimeError("用户不存在")
            embyid = user.embyid
            current_expiry = user.ex
        client = self._emby_client
        if client is None and action in {"suspend", "restore", "clear_account"}:
            from bot.func_helper.emby import emby as client
        actor = Actor.system("account-lifecycle-worker")
        if action == "suspend":
            if embyid and not await client.emby_change_policy(str(embyid), disable=True):
                raise RuntimeError("Emby 禁用账号失败")
            self._notify(tg, "账号已暂停", "你的 Emby 账号已被管理员暂停。如有疑问请提交工单。", "warning")
            self._set_account_status(tg, "suspended")
            return {"embyid": embyid}
        if action == "restore":
            if embyid and not await client.emby_change_policy(str(embyid), disable=False):
                raise RuntimeError("Emby 恢复账号失败")
            self._notify(tg, "账号已恢复", "你的 Emby 账号已恢复使用。", "success")
            self._set_account_status(tg, "active")
            return {"embyid": embyid}
        if action == "clear_account":
            if embyid and not await client.emby_del(str(embyid)):
                raise RuntimeError("Emby 删除账号失败")
            result = self._users.update_user(tg, {"embyid": None, "name": None, "pwd": None, "pwd2": None, "lv": "d", "cr": None, "ex": None}, actor, action="user.lifecycle.clear", idempotency_key=f"lifecycle:clear_account:{tg}:{parameters.get('batch_id', '')}")
            if not result.ok:
                raise RuntimeError(result.status)
            self._notify(tg, "账号已清理", "你的 Emby 账号已被清理，Telegram 用户资料仍然保留。", "warning")
            self._expire_membership(tg, "expired")
            return {"cleared": True}
        if action == "extend":
            days = max(1, min(3650, int(parameters.get("days", 0))))
            expires_at = max(current_expiry or utcnow(), utcnow()) + timedelta(days=days)
            result = self._users.update_user(tg, {"ex": expires_at}, actor, action="user.lifecycle.extend", idempotency_key=f"lifecycle:extend:{tg}:{parameters.get('batch_id', '')}")
            if not result.ok:
                raise RuntimeError(result.status)
            self._notify(tg, "账号有效期已延长", f"你的账号有效期已延长 {days} 天。", "success")
            self._extend_membership(tg, expires_at)
            return {"days": days, "expires_at": expires_at}
        if action in {"grant_coins", "grant_registration_days"}:
            amount = int(parameters.get("amount", 0))
            if amount == 0 or abs(amount) > 10_000_000:
                raise ValueError("调整数量无效")
            balance_type = "coins" if action == "grant_coins" else "registration_days"
            result = self._points.adjust(tg=tg, amount=amount, balance_type=balance_type, reason=str(parameters.get("reason") or "批量运营调整")[:255], actor=actor, allow_negative=False, idempotency_key=f"lifecycle:{action}:{tg}:{parameters.get('batch_id', '')}")
            if not result.ok:
                raise RuntimeError(result.status)
            self._notify(tg, "账户权益已更新", f"管理员已调整你的{'积分' if balance_type == 'coins' else '注册天数'}：{amount:+d}。", "info")
            return result.data
        if action == "notify":
            self._notify(tg, str(parameters.get("title") or "系统通知")[:200], str(parameters.get("body") or "")[:2000], str(parameters.get("severity") or "info"))
            return {"notified": True}
        raise RuntimeError("不支持的账号动作")

    def _notify(self, tg: int, title: str, body: str, severity: str) -> None:
        self._notifications.broadcast(target_tg=tg, category="system", title=title, body=body, severity=severity if severity in {"info", "success", "warning", "danger"} else "info", action_url="/account", actor=Actor.system("account-lifecycle-worker"))

    def _existing(self, batch_id: str, tg: int, action: str) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.operations.session.query(AccountLifecycleEvent).filter(
                AccountLifecycleEvent.batch_id == batch_id,
                AccountLifecycleEvent.tg == tg,
                AccountLifecycleEvent.action == action,
            ).first()
            if row is None:
                return None
            return {"status": row.status, "detail": _loads(row.detail_json) or {}}

    def _record(self, batch_id: str, tg: int, action: str, status: str, detail: dict, actor: Actor) -> None:
        with self._uow_factory() as uow:
            account = uow.accounts.by_legacy_tg(tg)
            uow.operations.session.add(AccountLifecycleEvent(account_id=account.id if account else None, batch_id=batch_id, tg=tg, action=action, status=status, detail_json=_json(detail), actor_kind=actor.kind, actor_id=actor.identifier, created_at=utcnow()))
            uow.operations.audit(actor=actor, action=f"user.lifecycle.{action}", resource_type="user", resource_id=str(tg), outcome="success" if status == "succeeded" else "failed", detail=detail)
            uow.operations.event("user.lifecycle.updated", "user", str(tg), {"tg": tg, "action": action, "status": status})

    def _set_account_status(self, tg: int, status: str) -> None:
        with self._uow_factory() as uow:
            account = uow.accounts.by_legacy_tg(tg, for_update=True)
            if account:
                account.status = status
                account.updated_at = utcnow()
            membership = uow.accounts.active_membership(account.id, for_update=True) if account else None
            if membership:
                membership.status = "suspended" if status == "suspended" else "active"
                membership.updated_at = utcnow()

    def _expire_membership(self, tg: int, status: str) -> None:
        with self._uow_factory() as uow:
            account = uow.accounts.by_legacy_tg(tg)
            membership = uow.accounts.active_membership(account.id, for_update=True) if account else None
            if membership:
                membership.status = status
                membership.updated_at = utcnow()

    def _extend_membership(self, tg: int, expires_at) -> None:
        with self._uow_factory() as uow:
            account = uow.accounts.by_legacy_tg(tg)
            membership = uow.accounts.active_membership(account.id, for_update=True) if account else None
            if membership:
                membership.expires_at = expires_at
                membership.status = "active"
                membership.updated_at = utcnow()

    def history(self, *, tg: Optional[int] = None, account_id: Optional[str] = None, limit: int = 100) -> dict:
        with self._uow_factory() as uow:
            query = uow.operations.session.query(AccountLifecycleEvent)
            if tg is not None:
                query = query.filter(AccountLifecycleEvent.tg == tg)
            if account_id:
                query = query.filter(AccountLifecycleEvent.account_id == account_id)
            rows = query.order_by(AccountLifecycleEvent.created_at.desc()).limit(limit).all()
            return {"items": [{"id": row.id, "account_id": row.account_id, "batch_id": row.batch_id, "tg": row.tg, "action": row.action, "status": row.status, "detail": _loads(row.detail_json), "actor_kind": row.actor_kind, "actor_id": row.actor_id, "created_at": row.created_at} for row in rows]}
