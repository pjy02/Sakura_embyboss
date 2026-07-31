#!/usr/bin/env python3
"""Regression tests for production deployment wiring."""

import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


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


if __name__ == "__main__":
    unittest.main()
