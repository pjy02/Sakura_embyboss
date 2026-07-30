from datetime import datetime, timedelta
from typing import Optional

from sqlalchemy.orm import Session

from bot.sql_helper.sql_partition import PartitionCode, PartitionGrant


class PartitionRepository:
    def __init__(self, session: Session):
        self.session = session

    def get_code_for_update(self, code: str) -> Optional[PartitionCode]:
        return (
            self.session.query(PartitionCode)
            .filter(PartitionCode.code == code)
            .with_for_update()
            .first()
        )

    def get_reservation_for_update(self, reservation_token: str) -> Optional[PartitionCode]:
        return (
            self.session.query(PartitionCode)
            .filter(PartitionCode.reservation_token == reservation_token)
            .with_for_update()
            .first()
        )

    def get_grant_for_update(self, tg: int, partition: str) -> Optional[PartitionGrant]:
        return (
            self.session.query(PartitionGrant)
            .filter(PartitionGrant.tg == tg, PartitionGrant.partition == partition)
            .with_for_update()
            .first()
        )

    def complete_grant(
        self,
        *,
        record: PartitionCode,
        tg: int,
        embyid: str,
        embyname: Optional[str],
        now: datetime,
    ) -> datetime:
        grant = self.get_grant_for_update(tg, record.partition)
        start_from = grant.expires_at if grant and grant.expires_at > now else now
        expires_at = start_from + timedelta(days=record.duration_days)
        if grant:
            grant.embyid = embyid
            grant.embyname = embyname or grant.embyname
            grant.expires_at = expires_at
            grant.status = "active"
            grant.code = record.code
            grant.updated_at = now
        else:
            grant = PartitionGrant(
                tg=tg,
                embyid=embyid,
                embyname=embyname,
                partition=record.partition,
                expires_at=expires_at,
                status="active",
                code=record.code,
                created_at=now,
                updated_at=now,
            )
            self.session.add(grant)
        self.session.delete(record)
        return expires_at
