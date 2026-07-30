from sqlalchemy.orm import Session
from sqlalchemy.dialects.mysql import insert as mysql_insert

from bot.sql_helper.sql_emby import Emby


class UserRepository:
    def __init__(self, session: Session):
        self.session = session

    def get(self, tg: int):
        return self.session.query(Emby).filter(Emby.tg == tg).first()

    def get_for_update(self, tg: int):
        return (
            self.session.query(Emby)
            .filter(Emby.tg == tg)
            .with_for_update()
            .first()
        )

    def add_if_missing(self, tg: int):
        row = self.get_for_update(tg)
        if row is not None:
            return row, False

        if self.session.bind and self.session.bind.dialect.name in {"mysql", "mariadb"}:
            result = self.session.execute(
                mysql_insert(Emby)
                .values(tg=tg)
                .prefix_with("IGNORE")
            )
            self.session.flush()
            return self.get_for_update(tg), result.rowcount == 1

        row = Emby(tg=tg)
        self.session.add(row)
        self.session.flush()
        return row, True
