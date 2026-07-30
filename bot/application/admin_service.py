import json
from typing import Optional

from sqlalchemy import or_

from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_application import AuditLog, PointTransaction
from bot.sql_helper.sql_emby import Emby


def _safe_json(value):
    if not value:
        return None
    try:
        return json.loads(value)
    except (TypeError, ValueError):
        return None


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

    def audit_logs(self, limit: int, offset: int) -> list[dict]:
        with self._uow_factory() as uow:
            rows = uow.auth.list_audit_logs(limit, offset)
            return [self._serialize_audit(row) for row in rows]

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
