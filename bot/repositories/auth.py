from datetime import datetime
from typing import Optional

from sqlalchemy import func, or_
from sqlalchemy.orm import Session

from bot.sql_helper.sql_application import (
    AuditLog,
    PointTransaction,
    SecurityEvent,
    WebLoginRequest,
    WebRole,
    WebRoleMember,
    WebSession,
)


class AuthRepository:
    def __init__(self, session: Session):
        self.session = session

    def add_login_request(self, request: WebLoginRequest) -> None:
        self.session.add(request)

    def get_login_request_by_hash(self, token_hash: str, *, for_update: bool = False):
        query = self.session.query(WebLoginRequest).filter(
            WebLoginRequest.request_token_hash == token_hash
        )
        if for_update:
            query = query.with_for_update()
        return query.first()

    def get_login_request_by_id(self, request_id: str, *, for_update: bool = False):
        query = self.session.query(WebLoginRequest).filter(WebLoginRequest.id == request_id)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def recent_login_request_count(self, ip_address: str, since: datetime) -> int:
        return (
            self.session.query(WebLoginRequest)
            .filter(
                WebLoginRequest.ip_address == ip_address,
                WebLoginRequest.created_at >= since,
            )
            .count()
        )

    def add_session(self, session: WebSession) -> None:
        self.session.add(session)

    def get_session_by_hash(self, token_hash: str, *, for_update: bool = False):
        query = self.session.query(WebSession).filter(WebSession.token_hash == token_hash)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def get_session_by_id(self, session_id: str, *, for_update: bool = False):
        query = self.session.query(WebSession).filter(WebSession.id == session_id)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def revoke_user_sessions(self, tg: int, revoked_at: datetime) -> int:
        return (
            self.session.query(WebSession)
            .filter(WebSession.tg == tg, WebSession.revoked_at.is_(None))
            .update({WebSession.revoked_at: revoked_at}, synchronize_session=False)
        )

    def revoke_account_sessions(self, account_id: str, revoked_at: datetime) -> int:
        return (
            self.session.query(WebSession)
            .filter(WebSession.account_id == account_id, WebSession.revoked_at.is_(None))
            .update({WebSession.revoked_at: revoked_at}, synchronize_session=False)
        )

    def get_roles_for_user(self, tg: int):
        return (
            self.session.query(WebRole)
            .join(WebRoleMember, WebRoleMember.role_id == WebRole.id)
            .filter(WebRoleMember.tg == tg)
            .all()
        )

    def list_roles(self):
        return self.session.query(WebRole).order_by(WebRole.id.asc()).all()

    def get_role(self, role_id: int):
        return self.session.get(WebRole, role_id)

    def get_role_by_name(self, name: str) -> Optional[WebRole]:
        return self.session.query(WebRole).filter(WebRole.name == name).first()

    def add_role(self, row: WebRole):
        self.session.add(row)

    def role_member_count(self, role_id: int):
        return (
            self.session.query(func.count(WebRoleMember.id))
            .filter(WebRoleMember.role_id == role_id)
            .scalar()
            or 0
        )

    def role_member_tgs(self, role_id: int) -> set[int]:
        return {
            int(row[0])
            for row in self.session.query(WebRoleMember.tg)
            .filter(WebRoleMember.role_id == role_id)
            .all()
        }

    def delete_role(self, row: WebRole):
        self.session.delete(row)

    def has_role(self, tg: int, role_id: int) -> bool:
        return (
            self.session.query(WebRoleMember)
            .filter(WebRoleMember.tg == tg, WebRoleMember.role_id == role_id)
            .first()
            is not None
        )

    def add_role_member(self, tg: int, role_id: int, created_by: int) -> None:
        self.session.add(WebRoleMember(tg=tg, role_id=role_id, created_by=created_by))

    def remove_role_member(self, tg: int, role_id: int) -> int:
        return (
            self.session.query(WebRoleMember)
            .filter(WebRoleMember.tg == tg, WebRoleMember.role_id == role_id)
            .delete(synchronize_session=False)
        )

    def recent_security_event_count(
        self,
        event_type: str,
        ip_address: str,
        since: datetime,
    ) -> int:
        return (
            self.session.query(SecurityEvent)
            .filter(
                SecurityEvent.event_type == event_type,
                SecurityEvent.ip_address == ip_address,
                SecurityEvent.created_at >= since,
            )
            .count()
        )

    def list_audit_logs(
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
    ):
        query = self.session.query(AuditLog)
        if search:
            pattern = f"%{search.strip()}%"
            query = query.filter(
                or_(
                    AuditLog.action.like(pattern),
                    AuditLog.actor_id.like(pattern),
                    AuditLog.resource_type.like(pattern),
                    AuditLog.resource_id.like(pattern),
                    AuditLog.request_id.like(pattern),
                )
            )
        if actor_kind:
            query = query.filter(AuditLog.actor_kind == actor_kind)
        if actor_id:
            query = query.filter(AuditLog.actor_id == actor_id)
        if action:
            query = query.filter(AuditLog.action == action)
        if resource_type:
            query = query.filter(AuditLog.resource_type == resource_type)
        if outcome:
            query = query.filter(AuditLog.outcome == outcome)
        if date_from:
            query = query.filter(AuditLog.created_at >= date_from)
        if date_to:
            query = query.filter(AuditLog.created_at <= date_to)
        total = query.count()
        rows = (
            query.order_by(AuditLog.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def list_point_transactions(self, tg: int, limit: int, offset: int):
        return (
            self.session.query(PointTransaction)
            .filter(PointTransaction.tg == tg)
            .order_by(PointTransaction.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
