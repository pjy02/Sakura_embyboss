from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import timedelta
from typing import Any, Optional

from sqlalchemy import func
from sqlalchemy.exc import IntegrityError

from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import (
    ConfigRevision,
    DynamicSetting,
    SecurityEvent,
    utcnow,
)


RISK_STATUSES = {"open", "acknowledged", "resolved", "ignored"}
RISK_SEVERITIES = {"info", "warning", "danger"}


def _loads(value):
    if value is None:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


def _json(value) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str)


def serialize_risk_event(row: SecurityEvent) -> dict:
    return {
        "id": row.id,
        "event_type": row.event_type,
        "severity": row.severity,
        "subject_kind": row.subject_kind,
        "subject_id": row.subject_id,
        "ip_address": row.ip_address,
        "detail": _loads(row.detail_json),
        "status": row.status,
        "assigned_to": row.assigned_to,
        "resolution_note": row.resolution_note,
        "resolved_by": row.resolved_by,
        "resolved_at": row.resolved_at,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


class RiskEventService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def list(
        self,
        *,
        search=None,
        severity=None,
        status=None,
        event_type=None,
        limit=50,
        offset=0,
    ) -> dict:
        if severity and severity not in RISK_SEVERITIES:
            raise ValueError("未知风险级别")
        if status and status not in RISK_STATUSES:
            raise ValueError("未知风险状态")
        with self._uow_factory() as uow:
            rows, total = uow.operations.list_security_events(
                search=search,
                severity=severity,
                status=status,
                event_type=event_type,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_risk_event(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def get(self, event_id: int) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.operations.get_security_event(event_id)
            return serialize_risk_event(row) if row else None

    def summary(self) -> dict:
        now = utcnow()
        with self._uow_factory() as uow:
            session = uow.operations.session
            open_statuses = ("open", "acknowledged")
            severity_counts = {
                severity: int(
                    session.query(func.count(SecurityEvent.id))
                    .filter(
                        SecurityEvent.status.in_(open_statuses),
                        SecurityEvent.severity == severity,
                    )
                    .scalar()
                    or 0
                )
                for severity in ("info", "warning", "danger")
            }
            status_counts = {
                status: int(
                    session.query(func.count(SecurityEvent.id))
                    .filter(SecurityEvent.status == status)
                    .scalar()
                    or 0
                )
                for status in ("open", "acknowledged", "resolved", "ignored")
            }
            recent_24h = int(
                session.query(func.count(SecurityEvent.id))
                .filter(SecurityEvent.created_at >= now - timedelta(days=1))
                .scalar()
                or 0
            )
            top_types = [
                {"event_type": event_type, "count": int(count)}
                for event_type, count in (
                    session.query(
                        SecurityEvent.event_type,
                        func.count(SecurityEvent.id),
                    )
                    .filter(SecurityEvent.created_at >= now - timedelta(days=7))
                    .group_by(SecurityEvent.event_type)
                    .order_by(func.count(SecurityEvent.id).desc())
                    .limit(8)
                    .all()
                )
            ]
            return {
                "open_total": sum(severity_counts.values()),
                "severity_counts": severity_counts,
                "status_counts": status_counts,
                "recent_24h": recent_24h,
                "top_types": top_types,
                "checked_at": now,
            }

    def update(
        self,
        event_id: int,
        *,
        status: str,
        assigned_to: Optional[int],
        resolution_note: Optional[str],
        actor: Actor,
    ) -> Optional[dict]:
        if status not in RISK_STATUSES:
            raise ValueError("未知风险状态")
        note = (resolution_note or "").strip() or None
        if note and len(note) > 1000:
            raise ValueError("处理说明不能超过 1000 个字符")
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.operations.get_security_event(event_id, for_update=True)
            if row is None:
                return None
            previous = {
                "status": row.status,
                "assigned_to": row.assigned_to,
                "resolution_note": row.resolution_note,
            }
            row.status = status
            row.assigned_to = assigned_to
            row.resolution_note = note
            row.updated_at = now
            if status in {"resolved", "ignored"}:
                row.resolved_by = (
                    int(actor.identifier) if actor.identifier.isdigit() else None
                )
                row.resolved_at = now
            else:
                row.resolved_by = None
                row.resolved_at = None
            uow.operations.audit(
                actor=actor,
                action="security.event.update",
                resource_type="security_event",
                resource_id=str(event_id),
                detail={
                    "before": previous,
                    "after": {
                        "status": status,
                        "assigned_to": assigned_to,
                        "resolution_note": note,
                    },
                },
            )
            uow.operations.event(
                "security.updated",
                "security",
                str(event_id),
                {
                    "resource_type": "security_event",
                    "resource_id": str(event_id),
                    "status": status,
                    "severity": row.severity,
                },
            )
            uow.flush()
            return serialize_risk_event(row)


@dataclass(frozen=True)
class SettingDefinition:
    key: str
    group: str
    label: str
    description: str
    value_type: str
    default: Any
    runtime_path: tuple[str, ...]
    minimum: Optional[int] = None
    maximum: Optional[int] = None
    options: tuple[str, ...] = ()
    restart_required: bool = False


SETTING_DEFINITIONS = (
    SettingDefinition("registration.enabled", "注册与容量", "开放注册", "控制用户是否可以创建新的 Emby 账号。", "boolean", False, ("_open", "stat")),
    SettingDefinition("registration.user_limit", "注册与容量", "账号容量上限", "达到上限后停止接受新的注册任务。", "integer", 1000, ("_open", "all_user"), 1, 100000),
    SettingDefinition("registration.worker_count", "注册与容量", "注册并发数", "同时处理注册任务的 Worker 数量。", "integer", 5, ("_open", "register_worker_count"), 1, 50),
    SettingDefinition("registration.queue_limit", "注册与容量", "注册队列长度", "允许等待处理的注册任务数量。", "integer", 100, ("_open", "register_queue_limit"), 1, 10000),
    SettingDefinition("economy.checkin_enabled", "积分与权益", "开放签到", "允许用户通过签到获得积分。", "boolean", True, ("_open", "checkin")),
    SettingDefinition("economy.exchange_enabled", "积分与权益", "开放兑换", "允许用户消耗积分兑换注册资格。", "boolean", True, ("_open", "exchange")),
    SettingDefinition("economy.exchange_cost", "积分与权益", "注册码兑换成本", "兑换一次注册资格需要的积分。", "integer", 300, ("_open", "exchange_cost"), 0, 100000000),
    SettingDefinition("economy.whitelist_cost", "积分与权益", "白名单兑换成本", "兑换白名单资格需要的积分。", "integer", 9999, ("_open", "whitelist_cost"), 0, 100000000),
    SettingDefinition("economy.invite_enabled", "积分与权益", "开放邀请码", "允许用户兑换邀请码。", "boolean", False, ("_open", "invite")),
    SettingDefinition("economy.invite_cost", "积分与权益", "邀请码成本", "兑换邀请码需要的积分。", "integer", 1000, ("_open", "invite_cost"), 0, 100000000),
    SettingDefinition("playback.client_filter_enabled", "播放策略", "启用客户端过滤", "按客户端规则检测和处理播放会话。", "boolean", False, ("config", "client_filter_enabled")),
    SettingDefinition("playback.client_filter_mode", "播放策略", "客户端过滤模式", "黑名单命中时拦截，白名单未命中时拦截。", "string", "blacklist", ("config", "client_filter_mode"), options=("blacklist", "whitelist")),
    SettingDefinition("playback.terminate_blocked_client", "播放策略", "终止违规客户端", "检测到违规客户端时立即终止会话。", "boolean", True, ("config", "client_filter_terminate_session")),
    SettingDefinition("playback.block_user_for_client", "播放策略", "封禁违规客户端用户", "检测到违规客户端时同步封禁用户。", "boolean", False, ("config", "client_filter_block_user")),
    SettingDefinition("playback.terminate_line_violation", "播放策略", "终止线路违规", "检测到线路权限违规时终止会话。", "boolean", True, ("config", "line_filter_terminate_session")),
    SettingDefinition("playback.block_user_for_line", "播放策略", "封禁线路违规用户", "检测到线路权限违规时同步封禁用户。", "boolean", False, ("config", "line_filter_block_user")),
    SettingDefinition("scheduler.expiry_check", "定时任务", "到期检查", "定时检查并处理到期账号。", "boolean", True, ("schedall", "check_ex"), restart_required=True),
    SettingDefinition("scheduler.partition_check", "定时任务", "分区权限检查", "定时同步用户媒体分区权限。", "boolean", True, ("schedall", "partition_check"), restart_required=True),
    SettingDefinition("scheduler.low_activity_check", "定时任务", "低活跃检查", "定期处理长期没有播放记录的账号。", "boolean", False, ("schedall", "low_activity"), restart_required=True),
    SettingDefinition("scheduler.database_backup", "定时任务", "自动数据库备份", "允许计划任务自动备份数据库。", "boolean", True, ("schedall", "backup_db"), restart_required=True),
    SettingDefinition("integrations.moviepilot_enabled", "外部集成", "启用 MoviePilot", "同步 MoviePilot 下载任务和求片进度。", "boolean", False, ("moviepilot", "status"), restart_required=True),
    SettingDefinition("integrations.moviepilot_price", "外部集成", "MoviePilot 单价", "求片下载使用的积分单价。", "integer", 1, ("moviepilot", "price"), 0, 100000),
)
SETTING_REGISTRY = {item.key: item for item in SETTING_DEFINITIONS}


class UnknownSettingError(KeyError):
    pass


class SettingConflictError(RuntimeError):
    pass


class InvalidSettingValue(ValueError):
    pass


class DynamicSettingsService:
    def __init__(
        self,
        uow_factory=SqlAlchemyUnitOfWork,
        runtime_values: Optional[dict[str, Any]] = None,
    ):
        self._uow_factory = uow_factory
        self._runtime_values = runtime_values

    def _definition(self, key: str) -> SettingDefinition:
        definition = SETTING_REGISTRY.get(key)
        if definition is None:
            raise UnknownSettingError(key)
        return definition

    def _runtime_value(self, definition: SettingDefinition):
        if self._runtime_values is not None:
            return self._runtime_values.get(definition.key, definition.default)
        import bot

        value = bot
        for part in definition.runtime_path:
            value = getattr(value, part)
        return value

    def _write_runtime_value(self, definition: SettingDefinition, value) -> None:
        if self._runtime_values is not None:
            self._runtime_values[definition.key] = value
            return
        import bot

        target = bot
        for part in definition.runtime_path[:-1]:
            target = getattr(target, part)
        setattr(target, definition.runtime_path[-1], value)

    @staticmethod
    def _validate(definition: SettingDefinition, value):
        if definition.value_type == "boolean":
            if type(value) is not bool:
                raise InvalidSettingValue("设置值必须是布尔值")
        elif definition.value_type == "integer":
            if type(value) is not int:
                raise InvalidSettingValue("设置值必须是整数")
            if definition.minimum is not None and value < definition.minimum:
                raise InvalidSettingValue(f"设置值不能小于 {definition.minimum}")
            if definition.maximum is not None and value > definition.maximum:
                raise InvalidSettingValue(f"设置值不能大于 {definition.maximum}")
        elif definition.value_type == "string":
            if not isinstance(value, str):
                raise InvalidSettingValue("设置值必须是字符串")
            if definition.options and value not in definition.options:
                raise InvalidSettingValue("设置值不在允许范围内")
        return value

    def _serialize(self, definition: SettingDefinition, row=None) -> dict:
        value = _loads(row.value_json) if row else self._runtime_value(definition)
        return {
            "key": definition.key,
            "group": definition.group,
            "label": definition.label,
            "description": definition.description,
            "value": value,
            "value_type": definition.value_type,
            "source": "database" if row else "config",
            "revision": int(row.revision) if row else 0,
            "minimum": definition.minimum,
            "maximum": definition.maximum,
            "options": list(definition.options),
            "restart_required": definition.restart_required,
            "updated_by": (
                f"{row.updated_by_kind}:{row.updated_by_id}" if row else None
            ),
            "updated_at": row.updated_at if row else None,
        }

    def list(self) -> dict:
        with self._uow_factory() as uow:
            saved = {
                row.setting_key: row for row in uow.operations.list_dynamic_settings()
            }
            return {
                "items": [
                    self._serialize(definition, saved.get(definition.key))
                    for definition in SETTING_DEFINITIONS
                ]
            }

    def get(self, key: str) -> dict:
        definition = self._definition(key)
        with self._uow_factory() as uow:
            return self._serialize(
                definition,
                uow.operations.get_dynamic_setting(key),
            )

    def update(
        self,
        key: str,
        *,
        value,
        expected_revision: int,
        actor: Actor,
        action: str = "setting.update",
    ) -> dict:
        definition = self._definition(key)
        value = self._validate(definition, value)
        now = utcnow()
        with self._uow_factory() as uow:
            row = uow.operations.get_dynamic_setting(key, for_update=True)
            current_revision = int(row.revision) if row else 0
            if expected_revision != current_revision:
                raise SettingConflictError("设置已被其他管理员修改，请刷新后重试")
            old_value = (
                _loads(row.value_json) if row else self._runtime_value(definition)
            )
            new_revision = current_revision + 1
            if row is None:
                row = DynamicSetting(
                    setting_key=key,
                    value_json=_json(value),
                    value_type=definition.value_type,
                    is_secret=False,
                    revision=new_revision,
                    updated_by_kind=actor.kind,
                    updated_by_id=actor.identifier,
                    created_at=now,
                    updated_at=now,
                )
                uow.operations.add_dynamic_setting(row)
            else:
                row.value_json = _json(value)
                row.value_type = definition.value_type
                row.revision = new_revision
                row.updated_by_kind = actor.kind
                row.updated_by_id = actor.identifier
                row.updated_at = now
            uow.operations.add_config_revision(
                ConfigRevision(
                    setting_key=key,
                    revision=new_revision,
                    old_value_json=_json(old_value),
                    new_value_json=_json(value),
                    actor_kind=actor.kind,
                    actor_id=actor.identifier,
                    created_at=now,
                )
            )
            uow.operations.audit(
                actor=actor,
                action=action,
                resource_type="dynamic_setting",
                resource_id=key,
                detail={
                    "revision": new_revision,
                    "restart_required": definition.restart_required,
                    "old_value": old_value,
                    "new_value": value,
                },
            )
            uow.operations.event(
                "setting.updated",
                "setting",
                key,
                {
                    "resource_type": "dynamic_setting",
                    "resource_id": key,
                    "revision": new_revision,
                    "restart_required": definition.restart_required,
                },
            )
            try:
                uow.flush()
            except IntegrityError as error:
                raise SettingConflictError(
                    "设置已被其他管理员修改，请刷新后重试"
                ) from error
            result = self._serialize(definition, row)
        self._write_runtime_value(definition, value)
        return result

    def update_latest(
        self,
        key: str,
        *,
        value,
        actor: Actor,
        action: str = "setting.update",
        retries: int = 2,
    ) -> dict:
        """Persist a fresh Telegram action while retaining revision safety."""
        for attempt in range(max(0, retries) + 1):
            current = self.get(key)
            try:
                return self.update(
                    key,
                    value=value,
                    expected_revision=current["revision"],
                    actor=actor,
                    action=action,
                )
            except SettingConflictError:
                if attempt >= retries:
                    raise
        raise SettingConflictError("设置更新冲突")

    def history(self, key: str, limit: int = 30) -> dict:
        self._definition(key)
        with self._uow_factory() as uow:
            rows = uow.operations.list_config_revisions(key, limit)
            return {
                "items": [
                    {
                        "id": row.id,
                        "setting_key": row.setting_key,
                        "revision": row.revision,
                        "old_value": _loads(row.old_value_json),
                        "new_value": _loads(row.new_value_json),
                        "actor_kind": row.actor_kind,
                        "actor_id": row.actor_id,
                        "created_at": row.created_at,
                    }
                    for row in rows
                ]
            }

    def rollback(
        self,
        key: str,
        *,
        target_revision: int,
        expected_revision: int,
        actor: Actor,
    ) -> dict:
        self._definition(key)
        with self._uow_factory() as uow:
            target = uow.operations.get_config_revision(key, target_revision)
            if target is None:
                raise UnknownSettingError(f"{key}@{target_revision}")
            value = _loads(target.new_value_json)
        return self.update(
            key,
            value=value,
            expected_revision=expected_revision,
            actor=actor,
            action="setting.rollback",
        )

    def apply_runtime_overrides(self) -> dict:
        applied = []
        with self._uow_factory() as uow:
            rows = uow.operations.list_dynamic_settings()
            for row in rows:
                definition = SETTING_REGISTRY.get(row.setting_key)
                if definition is None:
                    continue
                value = self._validate(definition, _loads(row.value_json))
                self._write_runtime_value(definition, value)
                applied.append(row.setting_key)
        return {"applied": applied, "count": len(applied)}
