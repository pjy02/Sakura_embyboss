from sqlalchemy.orm import Session

from bot.sql_helper.sql_code import Code


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
