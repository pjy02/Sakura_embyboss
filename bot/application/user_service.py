from datetime import datetime
from typing import Any, Dict, Optional
from uuid import NAMESPACE_URL, uuid5

from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.application.results import ServiceResult
from bot.sql_helper.sql_accounts import Account, AccountIdentity, AccountWallet
from bot.sql_helper.sql_application import utcnow


class UserService:
    """Canonical user mutations shared by all entry points."""

    MUTABLE_FIELDS = {
        "embyid",
        "name",
        "pwd",
        "pwd2",
        "lv",
        "cr",
        "ex",
        "us",
        "iv",
        "ch",
    }

    def __init__(self, uow_factory=SqlAlchemyUnitOfWork):
        self._uow_factory = uow_factory

    def ensure_user(self, tg: int, actor: Optional[Actor] = None) -> ServiceResult:
        actor = actor or Actor.system("user-service")
        with self._uow_factory() as uow:
            user, created = uow.users.add_if_missing(tg)
            account = uow.accounts.by_legacy_tg(tg, for_update=True)
            account_created = account is None
            if account is None:
                now = utcnow()
                account_id = str(uuid5(NAMESPACE_URL, f"sakura-account:{int(tg)}"))
                account = Account(
                    id=account_id,
                    legacy_tg=int(tg),
                    display_name=f"TG {tg}",
                    status="active",
                    created_at=now,
                    updated_at=now,
                )
                uow.accounts.add_account(account)
                uow.accounts.add_identity(
                    AccountIdentity(
                        id=str(uuid5(NAMESPACE_URL, f"sakura-identity:telegram:{int(tg)}")),
                        account_id=account_id,
                        provider="telegram",
                        subject=str(tg),
                        verified_at=now,
                        created_at=now,
                        updated_at=now,
                    )
                )
                for balance_type, value in (("coins", user.iv), ("registration_days", user.us)):
                    uow.accounts.add_wallet(
                        AccountWallet(
                            account_id=account_id,
                            balance_type=balance_type,
                            balance=int(value or 0),
                            revision=1,
                            updated_at=now,
                        )
                    )
            if not created and not account_created:
                return ServiceResult(
                    "ok",
                    {"tg": tg, "account_id": account.id, "created": False},
                )
            uow.operations.audit(
                actor=actor,
                action="user.create" if created else "account.canonical.backfill",
                resource_type="user",
                resource_id=str(tg),
                detail={"source": actor.kind, "account_id": account.id},
            )
            if created:
                uow.operations.event(
                    "user.created",
                    "user",
                    str(tg),
                    {"tg": tg, "account_id": account.id},
                )
            else:
                uow.operations.event(
                    "account.created",
                    "account",
                    account.id,
                    {"tg": tg, "account_id": account.id, "source": "legacy-backfill"},
                )
            return ServiceResult(
                "ok",
                {"tg": tg, "account_id": account.id, "created": created},
            )

    def update_user(
        self,
        tg: int,
        changes: Dict[str, Any],
        actor: Actor,
        *,
        action: str = "user.update",
        idempotency_key: Optional[str] = None,
    ) -> ServiceResult:
        invalid_fields = set(changes) - self.MUTABLE_FIELDS
        if invalid_fields:
            raise ValueError(f"Unsupported user fields: {sorted(invalid_fields)}")

        scope = action
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("user_not_found")

            replay = uow.operations.get_idempotent_result(scope, idempotency_key)
            if replay:
                return ServiceResult.from_dict(replay)

            changed_fields = []
            for key, value in changes.items():
                if getattr(user, key) != value:
                    setattr(user, key, value)
                    changed_fields.append(key)

            result = ServiceResult("ok", {"tg": tg, "changed_fields": changed_fields})
            uow.operations.audit(
                actor=actor,
                action=action,
                resource_type="user",
                resource_id=str(tg),
                detail={"changed_fields": changed_fields},
            )
            if changed_fields:
                uow.operations.event(
                    "user.updated",
                    "user",
                    str(tg),
                    {"tg": tg, "changed_fields": changed_fields},
                )
            uow.operations.save_idempotent_result(scope, idempotency_key, result.to_dict())
            return result

    def complete_registration(
        self,
        *,
        tg: int,
        embyid: str,
        name: str,
        pwd: str,
        pwd2: str,
        expires_at: datetime,
        consume_qualification: bool,
        actor: Actor,
    ) -> ServiceResult:
        with self._uow_factory() as uow:
            user = uow.users.get_for_update(tg)
            if not user:
                return ServiceResult("user_not_found")
            if user.embyid:
                return ServiceResult("account_already_bound")
            if consume_qualification and int(user.us or 0) <= 0:
                return ServiceResult("no_qualification")

            user.embyid = embyid
            user.name = name
            user.pwd = pwd
            user.pwd2 = pwd2
            user.lv = "b"
            user.cr = datetime.now()
            user.ex = expires_at
            if consume_qualification:
                consumed_days = int(user.us or 0)
                user.us = 0
                if consumed_days:
                    uow.operations.point_transaction(
                        tg=tg,
                        balance_type="registration_days",
                        amount=-consumed_days,
                        balance_after=0,
                        reason="registration_qualification_consumed",
                        actor=actor,
                        idempotency_key=f"registration-consume:{tg}:{embyid}",
                        metadata={"embyid": embyid},
                    )

            uow.operations.audit(
                actor=actor,
                action="user.registration.complete",
                resource_type="user",
                resource_id=str(tg),
                detail={"embyid": embyid, "name": name},
            )
            uow.operations.event(
                "user.updated",
                "user",
                str(tg),
                {"tg": tg, "changed_fields": ["embyid", "name", "lv", "cr", "ex", "us"]},
            )
            return ServiceResult("ok", {"tg": tg, "embyid": embyid})
