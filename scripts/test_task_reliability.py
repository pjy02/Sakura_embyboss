#!/usr/bin/env python3
"""Database-only tests for durable tasks, leases, retries and realtime events."""

import os
import sys
import types
import unittest
from datetime import timedelta
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


from bot.application import ReliabilityService, TaskService
from bot.domain import Actor
from bot.repositories import SqlAlchemyUnitOfWork
from bot.sql_helper import Base
from bot.sql_helper.sql_application import OperationTask, utcnow


class TaskReliabilityTests(unittest.TestCase):
    def setUp(self):
        self.engine = create_engine("sqlite:///:memory:")
        Base.metadata.create_all(self.engine)
        self.session_factory = sessionmaker(
            bind=self.engine,
            autoflush=False,
            expire_on_commit=False,
        )
        self.uow_factory = lambda: SqlAlchemyUnitOfWork(self.session_factory)
        self.tasks = TaskService(self.uow_factory)
        self.reliability = ReliabilityService(self.uow_factory)
        self.actor = Actor.web(9001)

    def tearDown(self):
        Base.metadata.drop_all(self.engine)
        self.engine.dispose()

    def enqueue(self, key="task-key"):
        return self.tasks.enqueue(
            task_type="sync.favorites",
            payload={},
            actor=self.actor,
            idempotency_key=key,
        )

    def test_enqueue_is_idempotent_and_emits_event(self):
        first = self.enqueue()
        second = self.enqueue()
        self.assertTrue(first.ok)
        self.assertEqual(first.data["id"], second.data["id"])
        self.assertEqual(self.tasks.list()["total"], 1)
        events = self.reliability.events_after(after_id=0)
        self.assertEqual(events[0]["event_type"], "task.created")

    def test_claim_heartbeat_and_complete(self):
        task_id = self.enqueue().data["id"]
        claimed = self.tasks.claim(worker_id="worker-1", lease_seconds=30)
        self.assertEqual(claimed["id"], task_id)
        self.assertEqual(claimed["status"], "running")
        self.assertTrue(self.tasks.heartbeat(task_id, "worker-1", 30))
        self.assertTrue(self.tasks.complete(task_id, "worker-1", {"items": 3}))
        completed = self.tasks.get(task_id)
        self.assertEqual(completed["status"], "succeeded")
        self.assertEqual(completed["result"]["items"], 3)

    def test_failure_retries_then_can_be_manually_retried(self):
        task_id = self.enqueue().data["id"]
        self.tasks.claim(worker_id="worker-1", lease_seconds=30)
        self.tasks.fail(task_id, "worker-1", "temporary error")
        failed_once = self.tasks.get(task_id)
        self.assertEqual(failed_once["status"], "retrying")
        self.assertEqual(failed_once["retry_count"], 1)

        with self.session_factory() as session:
            row = session.get(OperationTask, task_id)
            row.status = "failed"
            row.finished_at = utcnow()
            session.commit()
        retried = self.tasks.retry(task_id, self.actor)
        self.assertTrue(retried.ok)
        self.assertEqual(retried.data["status"], "pending")
        self.assertEqual(retried.data["retry_count"], 0)

    def test_expired_lease_is_recovered(self):
        task_id = self.enqueue().data["id"]
        self.tasks.claim(worker_id="dead-worker", lease_seconds=30)
        with self.session_factory() as session:
            row = session.get(OperationTask, task_id)
            row.lease_expires_at = utcnow() - timedelta(seconds=1)
            session.commit()
        self.assertIsNone(self.tasks.claim(worker_id="new-worker", lease_seconds=30))
        recovered = self.tasks.get(task_id)
        self.assertEqual(recovered["status"], "retrying")
        self.assertEqual(recovered["retry_count"], 1)

    def test_pending_cancel_and_worker_status(self):
        task_id = self.enqueue().data["id"]
        canceled = self.tasks.cancel(task_id, self.actor)
        self.assertEqual(canceled.data["status"], "canceled")
        self.reliability.heartbeat(
            worker_id="worker-1",
            worker_kind="task-worker",
            status="idle",
        )
        status = self.reliability.status()
        self.assertEqual(status["status"], "healthy")
        self.assertEqual(status["task_counts"]["canceled"], 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
