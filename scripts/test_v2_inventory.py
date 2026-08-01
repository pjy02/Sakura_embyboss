#!/usr/bin/env python3
"""Verify the committed v2 freeze inventory and phase-0 documentation."""

import unittest
from pathlib import Path

import inventory_v2


ROOT = Path(__file__).resolve().parents[1]
SNAPSHOT = ROOT / "docs/v3/generated/v2-inventory.json"
FREEZE_DOC = ROOT / "docs/v3/README.md"
INVENTORY_DOC = ROOT / "docs/v3/v2-system-inventory.md"
MIGRATION_DOC = ROOT / "docs/v3/v2-migration-decisions.md"
PRODUCTION_CHECKLIST = ROOT / "docs/v3/phase-0-production-checklist.md"


class V2InventoryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.generated = inventory_v2.inventory()

    def test_committed_snapshot_matches_working_tree(self):
        self.assertTrue(SNAPSHOT.is_file())
        self.assertEqual(
            SNAPSHOT.read_bytes(),
            inventory_v2.canonical_bytes(self.generated),
            "v2 inventory drifted; regenerate the snapshot and review the diff",
        )

    def test_inventory_covers_all_critical_entry_types(self):
        summary = self.generated["summary"]
        self.assertGreater(summary["http_routes"], 0)
        self.assertGreater(summary["bot_handlers"], 0)
        self.assertGreater(summary["scheduler_jobs"], 0)
        self.assertGreater(summary["task_definitions"], 0)
        self.assertGreater(summary["database_models"], 0)
        self.assertGreater(summary["migrations"], 0)
        self.assertGreater(summary["frontend_routes"], 0)
        self.assertGreater(summary["frontend_views"], 0)
        self.assertGreater(summary["characterization_tests"], 0)

    def test_every_known_model_has_an_explicit_migration_decision(self):
        decisions = MIGRATION_DOC.read_text(encoding="utf-8")
        missing = [
            item["table"]
            for item in self.generated["database_models"]
            if f"| `{item['table']}` |" not in decisions
        ]
        self.assertEqual(missing, [], f"tables without migration decision: {missing}")

    def test_phase_zero_documents_freeze_and_known_boundaries(self):
        freeze = FREEZE_DOC.read_text(encoding="utf-8")
        inventory = INVENTORY_DOC.read_text(encoding="utf-8")
        self.assertIn("v2 冻结规则", freeze)
        self.assertIn("第 0 阶段：冻结和盘点 | 仓库基线已完成", freeze)
        for boundary in ("Web 不调用 Bot", "Bot 不直接查询数据库"):
            self.assertIn(boundary, freeze)
        self.assertIn("反向依赖", inventory)
        checklist = PRODUCTION_CHECKLIST.read_text(encoding="utf-8")
        self.assertIn("profile_v2_database.py", checklist)
        self.assertIn("不包含用户名、密码、Token", checklist)


if __name__ == "__main__":
    unittest.main(verbosity=2)
