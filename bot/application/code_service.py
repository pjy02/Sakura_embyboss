from datetime import datetime, timedelta
from typing import Optional

from bot.application.results import ServiceResult
from bot.domain import Actor, secret_fingerprint
from bot.repositories import SqlAlchemyUnitOfWork


class CodeService:
    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def redeem_whitelist(
        self,
        *,
        code_value: str,
        tg: int,
        actor: Actor,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        return self._redeem(
            kind="whitelist",
            code_value=code_value,
            tg=tg,
            actor=actor,
            idempotency_key=idempotency_key,
        )

    def redeem_registration(
        self,
        *,
        code_value: str,
        tg: int,
        logo: str,
        actor: Actor,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        return self._redeem(
            kind="registration",
            code_value=code_value,
            tg=tg,
            logo=logo,
            actor=actor,
            idempotency_key=idempotency_key,
        )

    def redeem_renewal(
        self,
        *,
        code_value: str,
        tg: int,
        actor: Actor,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        return self._redeem(
            kind="renewal",
            code_value=code_value,
            tg=tg,
            actor=actor,
            idempotency_key=idempotency_key,
        )

    def _redeem(
        self,
        *,
        kind: str,
        code_value: str,
        tg: int,
        actor: Actor,
        logo: Optional[str] = None,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        scope = f"code.redeem.{kind}"
        now = datetime.now()
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("no_user")

            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)

            if kind in {"whitelist", "renewal"} and not user.embyid:
                return ServiceResult("no_account")
            if kind == "registration":
                if user.embyid:
                    return ServiceResult("has_account")
                if int(user.us or 0) > 0:
                    return ServiceResult("already_qualified")

            code = uow.codes.get_for_update(code_value)
            if not code:
                return ServiceResult("invalid_code")
            if code.used is not None:
                return ServiceResult("used", {"used": code.used})

            if kind == "whitelist":
                if user.lv == "a":
                    return ServiceResult("already_wl")
                user.lv = "a"
                data = {"issuer_tg": code.tg}
            elif kind == "registration":
                prefix = code_value.split("-")[0]
                if prefix != str(logo) and prefix != str(tg):
                    return ServiceResult("forbidden")
                user.us = int(user.us or 0) + int(code.us or 0)
                data = {"issuer_tg": code.tg, "days": code.us}
            elif kind == "renewal":
                current_ex = user.ex or now
                expired = now > current_ex
                ex_new = (now if expired else current_ex) + timedelta(days=int(code.us or 0))
                if expired and user.lv == "c":
                    user.lv = "b"
                user.ex = ex_new
                data = {
                    "issuer_tg": code.tg,
                    "days": code.us,
                    "ex_new": ex_new,
                    "embyid": user.embyid,
                    "restore_policy": expired,
                }
            else:
                raise ValueError(f"Unsupported code kind: {kind}")

            code.used = tg
            code.usedtime = now
            uow.operations.audit(
                actor=actor,
                action=f"code.redeem.{kind}",
                resource_type="registration_code",
                resource_id=secret_fingerprint(code_value),
                detail={"tg": tg, "issuer_tg": code.tg},
            )
            uow.operations.event(
                "code.redeemed",
                "registration_code",
                secret_fingerprint(code_value),
                {"tg": tg, "kind": kind},
            )
            result = ServiceResult("ok", data)
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result

    def purchase_registration_codes(
        self,
        *,
        tg: int,
        codes: list[str],
        days: int,
        cost: int,
        maximum_level: str,
        actor: Actor,
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        scope = "code.purchase.registration"
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("no_user")
            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)
            if user.lv > maximum_level:
                return ServiceResult("level_denied")
            balance = int(user.iv or 0)
            if balance < cost:
                return ServiceResult("insufficient_balance", {"balance": balance})

            new_balance = balance - int(cost)
            uow.codes.add_many(codes, tg, days)
            user.iv = new_balance
            uow.operations.point_transaction(
                tg=tg,
                balance_type="coins",
                amount=-int(cost),
                balance_after=new_balance,
                reason="purchase_registration_codes",
                actor=actor,
                idempotency_key=idempotency_key,
                metadata={"count": len(codes), "days": days},
            )
            uow.operations.audit(
                actor=actor,
                action="code.purchase.registration",
                resource_type="user",
                resource_id=str(tg),
                detail={"count": len(codes), "days": days, "cost": int(cost)},
            )
            uow.operations.event(
                "points.changed",
                "user",
                str(tg),
                {"tg": tg, "balance_type": "coins", "amount": -int(cost), "balance_after": new_balance},
            )
            result = ServiceResult("ok", {"balance": new_balance, "count": len(codes)})
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result
