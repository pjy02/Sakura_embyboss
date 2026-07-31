#!/usr/bin/env python3
import os
import sys
import types
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
BOT_DIR = ROOT / "bot"
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

os.environ.setdefault("SAKURA_RUNNING_MIGRATIONS", "1")
os.environ.setdefault(
    "SAKURA_WEB_SESSION_SECRET",
    "test-web-session-secret-that-is-long-enough",
)

bot_stub = types.ModuleType("bot")
bot_stub.__path__ = [str(BOT_DIR)]
bot_stub.db_host = "localhost"
bot_stub.db_user = "unused"
bot_stub.db_pwd = "unused"
bot_stub.db_name = "unused"
bot_stub.db_port = 3306
bot_stub.owner = 9001
bot_stub.admins = [9002]
bot_stub.bot_name = "sakura_test_bot"
bot_stub.bot_token = "test-token"
bot_stub.LOGGER = SimpleNamespace(
    debug=lambda *args, **kwargs: None,
    info=lambda *args, **kwargs: None,
    warning=lambda *args, **kwargs: None,
    error=lambda *args, **kwargs: None,
)
bot_stub.api = SimpleNamespace(
    status=True,
    http_url="127.0.0.1",
    http_port=8838,
    allow_origins=[],
    admin_path="hidden-console",
    user_path="app",
    public_base_url=None,
    cookie_secure=False,
    session_ttl_hours=24,
    login_ttl_seconds=300,
    legacy_api_enabled=False,
    docs_enabled=False,
    trusted_hosts=["testserver"],
)
sys.modules["bot"] = bot_stub

emby_module = types.ModuleType("bot.func_helper.emby")


class FakeEmby:
    async def authority_account(self, **_kwargs):
        return False, None


emby_module.emby = FakeEmby()
sys.modules["bot.func_helper.emby"] = emby_module

from sqlalchemy import BigInteger, create_engine
from sqlalchemy.ext.compiler import compiles
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool


@compiles(BigInteger, "sqlite")
def _compile_big_integer_as_integer(_type, _compiler, **_kwargs):
    return "INTEGER"


from fastapi.testclient import TestClient

from backend.api.tasks import _admin_event_prefixes
from backend.app import create_app
from backend.settings import WebSettings, get_settings
from bot.application import (
    AdminQueryService,
    PointService,
    ReliabilityService,
    TaskService,
    TokenCodec,
    WebAuthService,
)
from bot.application.auth_service import WebIdentity
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper import Base
from bot.sql_helper.sql_emby import Emby


class WebAuthRouteTests(unittest.TestCase):
    def setUp(self):
        self.engine = create_engine(
            "sqlite://",
            connect_args={"check_same_thread": False},
            poolclass=StaticPool,
        )
        Base.metadata.create_all(self.engine)
        self.session_factory = sessionmaker(
            bind=self.engine,
            autoflush=False,
            expire_on_commit=False,
        )
        self.uow_factory = lambda: SqlAlchemyUnitOfWork(self.session_factory)
        self.auth = WebAuthService(
            token_codec=TokenCodec("test-web-session-secret-that-is-long-enough"),
            owner_tg=9001,
            admin_tg_ids=[9002],
            uow_factory=self.uow_factory,
        )
        with self.session_factory() as session:
            session.add_all(
                [
                    Emby(tg=1001, embyid="emby-1", name="alice", lv="b"),
                    Emby(tg=9001, embyid="emby-owner", name="owner", lv="a"),
                ]
            )
            session.commit()

        self.settings = WebSettings(
            host="127.0.0.1",
            port=8838,
            admin_path="hidden-console",
            user_path="app",
            public_base_url=None,
            cookie_secure=False,
            cookie_name="sakura_session",
            csrf_cookie_name="sakura_csrf",
            session_ttl_hours=24,
            login_ttl_seconds=300,
            session_secret="test-web-session-secret-that-is-long-enough",
            cors_origins=(),
            trusted_hosts=("testserver",),
            docs_enabled=False,
            legacy_api_enabled=False,
            legacy_api_token=None,
            owner_tg=9001,
            admin_tg_ids=(9002,),
            bot_username="sakura_test_bot",
        )
        self.app = create_app(self.settings)
        self.app.dependency_overrides[get_settings] = lambda: self.settings
        self.patches = [
            patch("backend.api.auth.get_auth_service", return_value=self.auth),
            patch("backend.api.admin.get_auth_service", return_value=self.auth),
            patch("backend.dependencies.get_auth_service", return_value=self.auth),
            patch(
                "backend.api.admin.queries",
                AdminQueryService(self.uow_factory),
            ),
            patch(
                "backend.api.admin.points",
                PointService(self.uow_factory),
            ),
            patch(
                "backend.api.tasks.tasks",
                TaskService(self.uow_factory),
            ),
            patch(
                "backend.api.tasks.reliability",
                ReliabilityService(self.uow_factory),
            ),
        ]
        for item in self.patches:
            item.start()
        self.client = TestClient(self.app)

    def tearDown(self):
        self.client.close()
        for item in reversed(self.patches):
            item.stop()
        Base.metadata.drop_all(self.engine)
        self.engine.dispose()

    def test_custom_admin_path_and_common_decoys(self):
        self.assertEqual(self.client.get("/hidden-console").status_code, 200)
        self.assertEqual(self.client.get("/app").status_code, 200)
        runtime = self.client.get("/hidden-console/runtime-config.js")
        self.assertEqual(runtime.status_code, 200)
        self.assertIn('"basePath": "/hidden-console"', runtime.text)
        self.assertNotIn(self.settings.session_secret, runtime.text)
        self.assertEqual(self.client.get("/admin").status_code, 404)
        self.assertEqual(self.client.get("/manage").status_code, 404)
        health = self.client.get("/healthz")
        self.assertEqual(health.status_code, 200)
        self.assertEqual(health.headers["x-frame-options"], "DENY")

    def test_telegram_login_cookie_csrf_and_logout(self):
        started = self.client.post("/api/v1/auth/telegram/start", json={})
        self.assertEqual(started.status_code, 201)
        token = started.json()["request_token"]
        self.assertIn("https://t.me/sakura_test_bot?start=web_", started.json()["deep_link"])

        claimed = self.auth.claim_telegram_login(raw_token=token, tg=1001)
        self.auth.decide_telegram_login(
            request_id=claimed.data["request_id"],
            tg=1001,
            approve=True,
        )
        exchanged = self.client.post(
            "/api/v1/auth/telegram/exchange",
            json={"token": token},
        )
        self.assertEqual(exchanged.status_code, 200)
        self.assertIn("sakura_session", self.client.cookies)
        self.assertIn("sakura_csrf", self.client.cookies)

        session = self.client.get("/api/v1/auth/session")
        self.assertEqual(session.status_code, 200)
        self.assertEqual(session.json()["tg"], 1001)

        no_csrf = self.client.post("/api/v1/auth/logout")
        self.assertEqual(no_csrf.status_code, 403)
        logout = self.client.post(
            "/api/v1/auth/logout",
            headers={"X-CSRF-Token": self.client.cookies["sakura_csrf"]},
        )
        self.assertEqual(logout.status_code, 204)
        self.assertEqual(self.client.get("/api/v1/auth/session").status_code, 401)

    def test_unauthenticated_profile_is_rejected(self):
        response = self.client.get("/api/v1/me")
        self.assertEqual(response.status_code, 401)

    def test_admin_event_prefixes_follow_module_permissions(self):
        identity = WebIdentity(
            session_id="session-1",
            tg=9003,
            auth_method="telegram",
            roles=("content_moderator",),
            permissions=frozenset({"reviews:read", "notifications:read"}),
            csrf_hash="unused",
        )
        self.assertEqual(
            _admin_event_prefixes(identity),
            ("review", "notification"),
        )

    def test_admin_api_requires_telegram_auth_even_for_owner(self):
        emby_session = self.auth.create_emby_session(
            embyid="emby-owner",
            username="owner",
            user_agent="test",
            ip_address="127.0.0.1",
        )
        self.client.cookies.set(
            "sakura_session",
            emby_session.data["session_token"],
        )
        self.client.cookies.set("sakura_csrf", emby_session.data["csrf_token"])
        denied = self.client.get("/api/v1/admin/users")
        self.assertEqual(denied.status_code, 403)

        self.client.cookies.clear()
        started = self.auth.create_telegram_login(
            ip_address="127.0.0.1",
            requested_tg=9001,
        )
        claimed = self.auth.claim_telegram_login(
            raw_token=started.data["request_token"],
            tg=9001,
        )
        self.auth.decide_telegram_login(
            request_id=claimed.data["request_id"],
            tg=9001,
            approve=True,
        )
        telegram_session = self.auth.exchange_telegram_login(
            raw_token=started.data["request_token"],
            user_agent="test",
            ip_address="127.0.0.1",
        )
        self.client.cookies.set(
            "sakura_session",
            telegram_session.data["session_token"],
        )
        self.client.cookies.set(
            "sakura_csrf",
            telegram_session.data["csrf_token"],
        )
        allowed = self.client.get("/api/v1/admin/users")
        self.assertEqual(allowed.status_code, 200)
        overview = self.client.get("/api/v1/admin/overview")
        self.assertEqual(overview.status_code, 200)
        self.assertEqual(overview.json()["users_total"], 2)
        detail = self.client.get("/api/v1/admin/users/1001")
        self.assertEqual(detail.status_code, 200)
        self.assertIn("roles", detail.json())
        created_task = self.client.post(
            "/api/v1/admin/tasks",
            headers={
                "X-CSRF-Token": telegram_session.data["csrf_token"],
                "Idempotency-Key": "web-route-task-1",
            },
            json={
                "task_type": "sync.favorites",
                "payload": {},
                "confirm": True,
            },
        )
        self.assertEqual(created_task.status_code, 202)
        listed_tasks = self.client.get("/api/v1/admin/tasks")
        self.assertEqual(listed_tasks.status_code, 200)
        self.assertEqual(listed_tasks.json()["total"], 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
