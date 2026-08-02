import json
from datetime import timedelta, timezone
from typing import Optional

from sqlalchemy import func, or_

from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import AuditLog, PointTransaction, utcnow
from bot.sql_helper.sql_emby import Emby


def _safe_json(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


def _database_datetime(value):
    if value is None or value.tzinfo is None:
        return value
    return value.astimezone(timezone.utc).replace(tzinfo=None)


def serialize_user(user: Emby) -> dict:
    return {
        "tg": user.tg,
        "embyid": user.embyid,
        "name": user.name,
        "level": user.lv,
        "created_at": user.cr,
        "expires_at": user.ex,
        "registration_days": int(user.us or 0),
        "coins": int(user.iv or 0),
        "checked_in_at": user.ch,
        "has_account": bool(user.embyid),
    }


class AdminQueryService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def get_user(self, tg: int) -> Optional[dict]:
        with self._uow_factory() as uow:
            user = uow.users.get(tg)
            return serialize_user(user) if user else None

    def list_users(
        self,
        *,
        search: Optional[str] = None,
        level: Optional[str] = None,
        sort_by: str = "tg",
        sort_order: str = "desc",
        limit: int = 50,
        offset: int = 0,
    ) -> dict:
        with self._uow_factory() as uow:
            query = uow.users.session.query(Emby)
            if search:
                pattern = f"%{search.strip()}%"
                conditions = [
                    Emby.name.like(pattern),
                    Emby.embyid.like(pattern),
                ]
                if search.strip().lstrip("-").isdigit():
                    conditions.append(Emby.tg == int(search))
                query = query.filter(or_(*conditions))
            if level:
                query = query.filter(Emby.lv == level)
            total = query.count()
            sort_columns = {
                "tg": Emby.tg,
                "name": Emby.name,
                "created_at": Emby.cr,
                "expires_at": Emby.ex,
                "coins": Emby.iv,
            }
            sort_column = sort_columns.get(sort_by, Emby.tg)
            ordering = sort_column.asc() if sort_order == "asc" else sort_column.desc()
            rows = (
                query.order_by(ordering)
                .offset(offset)
                .limit(limit)
                .all()
            )
            return {
                "items": [serialize_user(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def point_transactions(self, tg: int, limit: int, offset: int) -> list[dict]:
        with self._uow_factory() as uow:
            rows = uow.auth.list_point_transactions(tg, limit, offset)
            return [self._serialize_point_transaction(row) for row in rows]

    def audit_logs(
        self,
        *,
        search=None,
        actor_kind=None,
        actor_id=None,
        action=None,
        resource_type=None,
        outcome=None,
        date_from=None,
        date_to=None,
        limit=50,
        offset=0,
    ) -> dict:
        with self._uow_factory() as uow:
            rows, total = uow.auth.list_audit_logs(
                search=search,
                actor_kind=actor_kind,
                actor_id=actor_id,
                action=action,
                resource_type=resource_type,
                outcome=outcome,
                date_from=_database_datetime(date_from),
                date_to=_database_datetime(date_to),
                limit=limit,
                offset=offset,
            )
            return {
                "items": [self._serialize_audit(row) for row in rows],
                "total": total,
                "limit": limit,
                "offset": offset,
            }

    def record_audit_export(self, *, actor: Actor, filters: dict, count: int) -> None:
        with self._uow_factory() as uow:
            uow.operations.audit(
                actor=actor,
                action="audit.export",
                resource_type="audit_log",
                resource_id=None,
                detail={"filters": filters, "exported_rows": count},
            )

    def overview(self) -> dict:
        now = utcnow()
        today = now.replace(hour=0, minute=0, second=0, microsecond=0)
        with self._uow_factory() as uow:
            session = uow.users.session
            users_total = session.query(func.count(Emby.tg)).scalar() or 0
            accounts_active = (
                session.query(func.count(Emby.tg))
                .filter(Emby.embyid.isnot(None), Emby.embyid != "")
                .scalar()
                or 0
            )
            expiring_soon = (
                session.query(func.count(Emby.tg))
                .filter(Emby.ex >= now, Emby.ex <= now + timedelta(days=7))
                .scalar()
                or 0
            )
            level_rows = (
                session.query(Emby.lv, func.count(Emby.tg))
                .group_by(Emby.lv)
                .all()
            )
            coins_total = session.query(func.coalesce(func.sum(Emby.iv), 0)).scalar() or 0
            point_changes_today = (
                session.query(func.count(PointTransaction.id))
                .filter(PointTransaction.created_at >= today)
                .scalar()
                or 0
            )
            audit_events_today = (
                session.query(func.count(AuditLog.id))
                .filter(AuditLog.created_at >= today)
                .scalar()
                or 0
            )
            levels = {"a": 0, "b": 0, "c": 0, "d": 0}
            for name, count in level_rows:
                if name in levels:
                    levels[name] = int(count)
            return {
                "users_total": int(users_total),
                "accounts_active": int(accounts_active),
                "expiring_soon": int(expiring_soon),
                "levels": levels,
                "coins_total": int(coins_total),
                "point_changes_today": int(point_changes_today),
                "audit_events_today": int(audit_events_today),
            }

    @staticmethod
    def _serialize_point_transaction(row: PointTransaction) -> dict:
        return {
            "id": row.id,
            "tg": row.tg,
            "balance_type": row.balance_type,
            "amount": row.amount,
            "balance_after": row.balance_after,
            "reason": row.reason,
            "actor_kind": row.actor_kind,
            "actor_id": row.actor_id,
            "metadata": _safe_json(row.metadata_json),
            "created_at": row.created_at,
        }

    @staticmethod
    def _serialize_audit(row: AuditLog) -> dict:
        return {
            "id": row.id,
            "request_id": row.request_id,
            "actor_kind": row.actor_kind,
            "actor_id": row.actor_id,
            "actor_name": row.actor_name,
            "action": row.action,
            "resource_type": row.resource_type,
            "resource_id": row.resource_id,
            "outcome": row.outcome,
            "detail": _safe_json(row.detail_json),
            "ip_address": row.ip_address,
            "created_at": row.created_at,
        }
