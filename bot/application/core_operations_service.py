import hashlib
import time
from datetime import timedelta
from typing import Any, Optional
from urllib.parse import urlsplit

import aiohttp
from sqlalchemy import text

from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import utcnow
from bot.sql_helper.sql_operations import (
    KnownDevice,
    LineEndpoint,
    LineHealthSample,
    PlaybackSession,
)


def _device_key(session: dict) -> str:
    raw = str(session.get("DeviceId") or "").strip()
    if raw:
        return raw[:255]
    fingerprint = "|".join(
        str(session.get(name) or "")
        for name in ("UserId", "DeviceName", "Client", "RemoteEndPoint")
    )
    return "derived:" + hashlib.sha256(fingerprint.encode("utf-8")).hexdigest()[:32]


def _playback_payload(session: dict) -> Optional[dict]:
    item = session.get("NowPlayingItem")
    session_id = str(session.get("Id") or "").strip()
    if not isinstance(item, dict) or not session_id:
        return None
    play_state = session.get("PlayState") or {}
    position = int(play_state.get("PositionTicks") or 0)
    runtime = int(item.get("RunTimeTicks") or 0)
    progress = round(position * 100 / runtime, 2) if runtime > 0 else 0.0
    return {
        "session_id": session_id,
        "emby_user_id": str(session.get("UserId") or "") or None,
        "emby_user_name": session.get("UserName"),
        "item_id": str(item.get("Id") or "") or None,
        "item_name": item.get("Name"),
        "series_name": item.get("SeriesName"),
        "item_type": item.get("Type"),
        "client_name": session.get("Client"),
        "app_version": session.get("ApplicationVersion"),
        "device_key": _device_key(session),
        "device_name": session.get("DeviceName"),
        "remote_address": session.get("RemoteEndPoint"),
        "position_ticks": position,
        "runtime_ticks": runtime,
        "progress_percent": progress,
        "is_paused": bool(play_state.get("IsPaused")),
        "is_transcoding": bool(session.get("TranscodingInfo")),
    }


def serialize_playback(row: PlaybackSession) -> dict:
    return {
        "id": row.id,
        "session_id": row.session_id,
        "emby_user_id": row.emby_user_id,
        "emby_user_name": row.emby_user_name,
        "tg": row.tg,
        "item_id": row.item_id,
        "item_name": row.item_name,
        "series_name": row.series_name,
        "item_type": row.item_type,
        "client_name": row.client_name,
        "app_version": row.app_version,
        "device_key": row.device_key,
        "device_name": row.device_name,
        "remote_address": row.remote_address,
        "position_ticks": int(row.position_ticks or 0),
        "runtime_ticks": int(row.runtime_ticks or 0),
        "progress_percent": float(row.progress_percent or 0),
        "is_paused": bool(row.is_paused),
        "is_transcoding": bool(row.is_transcoding),
        "started_at": row.started_at,
        "last_seen_at": row.last_seen_at,
        "ended_at": row.ended_at,
    }


def serialize_device(row: KnownDevice) -> dict:
    return {
        "device_key": row.device_key,
        "emby_user_id": row.emby_user_id,
        "emby_user_name": row.emby_user_name,
        "tg": row.tg,
        "device_name": row.device_name,
        "client_name": row.client_name,
        "app_version": row.app_version,
        "last_ip": row.last_ip,
        "trusted": bool(row.trusted),
        "banned": bool(row.banned),
        "risk_level": row.risk_level,
        "notes": row.notes,
        "playback_count": int(row.playback_count or 0),
        "first_seen_at": row.first_seen_at,
        "last_seen_at": row.last_seen_at,
    }


def serialize_line(row: LineEndpoint) -> dict:
    return {
        "id": row.id,
        "name": row.name,
        "base_url": row.base_url,
        "region": row.region,
        "carrier": row.carrier,
        "audience": row.audience,
        "weight": row.weight,
        "sort_order": row.sort_order,
        "enabled": bool(row.enabled),
        "maintenance": bool(row.maintenance),
        "revision": row.revision,
        "last_status": row.last_status,
        "last_latency_ms": row.last_latency_ms,
        "last_error": row.last_error,
        "last_checked_at": row.last_checked_at,
        "created_at": row.created_at,
        "updated_at": row.updated_at,
    }


class CoreOperationsService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork, emby_client=None):
        self._uow_factory = uow_factory
        self._emby_client = emby_client

    @property
    def emby_client(self):
        if self._emby_client is None:
            from bot.func_helper.emby import emby

            self._emby_client = emby
        return self._emby_client

    async def sync_live_sessions(self) -> dict:
        result = await self.emby_client._request("GET", "/emby/Sessions")
        if not result.success or not isinstance(result.data, list):
            return {
                "items": [],
                "total": 0,
                "source": "unavailable",
                "error": result.error or "Emby 返回了无效数据",
                "synced_at": utcnow(),
            }

        now = utcnow()
        payloads = [
            payload
            for payload in (_playback_payload(item) for item in result.data)
            if payload is not None
        ]
        active_session_ids = {item["session_id"] for item in payloads}
        live_rows: list[PlaybackSession] = []
        with self._uow_factory() as uow:
            lock_held = False
            if uow.session.get_bind().dialect.name == "mysql":
                lock_held = (
                    uow.session.execute(
                        text("SELECT GET_LOCK('sakura_core_operations_sync', 8)")
                    ).scalar()
                    == 1
                )
                if not lock_held:
                    return {
                        "items": [],
                        "total": 0,
                        "source": "unavailable",
                        "error": "核心运营数据正在由另一个进程同步，请稍后重试",
                        "synced_at": now,
                    }
            for payload in payloads:
                uow.core_operations.end_other_playback_items(
                    payload["session_id"], payload["item_id"], now
                )
                row = uow.core_operations.active_playback(payload["session_id"])
                is_new_play = row is None
                account = uow.core_operations.account_for_emby_user(
                    payload["emby_user_id"]
                )
                tg = int(account.tg) if account else None
                if row is None:
                    row = PlaybackSession(
                        **payload,
                        tg=tg,
                        started_at=now,
                        last_seen_at=now,
                    )
                    uow.core_operations.add_playback(row)
                else:
                    for key, value in payload.items():
                        setattr(row, key, value)
                    row.tg = tg
                    row.last_seen_at = now
                    row.updated_at = now

                device = uow.core_operations.get_device(payload["device_key"])
                if device is None:
                    device = KnownDevice(
                        device_key=payload["device_key"],
                        emby_user_id=payload["emby_user_id"],
                        emby_user_name=payload["emby_user_name"],
                        tg=tg,
                        device_name=payload["device_name"],
                        client_name=payload["client_name"],
                        app_version=payload["app_version"],
                        last_ip=payload["remote_address"],
                        playback_count=1,
                        first_seen_at=now,
                        last_seen_at=now,
                    )
                    uow.core_operations.add_device(device)
                else:
                    changed_owner = (
                        device.emby_user_id
                        and payload["emby_user_id"]
                        and device.emby_user_id != payload["emby_user_id"]
                    )
                    device.emby_user_id = payload["emby_user_id"]
                    device.emby_user_name = payload["emby_user_name"]
                    device.tg = tg
                    device.device_name = payload["device_name"]
                    device.client_name = payload["client_name"]
                    device.app_version = payload["app_version"]
                    device.last_ip = payload["remote_address"]
                    device.last_seen_at = now
                    if is_new_play:
                        device.playback_count = int(device.playback_count or 0) + 1
                    if changed_owner and not device.trusted:
                        device.risk_level = "warning"
                live_rows.append(row)
            uow.core_operations.end_missing_playback(active_session_ids, now)
            uow.flush()
            items = [serialize_playback(row) for row in live_rows]
            if lock_held:
                # Make the synchronized snapshot visible before another process
                # can acquire the advisory lock.
                uow.session.commit()
                uow.session.execute(
                    text("SELECT RELEASE_LOCK('sakura_core_operations_sync')")
                )

        return {
            "items": items,
            "total": len(items),
            "source": "emby",
            "error": None,
            "synced_at": now,
        }

    def list_playback(
        self,
        *,
        search: Optional[str] = None,
        active_only: bool = False,
        limit: int = 50,
        offset: int = 0,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.core_operations.list_playback(
                search=search,
                active_only=active_only,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_playback(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    async def stop_playback(
        self,
        session_id: str,
        *,
        reason: str,
        actor: Actor,
    ) -> bool:
        success = await self.emby_client.terminate_session(session_id, reason)
        with self._uow_factory() as uow:
            uow.operations.audit(
                actor=actor,
                action="playback.stop",
                resource_type="playback_session",
                resource_id=session_id,
                outcome="success" if success else "failed",
                detail={"reason": reason},
            )
            if success:
                row = uow.core_operations.active_playback(session_id)
                if row:
                    row.ended_at = utcnow()
                    row.updated_at = row.ended_at
                uow.operations.event(
                    "playback.stopped",
                    "playback_session",
                    session_id,
                    {"reason": reason, "actor": actor.identifier},
                )
        return success

    def list_devices(
        self,
        *,
        search: Optional[str],
        risk_level: Optional[str],
        limit: int,
        offset: int,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.core_operations.list_devices(
                search=search,
                risk_level=risk_level,
                limit=limit,
                offset=offset,
            )
            return {
                "items": [serialize_device(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def update_device(
        self,
        device_key: str,
        *,
        trusted: Optional[bool],
        banned: Optional[bool],
        notes: Optional[str],
        actor: Actor,
    ) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.core_operations.get_device(device_key)
            if row is None:
                return None
            changes: dict[str, Any] = {}
            if trusted is not None:
                row.trusted = trusted
                changes["trusted"] = trusted
            if banned is not None:
                row.banned = banned
                changes["banned"] = banned
            if notes is not None:
                row.notes = notes.strip() or None
                changes["notes"] = row.notes
            row.risk_level = (
                "high" if row.banned else ("normal" if row.trusted else row.risk_level)
            )
            row.updated_at = utcnow()
            uow.operations.audit(
                actor=actor,
                action="device.update",
                resource_type="device",
                resource_id=device_key,
                detail=changes,
            )
            uow.operations.event("device.updated", "device", device_key, changes)
            uow.flush()
            return serialize_device(row)

    def dashboard(self) -> dict:
        now = utcnow()
        today = now.replace(hour=0, minute=0, second=0, microsecond=0)
        with self._uow_factory() as uow:
            result = uow.core_operations.dashboard_counts(today)
            lines = uow.core_operations.list_lines()
            result["line_statuses"] = [
                {
                    "id": row.id,
                    "name": row.name,
                    "status": row.last_status,
                    "latency_ms": row.last_latency_ms,
                    "maintenance": bool(row.maintenance),
                }
                for row in lines[:6]
            ]
            result["checked_at"] = now
            return result

    @staticmethod
    def _normalize_line_url(base_url: str) -> str:
        value = base_url.strip().rstrip("/")
        parsed = urlsplit(value)
        if (
            parsed.scheme not in {"http", "https"}
            or not parsed.hostname
            or parsed.username
            or parsed.password
        ):
            raise ValueError("线路地址必须是无账号信息的 http/https URL")
        return value

    def list_lines(self) -> dict:
        with self._uow_factory() as uow:
            rows = uow.core_operations.list_lines()
            return {"items": [serialize_line(row) for row in rows], "total": len(rows)}

    def public_line_text(self, *, include_whitelist: bool = False) -> str:
        with self._uow_factory() as uow:
            configured = uow.core_operations.list_lines()
            if configured:
                rows = uow.core_operations.public_lines(
                    include_whitelist=include_whitelist
                )
                return (
                    "\n".join(row.base_url for row in rows)
                    if rows
                    else " - 暂无可用线路"
                )

        # Upgrade compatibility: keep the existing config.json values until an
        # administrator has created the first database-backed line.
        import bot

        normal = str(getattr(bot, "emby_line", "") or "").strip()
        whitelist = getattr(bot, "emby_whitelist_line", None)
        if not include_whitelist or not whitelist:
            return normal
        if isinstance(whitelist, (list, tuple)):
            extra = "\n".join(str(item).strip() for item in whitelist if item)
        else:
            extra = str(whitelist).strip()
        return "\n".join(item for item in (normal, extra) if item)

    def create_line(self, data: dict, *, actor: Actor) -> dict:
        base_url = self._normalize_line_url(data["base_url"])
        with self._uow_factory() as uow:
            if uow.core_operations.line_by_url(base_url):
                raise ValueError("该线路地址已存在")
            row = LineEndpoint(
                name=data["name"].strip(),
                base_url=base_url,
                region=(data.get("region") or "").strip() or None,
                carrier=(data.get("carrier") or "").strip() or None,
                audience=data.get("audience", "all"),
                weight=data.get("weight", 100),
                sort_order=data.get("sort_order", 0),
                enabled=data.get("enabled", True),
                maintenance=data.get("maintenance", False),
            )
            uow.core_operations.add_line(row)
            uow.flush()
            uow.operations.audit(
                actor=actor,
                action="line.create",
                resource_type="line",
                resource_id=str(row.id),
                detail={"name": row.name, "base_url": row.base_url},
            )
            uow.operations.event(
                "line.created", "line", str(row.id), {"name": row.name}
            )
            return serialize_line(row)

    def update_line(
        self,
        line_id: int,
        data: dict,
        *,
        actor: Actor,
    ) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.core_operations.get_line(line_id, for_update=True)
            if row is None:
                return None
            expected_revision = data.pop("revision", None)
            if expected_revision is not None and row.revision != expected_revision:
                raise RuntimeError("线路已被其他管理员修改，请刷新后重试")
            changes: dict[str, Any] = {}
            for field in (
                "name",
                "region",
                "carrier",
                "audience",
                "weight",
                "sort_order",
                "enabled",
                "maintenance",
            ):
                if field in data:
                    value = data[field]
                    if isinstance(value, str):
                        value = value.strip() or None
                    setattr(row, field, value)
                    changes[field] = value
            if "base_url" in data:
                value = self._normalize_line_url(data["base_url"])
                duplicate = uow.core_operations.line_by_url(value)
                if duplicate and duplicate.id != row.id:
                    raise ValueError("该线路地址已存在")
                row.base_url = value
                changes["base_url"] = value
            row.revision += 1
            row.updated_at = utcnow()
            uow.operations.audit(
                actor=actor,
                action="line.update",
                resource_type="line",
                resource_id=str(line_id),
                detail=changes,
            )
            uow.operations.event("line.updated", "line", str(line_id), changes)
            uow.flush()
            return serialize_line(row)

    async def probe_line(self, line_id: int, *, actor: Actor) -> Optional[dict]:
        with self._uow_factory() as uow:
            row = uow.core_operations.get_line(line_id)
            if row is None:
                return None
            target = row.base_url

        checked_at = utcnow()
        started = time.perf_counter()
        success = False
        status_code = None
        error = None
        try:
            timeout = aiohttp.ClientTimeout(total=8)
            async with aiohttp.ClientSession(timeout=timeout) as session:
                async with session.get(target, allow_redirects=True) as response:
                    status_code = response.status
                    success = response.status < 500
        except Exception as exc:
            error = f"{type(exc).__name__}: {str(exc)}"[:512]
        latency_ms = int((time.perf_counter() - started) * 1000)

        with self._uow_factory() as uow:
            row = uow.core_operations.get_line(line_id)
            if row is None:
                return None
            row.last_status = "healthy" if success else "offline"
            row.last_latency_ms = latency_ms
            row.last_error = error
            row.last_checked_at = checked_at
            row.updated_at = checked_at
            uow.core_operations.add_line_health(
                LineHealthSample(
                    line_id=line_id,
                    success=success,
                    status_code=status_code,
                    latency_ms=latency_ms,
                    error_message=error,
                    checked_at=checked_at,
                )
            )
            uow.operations.audit(
                actor=actor,
                action="line.probe",
                resource_type="line",
                resource_id=str(line_id),
                outcome="success" if success else "failed",
                detail={
                    "status_code": status_code,
                    "latency_ms": latency_ms,
                    "error": error,
                },
            )
            uow.operations.event(
                "line.probed",
                "line",
                str(line_id),
                {"status": row.last_status, "latency_ms": latency_ms},
            )
            uow.flush()
            return serialize_line(row)

    def line_health(self, line_id: int, limit: int = 30) -> Optional[dict]:
        with self._uow_factory() as uow:
            line = uow.core_operations.get_line(line_id)
            if line is None:
                return None
            rows = uow.core_operations.line_health(line_id, limit)
            return {
                "line": serialize_line(line),
                "items": [
                    {
                        "id": row.id,
                        "success": bool(row.success),
                        "status_code": row.status_code,
                        "latency_ms": row.latency_ms,
                        "error_message": row.error_message,
                        "checked_at": row.checked_at,
                    }
                    for row in rows
                ],
            }
