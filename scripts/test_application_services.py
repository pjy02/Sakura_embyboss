#!/usr/bin/env python3
"""Fast database tests for the shared business services.

The test suite uses SQLite and does not need Telegram, Emby or MySQL.
"""

import os
import sys
import types
import unittest
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BOT_DIR = ROOT / "bot"
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

os.environ.setdefault("SAKURA_RUNNING_MIGRATIONS", "1")

bot_stub = types.ModuleType("bot")
bot_stub.__path__ = [str(BOT_DIR)]
bot_stub.db_host = "localhost"
bot_stub.db_user = "unused"
bot_stub.db_pwd = "unused"
bot_stub.db_name = "unused"
bot_stub.db_port = 3306
bot_stub.LOGGER = types.SimpleNamespace(
    debug=lambda *args, **kwargs: None,
    info=lambda *args, **kwargs: None,
    warning=lambda *args, **kwargs: None,
    error=lambda *args, **kwargs: None,
)
sys.modules["bot"] = bot_stub

from sqlalchemy import BigInteger, create_engine
from sqlalchemy.ext.compiler import compiles
from sqlalchemy.orm import sessionmaker


@compiles(BigInteger, "sqlite")
def _compile_big_integer_as_integer(_type, _compiler, **_kwargs):
    return "INTEGER"


from bot.sql_helper import Base
from bot.sql_helper.sql_application import AuditLog, PointTransaction
from bot.sql_helper.sql_code import Code
from bot.sql_helper.sql_emby import Emby
from bot.sql_helper.sql_partition import PartitionCode, PartitionGrant
from bot.application import CodeService, PartitionService, PointService, UserService
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork


class ApplicationServiceTests(unittest.TestCase):
    def setUp(self):
        self.engine = create_engine("sqlite:///:memory:")
        Base.metadata.create_all(self.engine)
        self.session_factory = sessionmaker(
            bind=self.engine,
            autoflush=False,
            expire_on_commit=False,
        )
        self.uow_factory = lambda: SqlAlchemyUnitOfWork(self.session_factory)
        self.actor = Actor.telegram(1001, "tester")
        self.users = UserService(self.uow_factory)
        self.points = PointService(self.uow_factory)
        self.codes = CodeService(self.uow_factory)
        self.partitions = PartitionService(self.uow_factory)

    def tearDown(self):
        Base.metadata.drop_all(self.engine)
        self.engine.dispose()

    def _add_user(self, tg=1001, **values):
        with self.session_factory() as session:
            session.add(Emby(tg=tg, **values))
            session.commit()

    def test_ensure_user_is_idempotent_and_audited(self):
        first = self.users.ensure_user(1001, self.actor)
        second = self.users.ensure_user(1001, self.actor)

        self.assertTrue(first.ok)
        self.assertTrue(first.data["created"])
        self.assertFalse(second.data["created"])
        with self.session_factory() as session:
            self.assertEqual(session.query(Emby).count(), 1)
            self.assertEqual(session.query(AuditLog).count(), 1)

    def test_point_adjustment_has_ledger_and_idempotency(self):
        self._add_user(iv=10)
        first = self.points.adjust(
            tg=1001,
            amount=5,
            balance_type="coins",
            reason="test",
            actor=self.actor,
            idempotency_key="point-1",
        )
        second = self.points.adjust(
            tg=1001,
            amount=5,
            balance_type="coins",
            reason="test",
            actor=self.actor,
            idempotency_key="point-1",
        )

        self.assertEqual(first.data["balance"], 15)
        self.assertTrue(second.replayed)
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).iv, 15)
            self.assertEqual(session.query(PointTransaction).count(), 1)

    def test_check_in_cannot_credit_twice(self):
        self._add_user(iv=0, lv="b")
        now = datetime(2026, 7, 30, 9, 0, 0)
        first = self.points.check_in(
            tg=1001,
            reward=8,
            occurred_at=now,
            actor=self.actor,
            maximum_level="d",
            idempotency_key="checkin-20260730",
        )
        second = self.points.check_in(
            tg=1001,
            reward=10,
            occurred_at=now,
            actor=self.actor,
            maximum_level="d",
            idempotency_key="checkin-20260730-second-request",
        )

        self.assertTrue(first.ok)
        self.assertEqual(second.status, "already_checked_in")
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).iv, 8)

    def test_registration_code_redemption_is_atomic(self):
        self._add_user(us=0, lv="d")
        with self.session_factory() as session:
            session.add(Code(code="SAKURA-30-Register_test", tg=9001, us=30))
            session.commit()

        result = self.codes.redeem_registration(
            code_value="SAKURA-30-Register_test",
            tg=1001,
            logo="SAKURA",
            actor=self.actor,
            idempotency_key="redeem-code-1",
        )

        self.assertTrue(result.ok)
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).us, 30)
            code = session.get(Code, "SAKURA-30-Register_test")
            self.assertEqual(code.used, 1001)

    def test_failed_purchase_does_not_create_codes(self):
        self._add_user(iv=2, lv="b")
        result = self.codes.purchase_registration_codes(
            tg=1001,
            codes=["SAKURA-30-Register_new"],
            days=30,
            cost=100,
            maximum_level="d",
            actor=self.actor,
        )

        self.assertEqual(result.status, "insufficient_balance")
        with self.session_factory() as session:
            self.assertEqual(session.query(Code).count(), 0)
            self.assertEqual(session.get(Emby, 1001).iv, 2)

    def test_partition_code_is_only_consumed_after_completion(self):
        self._add_user(embyid="emby-1", name="alice", lv="b")
        with self.session_factory() as session:
            session.add(
                PartitionCode(
                    code="PARTITION-1",
                    partition="anime",
                    duration_days=7,
                    status="available",
                )
            )
            session.commit()

        reservation = self.partitions.reserve_code(
            code_value="PARTITION-1",
            tg=1001,
            actor=self.actor,
            now=datetime(2026, 7, 30, 10, 0, 0),
        )
        self.assertTrue(reservation.ok)
        with self.session_factory() as session:
            self.assertIsNotNone(session.get(PartitionCode, "PARTITION-1"))
            self.assertEqual(session.query(PartitionGrant).count(), 0)

        completed = self.partitions.complete_redemption(
            reservation_token=reservation.data["reservation_token"],
            tg=1001,
            actor=self.actor,
            now=datetime(2026, 7, 30, 10, 0, 0),
        )
        self.assertTrue(completed.ok)
        with self.session_factory() as session:
            self.assertIsNone(session.get(PartitionCode, "PARTITION-1"))
            self.assertEqual(session.query(PartitionGrant).count(), 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
