from sqlalchemy import func, or_
from sqlalchemy.orm import Session

from bot.sql_helper.sql_accounts import (
    Account,
    AccountIdentity,
    AccountLedgerEntry,
    AccountMembership,
    AccountTag,
    AccountTagAssignment,
    AccountWallet,
    MembershipPlan,
)


class AccountRepository:
    def __init__(self, session: Session):
        self.session = session

    def get(self, account_id: str, *, for_update: bool = False):
        query = self.session.query(Account).filter(Account.id == account_id)
        return query.with_for_update().first() if for_update else query.first()

    def by_legacy_tg(self, tg: int, *, for_update: bool = False):
        query = self.session.query(Account).filter(Account.legacy_tg == tg)
        return query.with_for_update().first() if for_update else query.first()

    def identity(self, provider: str, subject: str, *, for_update: bool = False):
        query = self.session.query(AccountIdentity).filter(
            AccountIdentity.provider == provider,
            AccountIdentity.subject == subject,
        )
        return query.with_for_update().first() if for_update else query.first()

    def local_identity(self, username_normalized: str, *, for_update: bool = False):
        query = self.session.query(AccountIdentity).filter(
            AccountIdentity.provider == "local",
            AccountIdentity.username_normalized == username_normalized,
        )
        return query.with_for_update().first() if for_update else query.first()

    def identities(self, account_id: str):
        return self.session.query(AccountIdentity).filter(AccountIdentity.account_id == account_id).all()

    def add_account(self, row: Account) -> None:
        self.session.add(row)

    def add_identity(self, row: AccountIdentity) -> None:
        self.session.add(row)

    def next_local_legacy_key(self) -> int:
        current = self.session.query(func.min(Account.legacy_tg)).scalar()
        return min(-1, int(current or 0) - 1)

    def default_plan(self):
        return (
            self.session.query(MembershipPlan)
            .filter(MembershipPlan.enabled.is_(True), MembershipPlan.is_default.is_(True))
            .order_by(MembershipPlan.sort_order.asc())
            .first()
        )

    def get_plan(self, plan_id: int):
        return self.session.get(MembershipPlan, plan_id)

    def list_plans(self, *, enabled_only: bool = False):
        query = self.session.query(MembershipPlan)
        if enabled_only:
            query = query.filter(MembershipPlan.enabled.is_(True))
        return query.order_by(MembershipPlan.sort_order.asc(), MembershipPlan.id.asc()).all()

    def add_plan(self, row: MembershipPlan) -> None:
        self.session.add(row)

    def active_membership(self, account_id: str, *, for_update: bool = False):
        query = (
            self.session.query(AccountMembership)
            .filter(AccountMembership.account_id == account_id, AccountMembership.status.in_(("active", "suspended")))
            .order_by(AccountMembership.created_at.desc())
        )
        return query.with_for_update().first() if for_update else query.first()

    def add_membership(self, row: AccountMembership) -> None:
        self.session.add(row)

    def wallet(self, account_id: str, balance_type: str, *, for_update: bool = False):
        query = self.session.query(AccountWallet).filter(
            AccountWallet.account_id == account_id,
            AccountWallet.balance_type == balance_type,
        )
        return query.with_for_update().first() if for_update else query.first()

    def add_wallet(self, row: AccountWallet) -> None:
        self.session.add(row)

    def add_ledger(self, row: AccountLedgerEntry) -> None:
        self.session.add(row)

    def list_ledger(self, account_id: str, *, limit: int = 100, offset: int = 0):
        return (
            self.session.query(AccountLedgerEntry)
            .filter(AccountLedgerEntry.account_id == account_id)
            .order_by(AccountLedgerEntry.created_at.desc(), AccountLedgerEntry.id.desc())
            .offset(offset)
            .limit(limit)
            .all()
        )

    def list_tags(self):
        return self.session.query(AccountTag).order_by(AccountTag.name.asc()).all()

    def get_tag(self, tag_id: int):
        return self.session.get(AccountTag, tag_id)

    def get_tag_by_name(self, name: str):
        return self.session.query(AccountTag).filter(func.lower(AccountTag.name) == name.lower()).first()

    def add_tag(self, row: AccountTag) -> None:
        self.session.add(row)

    def delete_tag(self, row: AccountTag) -> None:
        self.session.query(AccountTagAssignment).filter(AccountTagAssignment.tag_id == row.id).delete(synchronize_session=False)
        self.session.delete(row)

    def assignment(self, account_id: str, tag_id: int):
        return self.session.query(AccountTagAssignment).filter(
            AccountTagAssignment.account_id == account_id,
            AccountTagAssignment.tag_id == tag_id,
        ).first()

    def add_assignment(self, row: AccountTagAssignment) -> None:
        self.session.add(row)

    def remove_assignment(self, account_id: str, tag_id: int) -> int:
        return self.session.query(AccountTagAssignment).filter(
            AccountTagAssignment.account_id == account_id,
            AccountTagAssignment.tag_id == tag_id,
        ).delete(synchronize_session=False)

    def tags_for_account(self, account_id: str):
        return (
            self.session.query(AccountTag)
            .join(AccountTagAssignment, AccountTagAssignment.tag_id == AccountTag.id)
            .filter(AccountTagAssignment.account_id == account_id)
            .order_by(AccountTag.name.asc())
            .all()
        )

    def list_accounts(self, *, search=None, status=None, tag_id=None, limit=50, offset=0):
        query = self.session.query(Account)
        if search:
            pattern = f"%{search.strip()}%"
            query = query.outerjoin(AccountIdentity, AccountIdentity.account_id == Account.id).filter(
                or_(
                    Account.display_name.like(pattern),
                    AccountIdentity.username.like(pattern),
                    AccountIdentity.subject.like(pattern),
                )
            ).distinct()
        if status:
            query = query.filter(Account.status == status)
        if tag_id:
            query = query.join(AccountTagAssignment, AccountTagAssignment.account_id == Account.id).filter(AccountTagAssignment.tag_id == tag_id)
        total = query.count()
        rows = query.order_by(Account.created_at.desc()).offset(offset).limit(limit).all()
        return rows, total
