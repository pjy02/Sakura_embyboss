#!/usr/bin/env python3
"""Source-level contract tests for platform centers without external services."""

import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class PlatformContractTests(unittest.TestCase):
    def test_migration_contains_all_shared_platform_tables(self):
        source = (ROOT / "bot/sql_helper/alembic/versions/20260731_11_platform_center.py").read_text(encoding="utf-8")
        self.assertIn('down_revision = "20260731_10"', source)
        for table in (
            "device_client_rules", "managed_credentials", "emby_instances",
            "account_emby_bindings", "media_catalog_items", "automation_rules",
            "automation_runs", "api_clients",
        ):
            self.assertIn(f'"{table}"', source)

    def test_vault_encrypts_secrets_and_api_keys_are_hashed(self):
        source = (ROOT / "bot/application/platform_service.py").read_text(encoding="utf-8")
        self.assertIn("Fernet", source)
        self.assertIn("SAKURA_CREDENTIAL_MASTER_KEY", source)
        self.assertIn("hashlib.sha256(raw_key.encode", source)
        public_serializer = source[source.index("def _credential_public"):source.index("class CredentialService")]
        self.assertNotIn('"ciphertext"', public_serializer)

    def test_multi_emby_routing_honors_runtime_switch_and_legacy_adoption(self):
        platform = (ROOT / "bot/application/platform_service.py").read_text(encoding="utf-8")
        registration = (ROOT / "bot/application/registration_service.py").read_text(encoding="utf-8")
        operations = (ROOT / "bot/application/core_operations_service.py").read_text(encoding="utf-8")
        api = (ROOT / "backend/api/platform.py").read_text(encoding="utf-8")
        self.assertIn("integrations.multi_emby_enabled", platform)
        self.assertIn("managed_service.feature_enabled()", registration)
        self.assertIn("managed.feature_enabled()", operations)
        self.assertIn("adopt-legacy", api)
        self.assertIn('ManagedCredential.provider == "emby"', platform)

    def test_open_api_and_admin_routes_are_registered(self):
        source = (ROOT / "backend/api/platform.py").read_text(encoding="utf-8")
        self.assertIn('prefix="/api/open/v1"', source)
        self.assertIn("require_api_scope", source)
        app = (ROOT / "backend/app.py").read_text(encoding="utf-8")
        self.assertIn("app.include_router(open_router)", app)
        self.assertIn("api_v1.include_router(platform_admin_router)", app)

    def test_worker_owns_platform_scheduling(self):
        worker = (ROOT / "backend/worker.py").read_text(encoding="utf-8")
        for task_type in (
            "automation.evaluate", "monitor.diagnostics", "monitor.emby_instances",
            "sync.moviepilot", "maintenance.backup_database",
        ):
            self.assertIn(task_type, worker)

    def test_backup_avoids_shell_interpolation(self):
        helper = (ROOT / "bot/func_helper/backup_db_utils.py").read_text(encoding="utf-8")
        direct = helper[helper.index("async def backup_mysql_db("):helper.index("async def backup_mysql_db_docker(")]
        self.assertIn("create_subprocess_exec", direct)
        self.assertIn('"MYSQL_PWD"', direct)
        self.assertNotIn("create_subprocess_shell", direct)

    def test_web_routes_cover_all_new_centers(self):
        router = (ROOT / "web/src/router.ts").read_text(encoding="utf-8")
        for route in ("automation", "system/infrastructure", "system/recovery", "media"):
            self.assertIn(f'path: "{route}"', router)


if __name__ == "__main__":
    unittest.main()
