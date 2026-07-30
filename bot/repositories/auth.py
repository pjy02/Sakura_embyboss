from datetime import datetime
from typing import Optional

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

    def get_roles_for_user(self, tg: int):
        return (
            self.session.query(WebRole)
            .join(WebRoleMember, WebRoleMember.role_id == WebRole.id)
            .filter(WebRoleMember.tg == tg)
            .all()
        )

    def list_roles(self):
        return self.session.query(WebRole).order_by(WebRole.id.asc()).all()

    def get_role_by_name(self, name: str) -> Optional[WebRole]:
        return self.session.query(WebRole).filter(WebRole.name == name).first()

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

    def list_audit_logs(self, limit: int, offset: int):
        return (
            self.session.query(AuditLog)
            .order_by(AuditLog.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )

    def list_point_transactions(self, tg: int, limit: int, offset: int):
        return (
            self.session.query(PointTransaction)
            .filter(PointTransaction.tg == tg)
            .order_by(PointTransaction.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
