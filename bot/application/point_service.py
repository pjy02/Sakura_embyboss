from datetime import datetime
from typing import Optional

from bot.application.results import ServiceResult
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork

MAX_INT_VALUE = 2147483647
MIN_INT_VALUE = -2147483648


class PointService:
    BALANCE_FIELDS = {
        "coins": "iv",
        "registration_days": "us",
    }

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def adjust(
        self,
        *,
        tg: int,
        amount: int,
        balance_type: str,
        reason: str,
        actor: Actor,
        allow_negative: bool = False,
        idempotency_key: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> ServiceResult:
        field = self.BALANCE_FIELDS.get(balance_type)
        if field is None:
            raise ValueError(f"Unsupported balance type: {balance_type}")
        scope = f"points.adjust.{balance_type}"

        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("user_not_found")

            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)

            old_balance = int(getattr(user, field) or 0)
            new_balance = old_balance + int(amount)
            if new_balance > MAX_INT_VALUE or new_balance < MIN_INT_VALUE:
                return ServiceResult("overflow", {"balance": old_balance})
            if not allow_negative and new_balance < 0:
                return ServiceResult("insufficient_balance", {"balance": old_balance})

            setattr(user, field, new_balance)
            uow.operations.point_transaction(
                tg=tg,
                balance_type=balance_type,
                amount=int(amount),
                balance_after=new_balance,
                reason=reason,
                actor=actor,
                idempotency_key=idempotency_key,
                metadata=metadata,
            )
            uow.operations.audit(
                actor=actor,
                action="points.adjust",
                resource_type="user",
                resource_id=str(tg),
                detail={
                    "balance_type": balance_type,
                    "amount": int(amount),
                    "balance_after": new_balance,
                    "reason": reason,
                },
            )
            uow.operations.event(
                "points.changed",
                "user",
                str(tg),
                {
                    "tg": tg,
                    "balance_type": balance_type,
                    "amount": int(amount),
                    "balance_after": new_balance,
                },
            )
            result = ServiceResult("ok", {"balance": new_balance, "amount": int(amount)})
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result

    def check_in(
        self,
        *,
        tg: int,
        reward: int,
        occurred_at: datetime,
        actor: Actor,
        maximum_level: Optional[str] = None,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        scope = "points.check_in"
        db_time = occurred_at.replace(tzinfo=None) if occurred_at.tzinfo else occurred_at
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("user_not_found")

            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)
            if maximum_level and user.lv > maximum_level:
                return ServiceResult("level_denied")
            if user.ch and user.ch.date() >= db_time.date():
                return ServiceResult("already_checked_in", {"checked_at": user.ch})

            old_balance = int(user.iv or 0)
            new_balance = old_balance + int(reward)
            if new_balance > MAX_INT_VALUE:
                return ServiceResult("overflow", {"balance": old_balance})

            user.iv = new_balance
            user.ch = db_time
            uow.operations.point_transaction(
                tg=tg,
                balance_type="coins",
                amount=int(reward),
                balance_after=new_balance,
                reason="daily_check_in",
                actor=actor,
                idempotency_key=idempotency_key,
            )
            uow.operations.audit(
                actor=actor,
                action="points.check_in",
                resource_type="user",
                resource_id=str(tg),
                detail={"reward": int(reward), "balance_after": new_balance},
            )
            uow.operations.event(
                "points.changed",
                "user",
                str(tg),
                {"tg": tg, "balance_type": "coins", "amount": int(reward), "balance_after": new_balance},
            )
            result = ServiceResult(
                "ok",
                {"balance": new_balance, "reward": int(reward), "checked_at": db_time},
            )
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result

    def purchase_level(
        self,
        *,
        tg: int,
        target_level: str,
        cost: int,
        actor: Actor,
        reason: str,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        scope = f"points.purchase_level.{target_level}"
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("user_not_found")
            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)
            if not user.embyid:
                return ServiceResult("no_account")
            if user.lv == target_level:
                return ServiceResult("already_owned", {"balance": int(user.iv or 0)})
            balance = int(user.iv or 0)
            if balance < cost:
                return ServiceResult("insufficient_balance", {"balance": balance})

            new_balance = balance - int(cost)
            user.iv = new_balance
            user.lv = target_level
            uow.operations.point_transaction(
                tg=tg,
                balance_type="coins",
                amount=-int(cost),
                balance_after=new_balance,
                reason=reason,
                actor=actor,
                idempotency_key=idempotency_key,
            )
            uow.operations.audit(
                actor=actor,
                action="user.level.purchase",
                resource_type="user",
                resource_id=str(tg),
                detail={"target_level": target_level, "cost": int(cost), "balance_after": new_balance},
            )
            uow.operations.event(
                "user.updated",
                "user",
                str(tg),
                {"tg": tg, "changed_fields": ["lv", "iv"]},
            )
            result = ServiceResult("ok", {"balance": new_balance, "level": target_level})
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result
