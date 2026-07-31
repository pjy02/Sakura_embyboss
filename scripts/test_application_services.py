#!/usr/bin/env python3
"""Fast database tests for the shared business services.

The test suite uses SQLite and does not need Telegram, Emby or MySQL.
"""

import os
import asyncio
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
bot_stub.emby_line = "https://legacy.example.com"
bot_stub.emby_whitelist_line = "https://legacy-vip.example.com"
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
from bot.sql_helper.sql_application import AuditLog, PointTransaction, SystemEvent
from bot.sql_helper.sql_code import Code
from bot.sql_helper.sql_commerce import (
    BillingEntry,
    MediaRequest,
    RechargeProduct,
    SupportTicket,
    TicketMessage,
)
from bot.sql_helper.sql_emby import Emby
from bot.sql_helper.sql_partition import PartitionCode, PartitionGrant
from bot.application import (
    CodeService,
    CommerceService,
    CoreOperationsService,
    MediaRequestService,
    PartitionService,
    PointService,
    TicketService,
    TokenCodec,
    UserService,
    WebAuthService,
)
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper.sql_operations import KnownDevice, LineEndpoint, PlaybackSession


class StubEmbyResult:
    def __init__(self, data=None, success=True, error=None):
        self.data = data
        self.success = success
        self.error = error


class StubEmbyClient:
    def __init__(self):
        self.sessions = []
        self.stopped = []

    async def _request(self, _method, _endpoint):
        return StubEmbyResult(self.sessions)

    async def terminate_session(self, session_id, _reason):
        self.stopped.append(session_id)
        return True


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
        self.commerce = CommerceService(self.uow_factory)
        self.tickets = TicketService(self.uow_factory)
        self.media_requests = MediaRequestService(self.uow_factory)
        self.partitions = PartitionService(self.uow_factory)
        self.emby_client = StubEmbyClient()
        self.core_operations = CoreOperationsService(
            self.uow_factory,
            emby_client=self.emby_client,
        )
        self.auth = WebAuthService(
            token_codec=TokenCodec("test-web-session-secret-long-enough"),
            owner_tg=9001,
            admin_tg_ids=[9002],
            uow_factory=self.uow_factory,
        )

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

    def test_live_playback_builds_history_and_device_profile(self):
        self._add_user(embyid="emby-1", name="tester")
        self.emby_client.sessions = [
            {
                "Id": "session-1",
                "UserId": "emby-1",
                "UserName": "tester",
                "Client": "Sakura Player",
                "DeviceId": "device-1",
                "DeviceName": "Living room",
                "RemoteEndPoint": "203.0.113.8",
                "NowPlayingItem": {
                    "Id": "movie-1",
                    "Name": "Test Movie",
                    "Type": "Movie",
                    "RunTimeTicks": 10_000_000_000,
                },
                "PlayState": {"PositionTicks": 2_500_000_000},
            }
        ]

        result = asyncio.run(self.core_operations.sync_live_sessions())
        self.assertEqual(result["total"], 1)
        self.assertEqual(result["items"][0]["tg"], 1001)
        self.assertEqual(result["items"][0]["progress_percent"], 25.0)
        with self.session_factory() as session:
            self.assertEqual(session.query(PlaybackSession).count(), 1)
            device = session.get(KnownDevice, "device-1")
            self.assertEqual(device.emby_user_name, "tester")

        self.emby_client.sessions = []
        asyncio.run(self.core_operations.sync_live_sessions())
        with self.session_factory() as session:
            self.assertIsNotNone(session.query(PlaybackSession).one().ended_at)

    def test_line_lifecycle_and_device_decisions_are_audited(self):
        self.assertEqual(
            self.core_operations.public_line_text(),
            "https://legacy.example.com",
        )
        line = self.core_operations.create_line(
            {
                "name": "Primary",
                "base_url": "https://emby.example.com/",
                "region": "HK",
                "carrier": "BGP",
            },
            actor=self.actor,
        )
        self.core_operations.create_line(
            {
                "name": "VIP",
                "base_url": "https://vip.example.com",
                "audience": "whitelist",
            },
            actor=self.actor,
        )
        self.assertEqual(
            self.core_operations.public_line_text(),
            "https://emby.example.com",
        )
        self.assertEqual(
            self.core_operations.public_line_text(include_whitelist=True),
            "https://emby.example.com\nhttps://vip.example.com",
        )
        updated = self.core_operations.update_line(
            line["id"],
            {"revision": line["revision"], "maintenance": True},
            actor=self.actor,
        )
        self.assertTrue(updated["maintenance"])

        with self.session_factory() as session:
            session.add(KnownDevice(device_key="device-risk", device_name="TV"))
            session.commit()
        device = self.core_operations.update_device(
            "device-risk",
            trusted=False,
            banned=True,
            notes="shared credential",
            actor=self.actor,
        )
        self.assertTrue(device["banned"])
        self.assertEqual(device["risk_level"], "high")
        with self.session_factory() as session:
            self.assertEqual(session.query(LineEndpoint).count(), 2)
            self.assertGreaterEqual(session.query(AuditLog).count(), 4)

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

    def test_recharge_order_is_idempotent_and_credits_once(self):
        self._add_user(iv=10, lv="b")
        product = self.commerce.create_product(
            {
                "name": "测试积分包",
                "description": "service test",
                "amount_cents": 1200,
                "coins": 100,
                "bonus_coins": 20,
                "enabled": True,
                "sort_order": 10,
            },
            actor=self.actor,
        )
        first = self.commerce.create_order(
            tg=1001,
            product_id=product["id"],
            user_note="paid by transfer",
            actor=self.actor,
            idempotency_key="recharge-create-1",
        )
        replayed = self.commerce.create_order(
            tg=1001,
            product_id=product["id"],
            user_note="paid by transfer",
            actor=self.actor,
            idempotency_key="recharge-create-1",
        )
        self.assertEqual(first["id"], replayed["id"])

        credited = self.commerce.decide_order(
            first["id"],
            approve=True,
            payment_reference="PAY-1001",
            admin_note="verified",
            actor=Actor.web(9001),
        )
        credited_again = self.commerce.decide_order(
            first["id"],
            approve=True,
            payment_reference="PAY-1001",
            admin_note="verified",
            actor=Actor.web(9001),
        )
        self.assertEqual(credited["status"], "credited")
        self.assertEqual(credited_again["status"], "credited")
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).iv, 130)
            recharge_transactions = (
                session.query(PointTransaction)
                .filter(PointTransaction.reason.like("recharge:%"))
                .count()
            )
            self.assertEqual(recharge_transactions, 1)
            self.assertEqual(session.query(BillingEntry).count(), 2)

    def test_user_can_cancel_only_their_pending_recharge_order(self):
        self._add_user(iv=0)
        with self.session_factory() as session:
            session.add(
                RechargeProduct(
                    name="取消测试",
                    amount_cents=500,
                    coins=50,
                    enabled=True,
                )
            )
            session.commit()
            product_id = session.query(RechargeProduct.id).scalar()
        order = self.commerce.create_order(
            tg=1001,
            product_id=product_id,
            user_note=None,
            actor=self.actor,
            idempotency_key="recharge-cancel-1",
        )
        self.assertIsNone(
            self.commerce.cancel_order(
                order["id"],
                tg=1002,
                actor=Actor.web(1002),
            )
        )
        canceled = self.commerce.cancel_order(order["id"], tg=1001, actor=Actor.web(1001))
        self.assertEqual(canceled["status"], "canceled")
        ledger = self.commerce.ledger(tg=1001)
        self.assertEqual([item["entry_type"] for item in ledger["items"]], ["order_canceled", "order_created"])

    def test_ticket_messages_are_scoped_and_internal_notes_are_hidden(self):
        self._add_user(tg=1001, name="alice")
        self._add_user(tg=1002, name="bob")
        ticket = self.tickets.create(
            tg=1001,
            subject="播放设备无法连接",
            category="playback",
            priority="high",
            body="电视端提示连接失败",
            actor=Actor.web(1001),
        )
        self.assertIsNone(self.tickets.detail(ticket["id"], tg=1002))
        self.tickets.reply(
            ticket["id"],
            body="仅管理员可见的诊断",
            actor=Actor.web(9001),
            tg_scope=None,
            internal=True,
        )
        visible = self.tickets.detail(ticket["id"], tg=1001, include_internal=False)
        admin_detail = self.tickets.detail(ticket["id"], include_internal=True)
        self.assertEqual(len(visible["messages"]), 1)
        self.assertEqual(len(admin_detail["messages"]), 2)
        replied = self.tickets.reply(
            ticket["id"],
            body="请重新登录后测试",
            actor=Actor.web(9001),
            tg_scope=None,
        )
        self.assertEqual(replied["status"], "pending_user")
        user_reply = self.tickets.reply(
            ticket["id"],
            body="已经恢复",
            actor=Actor.web(1001),
            tg_scope=1001,
        )
        self.assertEqual(user_reply["status"], "pending_staff")
        with self.session_factory() as session:
            self.assertEqual(session.query(SupportTicket).count(), 1)
            self.assertEqual(session.query(TicketMessage).count(), 4)

    def test_media_requests_are_scoped_and_bot_downloads_sync(self):
        request = self.media_requests.create(
            tg=1001,
            title="测试电影",
            year=2026,
            media_type="movie",
            description="需要中文字幕",
            actor=Actor.web(1001),
        )
        self.assertEqual(self.media_requests.list(tg=1002)["total"], 0)
        canceled = self.media_requests.cancel(request["id"], tg=1001, actor=Actor.web(1001))
        self.assertEqual(canceled["status"], "canceled")

        imported = self.media_requests.import_download(
            tg=1001,
            download_id="mp-download-1",
            title="Bot 点播电影",
            description="legacy request",
            cost_coins=12,
            actor=self.actor,
        )
        imported_again = self.media_requests.import_download(
            tg=1001,
            download_id="mp-download-1",
            title="Bot 点播电影",
            description="legacy request",
            cost_coins=12,
            actor=self.actor,
        )
        self.assertEqual(imported["id"], imported_again["id"])
        self.assertEqual(len(self.media_requests.transfer_candidates()), 1)
        downloading = self.media_requests.sync_download(
            "mp-download-1",
            download_state="downloading",
            transfer_state=None,
            progress=42.7,
        )
        completed = self.media_requests.sync_download(
            "mp-download-1",
            download_state="completed",
            transfer_state=True,
            progress=87,
        )
        self.assertEqual(downloading["progress"], 42)
        self.assertEqual(completed["status"], "completed")
        self.assertEqual(completed["progress"], 100)
        self.assertEqual(self.media_requests.transfer_candidates(), [])
        with self.session_factory() as session:
            self.assertEqual(session.query(MediaRequest).count(), 2)
            request_events = (
                session.query(SystemEvent)
                .filter(SystemEvent.event_type.like("request.%"))
                .all()
            )
            self.assertTrue(request_events)
            self.assertTrue(
                all(
                    event.aggregate_type == "user"
                    and event.aggregate_id == "1001"
                    for event in request_events
                )
            )

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

    def test_telegram_login_is_confirmed_before_session_creation(self):
        self._add_user(embyid="emby-1", name="alice", lv="b")
        started = self.auth.create_telegram_login(ip_address="127.0.0.1")
        raw_token = started.data["request_token"]

        before_approval = self.auth.exchange_telegram_login(
            raw_token=raw_token,
            user_agent="test",
            ip_address="127.0.0.1",
        )
        self.assertEqual(before_approval.status, "not_approved")

        claimed = self.auth.claim_telegram_login(
            raw_token=raw_token,
            tg=1001,
            display_name="alice",
        )
        self.assertTrue(claimed.ok)
        approved = self.auth.decide_telegram_login(
            request_id=claimed.data["request_id"],
            tg=1001,
            approve=True,
        )
        self.assertTrue(approved.ok)

        exchanged = self.auth.exchange_telegram_login(
            raw_token=raw_token,
            user_agent="test",
            ip_address="127.0.0.1",
        )
        self.assertTrue(exchanged.ok)
        identity = self.auth.authenticate(exchanged.data["session_token"])
        self.assertEqual(identity.tg, 1001)
        self.assertEqual(identity.auth_method, "telegram")
        self.assertTrue(
            self.auth.verify_csrf(identity, exchanged.data["csrf_token"])
        )

    def test_telegram_login_rejects_a_different_requested_identity(self):
        self._add_user(tg=1001, embyid="emby-1", name="alice", lv="b")
        self._add_user(tg=1002, embyid="emby-2", name="bob", lv="b")
        started = self.auth.create_telegram_login(
            ip_address="127.0.0.1",
            requested_tg=1001,
        )
        claimed = self.auth.claim_telegram_login(
            raw_token=started.data["request_token"],
            tg=1002,
        )
        self.assertEqual(claimed.status, "identity_mismatch")

    def test_owner_permissions_and_emby_auth_method_are_distinct(self):
        self._add_user(tg=9001, embyid="owner-emby", name="owner", lv="a")
        result = self.auth.create_emby_session(
            embyid="owner-emby",
            username="owner",
            user_agent="test",
            ip_address="127.0.0.1",
        )
        identity = self.auth.authenticate(result.data["session_token"])
        self.assertEqual(identity.auth_method, "emby")
        self.assertTrue(identity.has_permission("users:read"))
        self.assertTrue(identity.is_owner)


if __name__ == "__main__":
    unittest.main(verbosity=2)
