import json
from datetime import datetime, timezone
from typing import Any, Optional

from sqlalchemy import or_
from sqlalchemy.orm import Session

from bot.domain import Actor
from bot.sql_helper.sql_application import (
    AuditLog,
    ConfigRevision,
    DynamicSetting,
    IdempotencyRecord,
    JobRun,
    OperationTask,
    PointTransaction,
    SecurityEvent,
    SystemEvent,
    WorkerHeartbeat,
)
from bot.sql_helper.sql_accounts import Account, AccountLedgerEntry, AccountWallet


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
        account = self.session.query(Account).filter(Account.legacy_tg == tg).first()
        account_id = account.id if account else None
        transaction = PointTransaction(
                account_id=account_id,
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
        self.session.add(transaction)
        self.session.flush()
        if account_id:
            wallet = (
                self.session.query(AccountWallet)
                .filter(
                    AccountWallet.account_id == account_id,
                    AccountWallet.balance_type == balance_type,
                )
                .with_for_update()
                .first()
            )
            if wallet is None:
                wallet = AccountWallet(
                    account_id=account_id,
                    balance_type=balance_type,
                    balance=balance_after,
                    revision=1,
                )
                self.session.add(wallet)
            else:
                wallet.balance = balance_after
                wallet.revision += 1
            self.session.add(
                AccountLedgerEntry(
                    source_transaction_id=transaction.id,
                    account_id=account_id,
                    legacy_tg=tg,
                    balance_type=balance_type,
                    amount=amount,
                    balance_after=balance_after,
                    reason=reason,
                    scope=f"points.{balance_type}",
                    idempotency_key=idempotency_key,
                    actor_kind=actor.kind,
                    actor_id=actor.identifier,
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
    ):
        row = SecurityEvent(
            event_type=event_type,
            severity=severity,
            subject_kind=subject_kind,
            subject_id=subject_id,
            ip_address=ip_address,
            detail_json=_json(detail),
        )
        self.session.add(row)
        self.event(
            "security.created",
            "security",
            subject_id,
            {
                "event_type": event_type,
                "severity": severity,
                "subject_kind": subject_kind,
                "subject_id": subject_id,
            },
        )
        return row

    def get_security_event(self, event_id: int, *, for_update: bool = False):
        query = self.session.query(SecurityEvent).filter(SecurityEvent.id == event_id)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def list_security_events(
        self,
        *,
        search: Optional[str] = None,
        severity: Optional[str] = None,
        status: Optional[str] = None,
        event_type: Optional[str] = None,
        limit: int = 50,
        offset: int = 0,
    ):
        query = self.session.query(SecurityEvent)
        if search:
            pattern = f"%{search}%"
            query = query.filter(
                or_(
                    SecurityEvent.event_type.like(pattern),
                    SecurityEvent.subject_kind.like(pattern),
                    SecurityEvent.subject_id.like(pattern),
                    SecurityEvent.ip_address.like(pattern),
                    SecurityEvent.detail_json.like(pattern),
                )
            )
        if severity:
            query = query.filter(SecurityEvent.severity == severity)
        if status:
            query = query.filter(SecurityEvent.status == status)
        if event_type:
            query = query.filter(SecurityEvent.event_type == event_type)
        total = query.count()
        rows = (
            query.order_by(SecurityEvent.created_at.desc(), SecurityEvent.id.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def get_dynamic_setting(self, key: str, *, for_update: bool = False):
        query = self.session.query(DynamicSetting).filter(
            DynamicSetting.setting_key == key
        )
        if for_update:
            query = query.with_for_update()
        return query.first()

    def list_dynamic_settings(self):
        return self.session.query(DynamicSetting).all()

    def add_dynamic_setting(self, row: DynamicSetting) -> None:
        self.session.add(row)

    def add_config_revision(self, row: ConfigRevision) -> None:
        self.session.add(row)

    def list_config_revisions(self, key: str, limit: int = 30):
        return (
            self.session.query(ConfigRevision)
            .filter(ConfigRevision.setting_key == key)
            .order_by(ConfigRevision.revision.desc())
            .limit(limit)
            .all()
        )

    def get_config_revision(self, key: str, revision: int):
        return (
            self.session.query(ConfigRevision)
            .filter(
                ConfigRevision.setting_key == key,
                ConfigRevision.revision == revision,
            )
            .first()
        )

    def get_task(self, task_id: str, *, for_update: bool = False):
        query = self.session.query(OperationTask).filter(OperationTask.id == task_id)
        if for_update:
            query = query.with_for_update()
        return query.first()

    def get_task_by_idempotency_key(self, key: Optional[str]):
        if not key:
            return None
        return (
            self.session.query(OperationTask)
            .filter(OperationTask.idempotency_key == key)
            .first()
        )

    def add_task(self, task: OperationTask) -> None:
        self.session.add(task)

    def list_tasks(
        self,
        *,
        statuses: Optional[list[str]] = None,
        task_type: Optional[str] = None,
        owner_id: Optional[str] = None,
        limit: int = 50,
        offset: int = 0,
    ):
        query = self.session.query(OperationTask)
        if statuses:
            query = query.filter(OperationTask.status.in_(statuses))
        if task_type:
            query = query.filter(OperationTask.task_type == task_type)
        if owner_id:
            query = query.filter(OperationTask.owner_id == owner_id)
        total = query.count()
        rows = (
            query.order_by(OperationTask.created_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def recover_expired_task_leases(self, now: datetime):
        return (
            self.session.query(OperationTask)
            .filter(
                OperationTask.status == "running",
                OperationTask.lease_expires_at.isnot(None),
                OperationTask.lease_expires_at < now,
            )
            .with_for_update()
            .all()
        )

    def claim_next_task(
        self,
        *,
        worker_id: str,
        now: datetime,
        lease_expires_at: datetime,
    ):
        candidate_ids = [
            row[0]
            for row in (
                self.session.query(OperationTask.id)
                .filter(
                    OperationTask.status.in_(("pending", "retrying")),
                    OperationTask.next_run_at <= now,
                    OperationTask.cancel_requested.is_(False),
                    or_(
                        OperationTask.lease_expires_at.is_(None),
                        OperationTask.lease_expires_at < now,
                    ),
                )
                .order_by(OperationTask.next_run_at.asc(), OperationTask.created_at.asc())
                .limit(10)
                .all()
            )
        ]
        for task_id in candidate_ids:
            updated = (
                self.session.query(OperationTask)
                .filter(
                    OperationTask.id == task_id,
                    OperationTask.status.in_(("pending", "retrying")),
                    OperationTask.cancel_requested.is_(False),
                    or_(
                        OperationTask.lease_expires_at.is_(None),
                        OperationTask.lease_expires_at < now,
                    ),
                )
                .update(
                    {
                        OperationTask.status: "running",
                        OperationTask.locked_by: worker_id,
                        OperationTask.lease_expires_at: lease_expires_at,
                        OperationTask.last_heartbeat_at: now,
                        OperationTask.started_at: now,
                        OperationTask.updated_at: now,
                    },
                    synchronize_session=False,
                )
            )
            if updated:
                self.session.flush()
                return self.get_task(task_id)
        return None

    def list_events_after(
        self,
        *,
        after_id: int,
        limit: int,
        aggregate_type: Optional[str] = None,
        aggregate_id: Optional[str] = None,
        event_prefixes: Optional[tuple[str, ...]] = None,
    ):
        query = self.session.query(SystemEvent).filter(SystemEvent.id > after_id)
        if aggregate_type:
            query = query.filter(SystemEvent.aggregate_type == aggregate_type)
        if aggregate_id:
            query = query.filter(SystemEvent.aggregate_id == aggregate_id)
        if event_prefixes is not None:
            if not event_prefixes:
                return []
            query = query.filter(
                or_(
                    *[
                        SystemEvent.event_type.like(f"{prefix}.%")
                        for prefix in event_prefixes
                    ]
                )
            )
        return query.order_by(SystemEvent.id.asc()).limit(limit).all()

    def list_unpublished_events(self, limit: int = 100):
        return (
            self.session.query(SystemEvent)
            .filter(SystemEvent.published_at.is_(None))
            .order_by(SystemEvent.id.asc())
            .limit(limit)
            .all()
        )

    def mark_events_published(self, event_ids: list[int], published_at: datetime) -> int:
        if not event_ids:
            return 0
        return (
            self.session.query(SystemEvent)
            .filter(
                SystemEvent.id.in_(event_ids),
                SystemEvent.published_at.is_(None),
            )
            .update(
                {SystemEvent.published_at: published_at},
                synchronize_session=False,
            )
        )

    def add_job_run(self, run: JobRun) -> None:
        self.session.add(run)

    def list_job_runs(self, limit: int, offset: int):
        query = self.session.query(JobRun)
        total = query.count()
        rows = (
            query.order_by(JobRun.started_at.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )
        return rows, total

    def upsert_worker_heartbeat(
        self,
        *,
        worker_id: str,
        worker_kind: str,
        hostname: str,
        process_id: int,
        status: str,
        current_task_id: Optional[str],
        metadata: Any,
        now: datetime,
    ) -> None:
        row = (
            self.session.query(WorkerHeartbeat)
            .filter(WorkerHeartbeat.worker_id == worker_id)
            .first()
        )
        if not row:
            self.session.add(
                WorkerHeartbeat(
                    worker_id=worker_id,
                    worker_kind=worker_kind,
                    hostname=hostname,
                    process_id=process_id,
                    status=status,
                    current_task_id=current_task_id,
                    metadata_json=_json(metadata),
                    started_at=now,
                    last_seen_at=now,
                )
            )
            return
        row.worker_kind = worker_kind
        row.hostname = hostname
        row.process_id = process_id
        row.status = status
        row.current_task_id = current_task_id
        row.metadata_json = _json(metadata)
        row.last_seen_at = now

    def list_worker_heartbeats(self):
        return (
            self.session.query(WorkerHeartbeat)
            .order_by(WorkerHeartbeat.last_seen_at.desc())
            .all()
        )
