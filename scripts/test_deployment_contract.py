#!/usr/bin/env python3
"""Regression tests for production deployment wiring."""

import re
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.preflight import validate
from scripts.verify_deployment import HttpResult, verify_result


def compose_block(source: str, name: str, *, indent: int) -> str:
    prefix = " " * indent
    match = re.search(
        rf"(?ms)^{re.escape(prefix + name)}:\s*(?:&[^\s]+\s*)?\n"
        rf"(?P<body>.*?)(?=^{re.escape(prefix)}\S[^:\n]*:\s*(?:\n|$)|\Z)",
        source,
    )
    if not match:
        raise AssertionError(f"Compose block not found: {name}")
    return match.group("body")


class DeploymentContractTests(unittest.TestCase):
    def test_bot_and_web_receive_the_same_login_configuration(self):
        source = (ROOT / "docker-compose.yml").read_text(encoding="utf-8")
        services = compose_block(source, "services", indent=0)
        bot = compose_block(services, "bot", indent=2)
        web = compose_block(services, "web", indent=2)

        expected_settings = (
            "SAKURA_WEB_SESSION_SECRET:",
            "SAKURA_LOGIN_TTL_SECONDS:",
            "SAKURA_SESSION_TTL_HOURS:",
        )
        for setting in expected_settings:
            self.assertIn(setting, bot)
            self.assertIn(setting, web)

    def test_example_environment_defines_the_shared_login_secret(self):
        source = (ROOT / "deploy.env.example").read_text(encoding="utf-8")
        self.assertRegex(
            source,
            r"(?m)^SAKURA_WEB_SESSION_SECRET=\S{32,}$",
        )

    def test_preflight_accepts_safe_production_configuration(self):
        env = {
            "SAKURA_IMAGE": "233bit/sakura_embyboss:2.3.0",
            "MYSQL_ROOT_PASSWORD": "root-secret-that-is-different",
            "MYSQL_PASSWORD": "app-secret-that-is-different",
            "SAKURA_WEB_SESSION_SECRET": "a" * 64,
            "SAKURA_PUBLIC_BASE_URL": "https://media.example.net",
            "SAKURA_COOKIE_SECURE": "true",
            "SAKURA_TRUSTED_HOSTS": "media.example.net",
            "WEB_ADMIN_PATH": "console-k7fd92",
            "WEB_USER_PATH": "app",
            "SAKURA_WEB_BIND_IP": "127.0.0.1",
        }
        config = {
            "bot_name": "sakura_real_bot",
            "bot_token": "123456:real-token-value-that-is-long",
            "owner_api": 123456,
            "owner_hash": "0123456789abcdef0123456789abcdef",
            "owner": 10001,
            "emby_api": "real-emby-api-key",
            "emby_url": "https://emby.internal.test",
        }
        findings = validate(env, config)
        self.assertFalse([item for item in findings if item.level == "error"])

    def test_preflight_rejects_public_database_style_configuration(self):
        env = {
            "SAKURA_IMAGE": "sakura_embyboss:latest",
            "MYSQL_ROOT_PASSWORD": "same-password",
            "MYSQL_PASSWORD": "same-password",
            "SAKURA_WEB_SESSION_SECRET": "short",
            "SAKURA_PUBLIC_BASE_URL": "http://emby.example.com",
            "SAKURA_COOKIE_SECURE": "false",
            "SAKURA_TRUSTED_HOSTS": "*",
            "WEB_ADMIN_PATH": "admin",
            "WEB_USER_PATH": "admin",
            "SAKURA_WEB_BIND_IP": "0.0.0.0",
        }
        findings = validate(env, {})
        self.assertGreaterEqual(
            len([item for item in findings if item.level == "error"]),
            8,
        )

    def test_deploy_script_contains_backup_health_wait_and_rollback(self):
        source = (ROOT / "scripts" / "deploy.sh").read_text(encoding="utf-8")
        self.assertIn("mysqldump", source)
        self.assertIn("--wait-timeout 180", source)
        self.assertIn("rollback-", source)

    def test_public_verifier_requires_security_headers_and_expected_body(self):
        valid = HttpResult(
            status=200,
            headers={
                "x-frame-options": "DENY",
                "x-content-type-options": "nosniff",
            },
            body='{"status":"ready"}',
        )
        self.assertEqual(
            verify_result("readyz", valid, contains='"status":"ready"'),
            [],
        )
        invalid = HttpResult(status=200, headers={}, body="placeholder")
        self.assertEqual(len(verify_result("portal", invalid, contains='id="app"')), 3)


if __name__ == "__main__":
    unittest.main()
