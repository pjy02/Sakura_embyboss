#!/usr/bin/env python3
"""Run isolated project test scripts with a disposable CI configuration."""

import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TESTS = (
    "scripts/test_annotation_safety.py",
    "scripts/test_application_services.py",
    "scripts/test_task_reliability.py",
    "scripts/test_web_auth.py",
    "scripts/test_deployment_contract.py",
    "scripts/test_register_queue.py",
    "scripts/test_emby_policy.py",
)


def main() -> int:
    config_path = ROOT / "config.json"
    created_config = False
    if not config_path.exists():
        shutil.copyfile(ROOT / "config_example.json", config_path)
        created_config = True

    env = os.environ.copy()
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    env["SAKURA_RUNNING_MIGRATIONS"] = "1"
    env["REGISTER_QUEUE_REAL"] = "0"

    try:
        for relative_path in TESTS:
            print(f"\n==> {relative_path}", flush=True)
            result = subprocess.run(
                [sys.executable, str(ROOT / relative_path)],
                cwd=ROOT,
                env=env,
                check=False,
            )
            if result.returncode:
                return result.returncode
    finally:
        if created_config:
            config_path.unlink(missing_ok=True)

    print("\nAll project tests passed.", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
