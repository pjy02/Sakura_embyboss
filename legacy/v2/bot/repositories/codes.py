from datetime import datetime

from sqlalchemy.orm import Session

from bot.sql_helper.sql_code import Code
from sqlalchemy import and_, or_


class CodeRepository:
    def __init__(self, session: Session):
        self.session = session

    def get_for_update(self, code: str):
        return (
            self.session.query(Code)
            .filter(Code.code == code)
            .with_for_update()
            .first()
        )

    def add_many(self, codes: list[str], tg: int, days: int) -> None:
        self.session.add_all([Code(code=value, tg=tg, us=days) for value in codes])

    def add_rows(self, rows: list[Code]) -> None:
        self.session.add_all(rows)

    def list(self, *, kind=None, status=None, search=None, limit=50, offset=0):
        query = self.session.query(Code)
        if kind:
            keyword = {"registration": "Register", "renewal": "Renew", "whitelist": "Whitelist"}.get(kind, kind)
            query = query.filter(Code.code.contains(keyword))
        if status == "used":
            query = query.filter(Code.used.isnot(None))
        elif status == "unused":
            query = query.filter(
                Code.used.is_(None),
                or_(Code.status.is_(None), Code.status == "active"),
                or_(Code.expires_at.is_(None), Code.expires_at > datetime.now()),
            )
        elif status == "expired":
            query = query.filter(
                Code.used.is_(None),
                or_(
                    Code.status == "expired",
                    and_(Code.expires_at.isnot(None), Code.expires_at <= datetime.now()),
                ),
            )
        elif status:
            query = query.filter(Code.status == status)
        if search:
            query = query.filter(Code.code.like(f"%{search.strip()}%"))
        total = query.count()
        rows = query.order_by(Code.usedtime.desc(), Code.code.asc()).offset(offset).limit(limit).all()
        return rows, total
