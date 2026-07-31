from datetime import datetime
from typing import Optional

from sqlalchemy import func, or_
from sqlalchemy.orm import Session

from bot.sql_helper.sql_emby import Emby
from bot.sql_helper.sql_operations import (
    KnownDevice,
    LineEndpoint,
    LineHealthSample,
    PlaybackSession,
)


class CoreOperationsRepository:
    def __init__(self, session: Session):
        self.session = session

    def account_for_emby_user(self, emby_user_id: Optional[str]):
        if not emby_user_id:
            return None
        return (
            self.session.query(Emby)
            .filter(Emby.embyid == str(emby_user_id))
            .first()
        )

    def active_playback(self, session_id: str):
        return (
            self.session.query(PlaybackSession)
            .filter(
                PlaybackSession.session_id == session_id,
                PlaybackSession.ended_at.is_(None),
            )
            .order_by(PlaybackSession.id.desc())
            .first()
        )

    def add_playback(self, row: PlaybackSession) -> None:
        self.session.add(row)

    def end_other_playback_items(
        self,
        session_id: str,
        item_id: Optional[str],
        ended_at: datetime,
    ) -> None:
        query = self.session.query(PlaybackSession).filter(
            PlaybackSession.session_id == session_id,
            PlaybackSession.ended_at.is_(None),
        )
        if item_id:
            query = query.filter(
                or_(
                    PlaybackSession.item_id != item_id,
                    PlaybackSession.item_id.is_(None),
                )
            )
        query.update(
            {
                PlaybackSession.ended_at: ended_at,
                PlaybackSession.updated_at: ended_at,
            },
            synchronize_session=False,
        )

    def end_missing_playback(
        self,
        active_session_ids: set[str],
        ended_at: datetime,
    ) -> None:
        query = self.session.query(PlaybackSession).filter(
            PlaybackSession.ended_at.is_(None)
        )
        if active_session_ids:
            query = query.filter(
                PlaybackSession.session_id.notin_(active_session_ids)
            )
        query.update(
            {
                PlaybackSession.ended_at: ended_at,
                PlaybackSession.updated_at: ended_at,
            },
            synchronize_session=False,
        )

    def list_playback(
        self,
        *,
        search: Optional[str],
        active_only: bool,
        limit: int,
        offset: int,
    ):
        query = self.session.query(PlaybackSession)
        if active_only:
            query = query.filter(PlaybackSession.ended_at.is_(None))
        if search:
            pattern = f"%{search.strip()}%"
            query = query.filter(
                or_(
                    PlaybackSession.emby_user_name.like(pattern),
                    PlaybackSession.item_name.like(pattern),
                    PlaybackSession.series_name.like(pattern),
                    PlaybackSession.device_name.like(pattern),
                    PlaybackSession.client_name.like(pattern),
                    PlaybackSession.remote_address.like(pattern),
                )
            )
        total = query.count()
        rows = (
            query.order_by(PlaybackSession.last_seen_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def get_device(self, device_key: str):
        return self.session.get(KnownDevice, device_key)

    def add_device(self, row: KnownDevice) -> None:
        self.session.add(row)

    def list_devices(
        self,
        *,
        search: Optional[str],
        risk_level: Optional[str],
        limit: int,
        offset: int,
    ):
        query = self.session.query(KnownDevice)
        if risk_level:
            query = query.filter(KnownDevice.risk_level == risk_level)
        if search:
            pattern = f"%{search.strip()}%"
            query = query.filter(
                or_(
                    KnownDevice.device_name.like(pattern),
                    KnownDevice.client_name.like(pattern),
                    KnownDevice.emby_user_name.like(pattern),
                    KnownDevice.last_ip.like(pattern),
                    KnownDevice.device_key.like(pattern),
                )
            )
        total = query.count()
        rows = (
            query.order_by(KnownDevice.last_seen_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def get_line(self, line_id: int, *, for_update: bool = False):
        query = self.session.query(LineEndpoint).filter(LineEndpoint.id == line_id)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def line_by_url(self, base_url: str):
        return (
            self.session.query(LineEndpoint)
            .filter(LineEndpoint.base_url == base_url)
            .first()
        )

    def add_line(self, row: LineEndpoint) -> None:
        self.session.add(row)

    def list_lines(self):
        return (
            self.session.query(LineEndpoint)
            .order_by(LineEndpoint.sort_order.asc(), LineEndpoint.id.asc())
            .all()
        )

    def public_lines(self, *, include_whitelist: bool):
        audiences = ("all", "whitelist") if include_whitelist else ("all",)
        return (
            self.session.query(LineEndpoint)
            .filter(
                LineEndpoint.enabled.is_(True),
                LineEndpoint.maintenance.is_(False),
                LineEndpoint.audience.in_(audiences),
            )
            .order_by(LineEndpoint.sort_order.asc(), LineEndpoint.id.asc())
            .all()
        )

    def add_line_health(self, row: LineHealthSample) -> None:
        self.session.add(row)

    def line_health(self, line_id: int, limit: int):
        return (
            self.session.query(LineHealthSample)
            .filter(LineHealthSample.line_id == line_id)
            .order_by(LineHealthSample.checked_at.desc())
            .limit(limit)
            .all()
        )

    def dashboard_counts(self, since: datetime) -> dict:
        return {
            "live_sessions": int(
                self.session.query(func.count(PlaybackSession.id))
                .filter(PlaybackSession.ended_at.is_(None))
                .scalar()
                or 0
            ),
            "plays_today": int(
                self.session.query(func.count(PlaybackSession.id))
                .filter(PlaybackSession.started_at >= since)
                .scalar()
                or 0
            ),
            "known_devices": int(
                self.session.query(func.count(KnownDevice.device_key)).scalar()
                or 0
            ),
            "risk_devices": int(
                self.session.query(func.count(KnownDevice.device_key))
                .filter(
                    or_(
                        KnownDevice.banned.is_(True),
                        KnownDevice.risk_level.in_(("warning", "high")),
                    )
                )
                .scalar()
                or 0
            ),
            "lines_total": int(
                self.session.query(func.count(LineEndpoint.id)).scalar() or 0
            ),
            "lines_healthy": int(
                self.session.query(func.count(LineEndpoint.id))
                .filter(
                    LineEndpoint.enabled.is_(True),
                    LineEndpoint.maintenance.is_(False),
                    LineEndpoint.last_status == "healthy",
                )
                .scalar()
                or 0
            ),
        }
