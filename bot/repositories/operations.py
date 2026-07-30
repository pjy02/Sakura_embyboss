import json
from datetime import datetime, timezone
from typing import Any, Optional

from sqlalchemy.orm import Session

from bot.domain import Actor
from bot.sql_helper.sql_application import (
    AuditLog,
    IdempotencyRecord,
    PointTransaction,
    SecurityEvent,
    SystemEvent,
)


def _json(value: Any) -> Optional[str]:
    if value is None:
        return None
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str)


class OperationRepository:
    def __init__(self, session: Session):
        self.session = session

    def get_idempotent_result(self, scope: str, key: Optional[str]):
        if not key:
            return None
        row = (
            self.session.query(IdempotencyRecord)
            .filter(
                IdempotencyRecord.scope == scope,
                IdempotencyRecord.idempotency_key == key,
            )
            .first()
        )
        return json.loads(row.result_json) if row and row.result_json else None

    def save_idempotent_result(self, scope: str, key: Optional[str], result: dict) -> None:
        if not key:
            return
        self.session.add(
            IdempotencyRecord(
                scope=scope,
                idempotency_key=key,
                result_json=_json(result),
            )
        )

    def audit(
        self,
        *,
        actor: Actor,
        action: str,
        resource_type: str,
        resource_id: Optional[str],
        outcome: str = "success",
        detail: Any = None,
        request_id: Optional[str] = None,
    ) -> None:
        self.session.add(
            AuditLog(
                request_id=request_id,
                actor_kind=actor.kind,
                actor_id=actor.identifier,
                actor_name=actor.display_name,
                action=action,
                resource_type=resource_type,
                resource_id=resource_id,
                outcome=outcome,
                detail_json=_json(detail),
            )
        )

    def point_transaction(
        self,
        *,
        tg: int,
        balance_type: str,
        amount: int,
        balance_after: int,
        reason: str,
        actor: Actor,
        idempotency_key: Optional[str] = None,
        metadata: Any = None,
    ) -> None:
        self.session.add(
            PointTransaction(
                tg=tg,
                balance_type=balance_type,
                amount=amount,
                balance_after=balance_after,
                reason=reason,
                actor_kind=actor.kind,
                actor_id=actor.identifier,
                idempotency_key=idempotency_key,
                metadata_json=_json(metadata),
            )
        )

    def event(
        self,
        event_type: str,
        aggregate_type: str,
        aggregate_id: Optional[str],
        payload: Any,
    ) -> None:
        self.session.add(
            SystemEvent(
                event_type=event_type,
                aggregate_type=aggregate_type,
                aggregate_id=aggregate_id,
                payload_json=_json(payload),
                created_at=datetime.now(timezone.utc).replace(tzinfo=None),
            )
        )

    def security_event(
        self,
        *,
        event_type: str,
        severity: str = "info",
        subject_kind: Optional[str] = None,
        subject_id: Optional[str] = None,
        ip_address: Optional[str] = None,
        detail: Any = None,
    ) -> None:
        self.session.add(
            SecurityEvent(
                event_type=event_type,
                severity=severity,
                subject_kind=subject_kind,
                subject_id=subject_id,
                ip_address=ip_address,
                detail_json=_json(detail),
            )
        )
