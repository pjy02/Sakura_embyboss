from datetime import datetime, timedelta
from typing import Optional
from uuid import uuid4

from bot.application.results import ServiceResult
from bot.domain import Actor, secret_fingerprint
from bot.repositories import SqlAlchemyUnitOfWork


class PartitionService:
    RESERVATION_TTL = timedelta(minutes=5)

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def reserve_code(
        self,
        *,
        code_value: str,
        tg: int,
        actor: Actor,
        now: Optional[datetime] = None,
    ) -> ServiceResult:
        now = now or datetime.now()
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user or not user.embyid:
                return ServiceResult("no_account")
            record = uow.partitions.get_code_for_update(code_value)
            if not record:
                return ServiceResult("invalid_code")

            if record.status == "reserved":
                is_stale = not record.reserved_at or record.reserved_at <= now - self.RESERVATION_TTL
                if not is_stale:
                    if record.reserved_by == tg:
                        return ServiceResult(
                            "ok",
                            {
                                "reservation_token": record.reservation_token,
                                "partition": record.partition,
                                "embyid": user.embyid,
                                "embyname": user.name,
                            },
                            replayed=True,
                        )
                    return ServiceResult("code_busy")

            token = str(uuid4())
            record.status = "reserved"
            record.reserved_by = tg
            record.reserved_at = now
            record.reservation_token = token
            uow.operations.audit(
                actor=actor,
                action="partition_code.reserve",
                resource_type="partition_code",
                resource_id=secret_fingerprint(code_value),
                detail={"tg": tg, "partition": record.partition},
            )
            return ServiceResult(
                "ok",
                {
                    "reservation_token": token,
                    "partition": record.partition,
                    "embyid": user.embyid,
                    "embyname": user.name,
                },
            )

    def complete_redemption(
        self,
        *,
        reservation_token: str,
        tg: int,
        actor: Actor,
        now: Optional[datetime] = None,
    ) -> ServiceResult:
        now = now or datetime.now()
        scope = "partition_code.redeem"
        effective_idempotency_key = f"reservation:{reservation_token}"
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user or not user.embyid:
                return ServiceResult("no_account")
            replay = uow.operations.get_idempotent_result(scope, effective_idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)

            record = uow.partitions.get_reservation_for_update(reservation_token)
            if not record or record.status != "reserved" or record.reserved_by != tg:
                return ServiceResult("reservation_invalid")

            code_value = record.code
            partition = record.partition
            expires_at = uow.partitions.complete_grant(
                record=record,
                tg=tg,
                embyid=user.embyid,
                embyname=user.name,
                now=now,
            )
            uow.operations.audit(
                actor=actor,
                action="partition_code.redeem",
                resource_type="partition_grant",
                resource_id=f"{tg}:{partition}",
                detail={
                    "tg": tg,
                    "partition": partition,
                    "code_fingerprint": secret_fingerprint(code_value),
                    "expires_at": expires_at,
                },
            )
            uow.operations.event(
                "partition.changed",
                "user",
                str(tg),
                {"tg": tg, "partition": partition, "expires_at": expires_at},
            )
            result = ServiceResult(
                "ok",
                {"partition": partition, "expires_at": expires_at},
            )
            uow.operations.save_idempotent_result(scope, effective_idempotency_key, result.to_dict())
            return result

    def release_reservation(
        self,
        *,
        reservation_token: str,
        tg: int,
        actor: Actor,
        reason: str,
    ) -> ServiceResult:
        with self._uow_factory() as uow:
            record = uow.partitions.get_reservation_for_update(reservation_token)
            if not record or record.status != "reserved" or record.reserved_by != tg:
                return ServiceResult("reservation_invalid")
            code_value = record.code
            record.status = "available"
            record.reserved_by = None
            record.reserved_at = None
            record.reservation_token = None
            uow.operations.audit(
                actor=actor,
                action="partition_code.release",
                resource_type="partition_code",
                resource_id=secret_fingerprint(code_value),
                outcome="failed",
                detail={"tg": tg, "reason": reason},
            )
            return ServiceResult("ok")
