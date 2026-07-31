#!/usr/bin/env python3
"""Fast database tests for the shared business services.

The test suite uses SQLite and does not need Telegram, Emby or MySQL.
"""

import os
import asyncio
import json
import sys
import types
import unittest
from datetime import datetime, timedelta
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
bot_stub._open = types.SimpleNamespace(
    stat=True,
    open_us=30,
    all_user=100,
    register_queue_limit=10,
)
bot_stub.ranks = types.SimpleNamespace(logo="SAKURA")
bot_stub.owner = 9001
bot_stub.admins = [9002]
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
from bot.sql_helper.sql_application import (
    AccountLifecycleEvent,
    AlertDelivery,
    AuditLog,
    ConfigRevision,
    DynamicSetting,
    OperationTask,
    PointTransaction,
    RiskRule,
    SecurityEvent,
    ServiceProbe,
    SystemEvent,
    WebRole,
)
from bot.sql_helper.sql_accounts import (
    Account,
    AccountIdentity,
    AccountLedgerEntry,
    AccountMembership,
    AccountTag,
    AccountWallet,
    MembershipPlan,
)
from bot.sql_helper.sql_code import Code
from bot.sql_helper.sql_commerce import (
    BillingEntry,
    MediaRequest,
    RechargeProduct,
    SupportTicket,
    TicketMessage,
)
from bot.sql_helper.sql_community import (
    MediaReview,
    ReviewReaction,
    ReviewReport,
    UserNotification,
)
from bot.sql_helper.sql_emby import Emby
from bot.sql_helper.sql_partition import PartitionCode, PartitionGrant
from bot.application import (
    AccountService,
    AccountLifecycleService,
    AdminQueryService,
    CodeService,
    CommerceService,
    CoreOperationsService,
    DynamicSettingsService,
    DiagnosticService,
    MediaRequestService,
    NotificationService,
    PartitionService,
    PointService,
    RegistrationService,
    RiskAutomationService,
    RiskRuleService,
    TicketService,
    TokenCodec,
    UserService,
    WebAuthService,
    ReviewService,
    RiskEventService,
)
from bot.domain import Actor
from bot.application.auth_service import DEFAULT_ROLE_PERMISSIONS
from bot.application.governance_service import SettingConflictError
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


class StubRegistrationEmbyClient:
    def __init__(self):
        self.created = []
        self.deleted = []

    async def emby_create(self, name, days):
        self.created.append((name, days))
        return f"emby-{name}", "generated-password", datetime(2026, 8, 30, 12, 0, 0)

    async def emby_del(self, emby_id):
        self.deleted.append(emby_id)
        return True


class StubLifecycleEmbyClient:
    def __init__(self):
        self.policy_changes = []
        self.deleted = []

    async def emby_change_policy(self, emby_id, admin=False, disable=False):
        self.policy_changes.append((emby_id, disable))
        return True

    async def emby_del(self, emby_id):
        self.deleted.append(emby_id)
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
        self.accounts = AccountService(self.uow_factory)
        self.admin_queries = AdminQueryService(self.uow_factory)
        self.users = UserService(self.uow_factory)
        self.points = PointService(self.uow_factory)
        self.codes = CodeService(self.uow_factory)
        self.commerce = CommerceService(self.uow_factory)
        self.tickets = TicketService(self.uow_factory)
        self.media_requests = MediaRequestService(self.uow_factory)
        self.notifications = NotificationService(self.uow_factory)
        self.reviews = ReviewService(self.uow_factory)
        self.risk_events = RiskEventService(self.uow_factory)
        self.runtime_settings = {}
        self.dynamic_settings = DynamicSettingsService(
            self.uow_factory,
            runtime_values=self.runtime_settings,
        )
        self.partitions = PartitionService(self.uow_factory)
        self.emby_client = StubEmbyClient()
        self.core_operations = CoreOperationsService(
            self.uow_factory,
            emby_client=self.emby_client,
        )
        self.registration_emby = StubRegistrationEmbyClient()
        self.registrations = RegistrationService(
            self.uow_factory,
            emby_client=self.registration_emby,
        )
        self.lifecycle_emby = StubLifecycleEmbyClient()
        self.lifecycle = AccountLifecycleService(
            self.uow_factory,
            emby_client=self.lifecycle_emby,
        )
        self.risk_rules = RiskRuleService(self.uow_factory)
        self.risk_automation = RiskAutomationService(self.uow_factory)
        self.diagnostics = DiagnosticService(self.uow_factory)
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
            self.assertEqual(session.query(Account).count(), 1)
            self.assertEqual(session.query(AccountIdentity).count(), 1)
            self.assertEqual(session.query(AccountWallet).count(), 2)
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

    def test_local_account_identity_membership_tags_and_wallet_ledger(self):
        created = self.accounts.create_local(
            username="web-alice",
            password="a-secure-password",
            display_name="Alice",
            actor=Actor.web(9001),
        )
        self.assertTrue(created.ok)
        account_id = created.data["account_id"]
        legacy_tg = created.data["legacy_tg"]
        authenticated = self.accounts.authenticate_local(
            "WEB-ALICE",
            "a-secure-password",
        )
        self.assertTrue(authenticated.ok)
        self.assertEqual(authenticated.data["account_id"], account_id)

        plan = self.accounts.create_plan(
            {
                "code": "monthly",
                "name": "月度会员",
                "duration_days": 30,
                "legacy_level": "b",
                "entitlements": {"devices": 4},
                "enabled": True,
                "is_default": True,
            },
            Actor.web(9001),
        )
        assigned = self.accounts.assign_plan(
            account_id=account_id,
            plan_id=plan["id"],
            duration_days=45,
            actor=Actor.web(9001),
        )
        self.assertTrue(assigned.ok)

        tag = self.accounts.create_tag(
            name="新用户",
            color="#8b7cf6",
            description="Web 注册用户",
            actor=Actor.web(9001),
        )
        self.assertTrue(tag.ok)
        changed = self.accounts.assign_tags(
            account_ids=[account_id],
            tag_ids=[tag.data["id"]],
            mode="add",
            actor=Actor.web(9001),
        )
        self.assertEqual(changed["changed"], 1)

        credited = self.points.adjust(
            tg=legacy_tg,
            amount=25,
            balance_type="coins",
            reason="welcome-credit",
            actor=Actor.web(9001),
            idempotency_key="local-welcome-credit",
        )
        self.assertTrue(credited.ok)
        with self.session_factory() as session:
            self.assertEqual(session.query(Account).count(), 1)
            self.assertEqual(session.query(AccountIdentity).count(), 1)
            self.assertEqual(session.query(MembershipPlan).count(), 1)
            self.assertEqual(session.query(AccountMembership).count(), 1)
            self.assertEqual(session.query(AccountTag).count(), 1)
            wallet = session.query(AccountWallet).filter_by(
                account_id=account_id,
                balance_type="coins",
            ).one()
            self.assertEqual(wallet.balance, 25)
            ledger = session.query(AccountLedgerEntry).one()
            self.assertEqual(ledger.account_id, account_id)
            self.assertEqual(ledger.balance_after, 25)
            self.assertIsNotNone(ledger.source_transaction_id)

    def test_local_web_auth_session_does_not_require_telegram(self):
        created = self.accounts.create_local(
            username="standalone-user",
            password="standalone-password",
            display_name=None,
            actor=Actor.web(9001),
        )
        session = self.auth.create_local_session(
            username="standalone-user",
            password="standalone-password",
            user_agent="test-browser",
            ip_address="127.0.0.1",
        )
        self.assertTrue(session.ok)
        self.assertEqual(session.data["account_id"], created.data["account_id"])
        self.assertEqual(session.data["auth_method"], "local")

    def test_owner_can_be_bootstrapped_for_local_admin_login_once(self):
        first = self.accounts.bootstrap_owner(
            owner_tg=9001,
            username="local-owner",
            password="owner-local-password",
        )
        second = self.accounts.bootstrap_owner(
            owner_tg=9001,
            username="ignored-owner",
            password="ignored-owner-password",
        )
        self.assertTrue(first.ok)
        self.assertEqual(second.status, "already_configured")
        session = self.auth.create_local_session(
            username="local-owner",
            password="owner-local-password",
            user_agent="admin-browser",
            ip_address="127.0.0.1",
        )
        self.assertTrue(session.ok)
        identity = self.auth.authenticate(session.data["session_token"])
        self.assertIsNotNone(identity)
        self.assertIn("owner", identity.roles)
        self.assertEqual(identity.auth_method, "local")

    def test_admin_generated_invitation_codes_keep_legacy_format(self):
        generated = self.codes.generate(
            kind="registration",
            days=60,
            count=3,
            logo="SAKURA",
            issuer_tg=-1,
            issuer_account_id="account-owner",
            expires_at=datetime.now() + timedelta(days=7),
            actor=Actor.web(9001),
        )
        self.assertTrue(generated.ok)
        self.assertEqual(generated.data["count"], 3)
        self.assertTrue(
            all(item["code"].startswith("SAKURA-60-Register_") for item in generated.data["items"])
        )
        listed = self.codes.list_codes(kind="registration", status="active")
        self.assertEqual(listed["total"], 3)
        revoked = self.codes.revoke(
            code_value=generated.data["items"][0]["code"],
            actor=Actor.web(9001),
        )
        self.assertEqual(revoked.data["status"], "revoked")

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

    def test_shared_registration_queue_is_idempotent_and_completes_account(self):
        self._add_user(us=0, lv="d")
        submitted = self.registrations.submit(
            tg=1001,
            username="alice",
            safety_code="1234",
            registration_code=None,
            actor=self.actor,
            idempotency_key="web-registration-1",
        )
        replay = self.registrations.submit(
            tg=1001,
            username="alice",
            safety_code="1234",
            registration_code=None,
            actor=self.actor,
            idempotency_key="web-registration-1",
        )
        duplicate = self.registrations.submit(
            tg=1001,
            username="alice",
            safety_code="1234",
            registration_code=None,
            actor=self.actor,
            idempotency_key="web-registration-2",
        )

        self.assertTrue(submitted.ok)
        self.assertEqual(replay.data["id"], submitted.data["id"])
        self.assertEqual(duplicate.status, "duplicate")
        self.assertNotIn("safety_code", submitted.data["input"])

        with self.session_factory() as session:
            row = session.get(OperationTask, submitted.data["id"])
            payload = json.loads(row.input_json)
        result = asyncio.run(self.registrations.execute(payload))
        self.assertTrue(result["ok"])
        self.assertEqual(result["emby_password"], "generated-password")
        with self.session_factory() as session:
            user = session.get(Emby, 1001)
            self.assertEqual(user.embyid, "emby-alice")
            self.assertEqual(user.name, "alice")
            self.assertEqual(user.pwd2, "1234")

    def test_registration_code_can_enter_shared_queue_when_closed(self):
        self.dynamic_settings.update(
            "registration.enabled",
            value=False,
            expected_revision=0,
            actor=Actor.web(9001),
        )
        self._add_user(us=0, lv="d")
        with self.session_factory() as session:
            session.add(Code(code="SAKURA-30-Register_web", tg=9001, us=30))
            session.commit()
        submitted = self.registrations.submit(
            tg=1001,
            username="invited-user",
            safety_code="5678",
            registration_code="SAKURA-30-Register_web",
            actor=self.actor,
            idempotency_key="web-registration-code-1",
        )
        self.assertTrue(submitted.ok)
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).us, 30)
            self.assertEqual(
                session.get(Code, "SAKURA-30-Register_web").used,
                1001,
            )

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
            notification = session.query(UserNotification).one()
            self.assertEqual(notification.tg, 1001)
            self.assertEqual(notification.category, "billing")
            self.assertEqual(notification.severity, "success")
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

    def test_refund_reverses_credited_order_once_and_reconciles(self):
        self._add_user(iv=25, lv="b")
        product = self.commerce.create_product(
            {
                "name": "退款测试包",
                "description": None,
                "amount_cents": 600,
                "coins": 60,
                "bonus_coins": 5,
                "enabled": True,
                "sort_order": 1,
            },
            actor=self.actor,
        )
        order = self.commerce.create_order(
            tg=1001,
            product_id=product["id"],
            user_note=None,
            actor=self.actor,
            idempotency_key="refund-create-1",
        )
        self.commerce.decide_order(
            order["id"],
            approve=True,
            payment_reference="PAY-REFUND",
            admin_note=None,
            actor=Actor.web(9001),
        )
        refunded = self.commerce.refund_order(
            order["id"],
            reason="用户重复付款",
            actor=Actor.web(9001),
        )
        replay = self.commerce.refund_order(
            order["id"],
            reason="用户重复付款",
            actor=Actor.web(9001),
        )

        self.assertEqual(refunded["status"], "refunded")
        self.assertEqual(replay["status"], "refunded")
        self.assertEqual(self.commerce.reconciliation_summary()["status"], "healthy")
        with self.session_factory() as session:
            self.assertEqual(session.get(Emby, 1001).iv, 25)
            self.assertEqual(session.query(BillingEntry).count(), 3)
            self.assertEqual(session.query(PointTransaction).count(), 2)
            self.assertEqual(session.query(UserNotification).count(), 2)

    def test_risk_rules_trigger_alert_tasks_with_cooldown(self):
        rule = self.risk_rules.create(
            {
                "name": "测试登录风险",
                "event_pattern": "auth.test.failed",
                "severity": "danger",
                "threshold_count": 2,
                "window_minutes": 10,
                "cooldown_minutes": 30,
                "enabled": True,
                "telegram_alert": True,
            },
            self.actor,
        )
        with self.uow_factory() as uow:
            uow.operations.security_event(event_type="auth.test.failed", severity="warning", subject_kind="user", subject_id="1001")
            uow.operations.security_event(event_type="auth.test.failed", severity="warning", subject_kind="user", subject_id="1002")

        first = self.risk_automation.evaluate()
        second = self.risk_automation.evaluate()
        self.assertEqual(first["triggered"][0]["rule_id"], rule["id"])
        self.assertEqual(first["alerts_queued"], 2)
        self.assertEqual(second["triggered"], [])
        with self.session_factory() as session:
            self.assertEqual(session.query(RiskRule).count(), 1)
            self.assertEqual(session.query(AlertDelivery).count(), 2)
            self.assertEqual(
                session.query(OperationTask)
                .filter(OperationTask.task_type == "alert.telegram")
                .count(),
                2,
            )

    def test_diagnostics_record_transitions_without_duplicate_risk_events(self):
        checked_at = datetime.now()
        base = {
            "service_name": "emby",
            "service_kind": "media",
            "latency_ms": 25,
            "status_code": 503,
            "message": "HTTP 503",
            "checked_at": checked_at,
        }
        self.diagnostics._persist([{**base, "status": "unhealthy"}])
        self.diagnostics._persist([{**base, "status": "unhealthy", "checked_at": checked_at + timedelta(minutes=1)}])
        self.diagnostics._persist([{**base, "status": "healthy", "status_code": 200, "message": "HTTP 200", "checked_at": checked_at + timedelta(minutes=2)}])

        with self.session_factory() as session:
            self.assertEqual(session.query(ServiceProbe).count(), 3)
            self.assertEqual(
                session.query(SecurityEvent)
                .filter(SecurityEvent.event_type == "service.probe.failed")
                .count(),
                1,
            )
            self.assertEqual(
                session.query(SystemEvent)
                .filter(SystemEvent.event_type == "service.probe.recovered")
                .count(),
                1,
            )

    def test_batch_lifecycle_records_each_user_and_queues_notifications(self):
        self._add_user(embyid="emby-1001", name="alice", iv=10, lv="b")
        submitted = self.lifecycle.enqueue_batch(
            action="suspend",
            tg_ids=[1001, 9999],
            parameters={},
            actor=Actor.web(9001),
            idempotency_key="batch-suspend-1",
        )
        self.assertTrue(submitted.ok)
        with self.session_factory() as session:
            task = session.get(OperationTask, submitted.data["id"])
            payload = json.loads(task.input_json)
        result = asyncio.run(self.lifecycle.execute_batch(payload))
        replayed = asyncio.run(self.lifecycle.execute_batch(payload))

        self.assertEqual(result["succeeded"], 1)
        self.assertTrue(replayed["items"][0]["replayed"])
        self.assertEqual(self.lifecycle_emby.policy_changes, [("emby-1001", True)])
        with self.session_factory() as session:
            self.assertEqual(session.query(AccountLifecycleEvent).count(), 1)
            self.assertEqual(session.query(UserNotification).count(), 1)
            self.assertEqual(
                session.query(OperationTask)
                .filter(OperationTask.task_type == "notification.telegram")
                .count(),
                1,
            )

    def test_batch_operations_accept_web_only_account_ids(self):
        created = self.accounts.create_local(
            username="batch-web-user",
            password="batch-web-password",
            display_name=None,
            actor=Actor.web(9001),
        )
        account_id = created.data["account_id"]
        submitted = self.lifecycle.enqueue_batch(
            action="grant_coins",
            tg_ids=[],
            account_ids=[account_id],
            parameters={"amount": 40, "reason": "Web 用户批量赠送"},
            actor=Actor.web(9001),
            idempotency_key="batch-web-account-1",
        )
        self.assertTrue(submitted.ok)
        with self.session_factory() as session:
            payload = json.loads(session.get(OperationTask, submitted.data["id"]).input_json)
        result = asyncio.run(self.lifecycle.execute_batch(payload))
        self.assertEqual(result["succeeded"], 1)
        with self.session_factory() as session:
            account = session.get(Account, account_id)
            wallet = session.query(AccountWallet).filter_by(
                account_id=account_id,
                balance_type="coins",
            ).one()
            self.assertEqual(wallet.balance, 40)
            self.assertEqual(session.get(Emby, account.legacy_tg).iv, 40)
            self.assertEqual(session.query(AccountLedgerEntry).count(), 1)
            self.assertEqual(
                session.query(OperationTask)
                .filter(OperationTask.task_type == "notification.telegram")
                .count(),
                0,
            )

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

    def test_review_moderation_reactions_reports_and_notifications(self):
        self._add_user(tg=1001, name="alice")
        self._add_user(tg=1002, name="bob")
        review = self.reviews.create(
            tg=1001,
            media_key="tmdb:550",
            media_title="测试电影",
            media_year=2026,
            rating=9,
            content="这是一篇足够长的测试影评内容。",
            spoiler=True,
            actor=Actor.web(1001),
        )
        with self.assertRaises(RuntimeError):
            self.reviews.create(
                tg=1001,
                media_key="TMDB:550",
                media_title="测试电影",
                media_year=2026,
                rating=8,
                content="不能对同一作品重复提交影评。",
                spoiler=False,
                actor=Actor.web(1001),
            )
        published = self.reviews.moderate(
            review["id"],
            status="published",
            admin_note="内容合规",
            actor=Actor.web(9001),
        )
        self.assertEqual(published["status"], "published")
        liked = self.reviews.react(review["id"], tg=1002, enabled=True)
        self.assertTrue(liked["liked"])
        self.assertEqual(liked["like_count"], 1)
        reported = self.reviews.report(
            review["id"],
            tg=1002,
            reason="spoiler",
            detail="剧透标记需要更醒目",
            actor=Actor.web(1002),
        )
        self.assertEqual(reported["report_count"], 1)
        detail = self.reviews.detail_admin(review["id"])
        self.assertIsNotNone(detail)
        self.assertEqual(len(detail["reports"]), 1)
        self.assertEqual(detail["reports"][0]["reason"], "spoiler")
        with self.assertRaises(RuntimeError):
            self.reviews.report(
                review["id"],
                tg=1002,
                reason="spoiler",
                detail=None,
                actor=Actor.web(1002),
            )
        with self.session_factory() as session:
            self.assertEqual(session.query(MediaReview).count(), 1)
            self.assertEqual(session.query(ReviewReaction).count(), 1)
            self.assertEqual(session.query(ReviewReport).count(), 1)
            notification = session.query(UserNotification).one()
            self.assertEqual(notification.tg, 1001)
            self.assertEqual(notification.category, "review")

    def test_notification_scope_preferences_and_broadcast(self):
        self._add_user(tg=1001, name="alice")
        self._add_user(tg=1002, name="bob")
        result = self.notifications.broadcast(
            target_tg=None,
            category="system",
            title="维护通知",
            body="系统将在今晚进行维护。",
            severity="warning",
            action_url=None,
            actor=Actor.web(9001),
        )
        self.assertEqual(result["created"], 2)
        self.assertEqual(self.notifications.unread_count(1001), 1)
        item = self.notifications.list(tg=1001)["items"][0]
        self.assertIsNone(self.notifications.mark_read(item["id"], tg=1002))
        self.assertIsNotNone(self.notifications.mark_read(item["id"], tg=1001))
        self.assertEqual(self.notifications.unread_count(1001), 0)
        self.notifications.update_preference(
            tg=1001,
            category="system",
            web_enabled=False,
        )
        second = self.notifications.broadcast(
            target_tg=None,
            category="system",
            title="第二条通知",
            body="偏好关闭的用户不应收到。",
            severity="info",
            action_url=None,
            actor=Actor.web(9001),
        )
        self.assertEqual(second["created"], 1)
        self.assertEqual(self.notifications.list(tg=1001)["total"], 1)
        self.assertEqual(self.notifications.list(tg=1002)["total"], 2)
        with self.session_factory() as session:
            self.assertEqual(
                session.query(OperationTask)
                .filter(OperationTask.task_type == "notification.telegram")
                .count(),
                4,
            )

    def test_custom_role_lifecycle_enforces_member_safety(self):
        self._add_user(tg=1001, name="alice")
        created = self.auth.create_role(
            name="content_moderator",
            permissions=["reviews:read", "reviews:update"],
            actor_tg=9001,
        )
        self.assertTrue(created.ok)
        role_id = created.data["id"]
        assigned = self.auth.set_role(
            target_tg=1001,
            role_name="content_moderator",
            enabled=True,
            actor_tg=9001,
        )
        self.assertTrue(assigned.ok)
        blocked = self.auth.delete_role(role_id=role_id, actor_tg=9001)
        self.assertEqual(blocked.status, "role_in_use")
        updated = self.auth.update_role(
            role_id=role_id,
            permissions=["reviews:read"],
            actor_tg=9001,
        )
        self.assertTrue(updated.ok)
        self.auth.set_role(
            target_tg=1001,
            role_name="content_moderator",
            enabled=False,
            actor_tg=9001,
        )
        deleted = self.auth.delete_role(role_id=role_id, actor_tg=9001)
        self.assertTrue(deleted.ok)
        audit = self.admin_queries.audit_logs(
            action="role.create",
            actor_kind="web",
            actor_id="9001",
        )
        self.assertEqual(audit["total"], 1)
        self.assertEqual(audit["items"][0]["resource_type"], "web_role")
        self.admin_queries.record_audit_export(
            actor=Actor.web(9001),
            filters={"action": "role.create"},
            count=1,
        )
        export_audit = self.admin_queries.audit_logs(action="audit.export")
        self.assertEqual(export_audit["total"], 1)
        self.assertEqual(export_audit["items"][0]["detail"]["exported_rows"], 1)

    def test_configured_admin_uses_editable_database_permissions(self):
        with self.session_factory() as session:
            session.add(
                WebRole(
                    name="admin",
                    permissions_json='["reviews:read"]',
                    is_system=True,
                )
            )
            session.commit()
        with self.uow_factory() as uow:
            roles, permissions = self.auth._resolve_roles(uow, 9002)
        self.assertIn("admin", roles)
        self.assertIn("reviews:read", permissions)
        self.assertNotIn("users:*", permissions)
        admin = next(item for item in self.auth.list_roles() if item["name"] == "admin")
        self.assertEqual(admin["member_count"], 1)

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

    def test_registration_verification_creates_user_and_is_purpose_scoped(self):
        started = self.auth.create_telegram_login(
            ip_address="127.0.0.1",
            purpose="registration",
        )
        wrong_purpose = self.auth.claim_telegram_login(
            raw_token=started.data["request_token"],
            tg=1001,
        )
        self.assertEqual(wrong_purpose.status, "purpose_mismatch")

        claimed = self.auth.claim_telegram_login(
            raw_token=started.data["request_token"],
            tg=1001,
            display_name="new-user",
            expected_purpose="registration",
        )
        self.assertTrue(claimed.ok)
        self.assertTrue(
            self.auth.decide_telegram_login(
                request_id=claimed.data["request_id"],
                tg=1001,
                approve=True,
            ).ok
        )
        exchanged = self.auth.exchange_telegram_login(
            raw_token=started.data["request_token"],
            user_agent="test",
            ip_address="127.0.0.1",
            expected_purpose="registration",
        )
        self.assertTrue(exchanged.ok)
        identity = self.auth.authenticate(exchanged.data["session_token"])
        self.assertEqual(identity.purpose, "registration")
        self.assertEqual(identity.roles, ("member",))
        self.assertFalse(identity.permissions)
        with self.session_factory() as session:
            self.assertIsNotNone(session.get(Emby, 1001))

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

    def test_risk_events_can_be_triaged_and_are_audited(self):
        with self.uow_factory() as uow:
            created = uow.operations.security_event(
                event_type="device.banned_playback",
                severity="danger",
                subject_kind="device",
                subject_id="device-1",
                ip_address="127.0.0.1",
                detail={"session_id": "session-1"},
            )
            uow.flush()
            event_id = created.id

        summary = self.risk_events.summary()
        self.assertEqual(summary["open_total"], 1)
        self.assertEqual(summary["severity_counts"]["danger"], 1)
        listed = self.risk_events.list(status="open", severity="danger")
        self.assertEqual(listed["total"], 1)
        self.assertEqual(listed["items"][0]["event_type"], "device.banned_playback")

        updated = self.risk_events.update(
            event_id,
            status="resolved",
            assigned_to=1001,
            resolution_note="confirmed and blocked",
            actor=self.actor,
        )
        self.assertEqual(updated["status"], "resolved")
        self.assertEqual(updated["resolved_by"], 1001)
        self.assertIsNotNone(updated["resolved_at"])
        with self.session_factory() as session:
            self.assertEqual(session.query(SecurityEvent).count(), 1)
            self.assertEqual(
                session.query(AuditLog)
                .filter(AuditLog.action == "security.event.update")
                .count(),
                1,
            )
            self.assertEqual(
                session.query(SystemEvent)
                .filter(SystemEvent.event_type == "security.updated")
                .count(),
                1,
            )
            self.assertEqual(
                session.query(SystemEvent)
                .filter(SystemEvent.event_type == "security.created")
                .count(),
                1,
            )

    def test_dynamic_settings_support_conflicts_history_and_rollback(self):
        initial = self.dynamic_settings.get("economy.exchange_cost")
        self.assertEqual(initial["revision"], 0)

        first = self.dynamic_settings.update(
            "economy.exchange_cost",
            value=400,
            expected_revision=0,
            actor=self.actor,
        )
        self.assertEqual(first["revision"], 1)
        self.assertEqual(self.runtime_settings["economy.exchange_cost"], 400)

        with self.assertRaises(SettingConflictError):
            self.dynamic_settings.update(
                "economy.exchange_cost",
                value=450,
                expected_revision=0,
                actor=self.actor,
            )

        second = self.dynamic_settings.update_latest(
            "economy.exchange_cost",
            value=450,
            actor=self.actor,
        )
        rolled_back = self.dynamic_settings.rollback(
            "economy.exchange_cost",
            target_revision=1,
            expected_revision=second["revision"],
            actor=self.actor,
        )
        self.assertEqual(rolled_back["value"], 400)
        self.assertEqual(rolled_back["revision"], 3)
        self.runtime_settings["economy.exchange_cost"] = 999
        applied = self.dynamic_settings.apply_runtime_overrides()
        self.assertIn("economy.exchange_cost", applied["applied"])
        self.assertEqual(self.runtime_settings["economy.exchange_cost"], 400)

        history = self.dynamic_settings.history("economy.exchange_cost")
        self.assertEqual([item["revision"] for item in history["items"]], [3, 2, 1])
        with self.session_factory() as session:
            self.assertEqual(session.query(DynamicSetting).count(), 1)
            self.assertEqual(session.query(ConfigRevision).count(), 3)
            self.assertEqual(
                session.query(AuditLog)
                .filter(AuditLog.resource_type == "dynamic_setting")
                .count(),
                3,
            )
            self.assertEqual(
                session.query(SystemEvent)
                .filter(SystemEvent.event_type == "setting.updated")
                .count(),
                3,
            )

    def test_dynamic_settings_materialize_all_non_secret_defaults_once(self):
        first = self.dynamic_settings.materialize_defaults()
        second = self.dynamic_settings.materialize_defaults()
        self.assertGreater(first["count"], 30)
        self.assertEqual(second["count"], 0)
        settings = self.dynamic_settings.list()["items"]
        self.assertTrue(settings)
        self.assertTrue(all(item["source"] == "database" for item in settings))

    def test_governance_permissions_are_in_default_roles(self):
        self.assertIn("security:read", DEFAULT_ROLE_PERMISSIONS["admin"])
        self.assertIn("security:manage", DEFAULT_ROLE_PERMISSIONS["admin"])
        self.assertIn("settings:read", DEFAULT_ROLE_PERMISSIONS["admin"])
        self.assertIn("settings:manage", DEFAULT_ROLE_PERMISSIONS["admin"])
        self.assertIn("security:read", DEFAULT_ROLE_PERMISSIONS["operator"])
        self.assertNotIn("settings:manage", DEFAULT_ROLE_PERMISSIONS["operator"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
