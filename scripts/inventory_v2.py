#!/usr/bin/env python3
"""Build a deterministic, import-free inventory of the v2 codebase.

The scanner deliberately uses the Python AST and plain-text parsing instead of
importing Sakura modules. Importing v2 modules can create clients, read runtime
configuration, or run database migrations, which is unsafe for an inventory
command.
"""

from __future__ import annotations

import argparse
import ast
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any, Iterable


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT = ROOT / "docs" / "v3" / "generated" / "v2-inventory.json"
PYTHON_ROOTS = (ROOT / "backend", ROOT / "bot")
HTTP_METHODS = {"get", "post", "put", "patch", "delete"}
BOT_HANDLER_NAMES = {
    "on_message",
    "on_callback_query",
    "on_inline_query",
    "on_chosen_inline_result",
    "on_chat_member_updated",
    "on_raw_update",
}
SENSITIVE_CONFIG_PARTS = {
    "api",
    "hash",
    "key",
    "password",
    "pwd",
    "secret",
    "token",
}


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def python_files() -> list[Path]:
    files: list[Path] = [ROOT / "main.py"]
    for base in PYTHON_ROOTS:
        files.extend(base.rglob("*.py"))
    return sorted({path for path in files if path.is_file()})


def parse_python(path: Path) -> ast.Module | None:
    try:
        return ast.parse(path.read_text(encoding="utf-8-sig"), filename=str(path))
    except (SyntaxError, UnicodeDecodeError) as error:
        print(f"warning: cannot parse {relative(path)}: {error}", file=sys.stderr)
        return None


def literal_string(node: ast.AST | None) -> str | None:
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    if isinstance(node, ast.JoinedStr):
        parts: list[str] = []
        for value in node.values:
            if isinstance(value, ast.Constant) and isinstance(value.value, str):
                parts.append(value.value)
            elif isinstance(value, ast.FormattedValue):
                parts.append("{" + ast.unparse(value.value) + "}")
        return "".join(parts)
    return None


def call_name(node: ast.AST) -> str:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        prefix = call_name(node.value)
        return f"{prefix}.{node.attr}" if prefix else node.attr
    return ast.unparse(node)


def keyword(call: ast.Call, name: str) -> ast.AST | None:
    for item in call.keywords:
        if item.arg == name:
            return item.value
    return None


def router_prefixes(tree: ast.Module) -> dict[str, str]:
    prefixes: dict[str, str] = {}
    for node in tree.body:
        if not isinstance(node, (ast.Assign, ast.AnnAssign)):
            continue
        value = node.value
        if not isinstance(value, ast.Call) or call_name(value.func).split(".")[-1] != "APIRouter":
            continue
        targets = node.targets if isinstance(node, ast.Assign) else [node.target]
        prefix = literal_string(keyword(value, "prefix")) or ""
        for target in targets:
            if isinstance(target, ast.Name):
                prefixes[target.id] = prefix
    return prefixes


def scan_http_routes(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    prefixes = router_prefixes(tree)
    routes: list[dict[str, Any]] = []
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for decorator in node.decorator_list:
            if not isinstance(decorator, ast.Call) or not isinstance(decorator.func, ast.Attribute):
                continue
            method = decorator.func.attr.lower()
            if method not in HTTP_METHODS:
                continue
            router = call_name(decorator.func.value)
            declared_path = literal_string(decorator.args[0]) if decorator.args else ""
            if declared_path is None:
                declared_path = ast.unparse(decorator.args[0]) if decorator.args else ""
            routes.append(
                {
                    "method": method.upper(),
                    "router": router,
                    "router_prefix": prefixes.get(router, ""),
                    "declared_path": declared_path,
                    "handler": node.name,
                    "source": relative(path),
                    "line": decorator.lineno,
                }
            )
    return routes


def find_calls(node: ast.AST, terminal_name: str) -> Iterable[ast.Call]:
    for child in ast.walk(node):
        if isinstance(child, ast.Call) and call_name(child.func).split(".")[-1] == terminal_name:
            yield child


def scan_bot_handlers(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    handlers: list[dict[str, Any]] = []
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for decorator in node.decorator_list:
            if not isinstance(decorator, ast.Call) or not isinstance(decorator.func, ast.Attribute):
                continue
            kind = decorator.func.attr
            if kind not in BOT_HANDLER_NAMES:
                continue
            commands: list[str] = []
            regexes: list[str] = []
            for command_call in find_calls(decorator, "command"):
                if command_call.args:
                    value = command_call.args[0]
                    if isinstance(value, (ast.List, ast.Tuple)):
                        commands.extend(
                            item.value
                            for item in value.elts
                            if isinstance(item, ast.Constant) and isinstance(item.value, str)
                        )
                    else:
                        command = literal_string(value)
                        if command:
                            commands.append(command)
            for regex_call in find_calls(decorator, "regex"):
                if regex_call.args:
                    pattern = literal_string(regex_call.args[0])
                    if pattern:
                        regexes.append(pattern)
            handlers.append(
                {
                    "kind": kind,
                    "handler": node.name,
                    "commands": sorted(set(commands)),
                    "regexes": sorted(set(regexes)),
                    "filter_expression": ast.unparse(decorator),
                    "source": relative(path),
                    "line": decorator.lineno,
                }
            )
    return handlers


def scan_scheduler_jobs(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    jobs: list[dict[str, Any]] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call) or call_name(node.func).split(".")[-1] != "add_job":
            continue
        target = ast.unparse(node.args[0]) if node.args else None
        job_id = literal_string(keyword(node, "id"))
        trigger = None
        if len(node.args) > 1:
            trigger = literal_string(node.args[1]) or ast.unparse(node.args[1])
        if keyword(node, "trigger") is not None:
            trigger = literal_string(keyword(node, "trigger")) or ast.unparse(keyword(node, "trigger"))
        jobs.append(
            {
                "target": target,
                "id": job_id,
                "trigger": trigger,
                "expression": ast.unparse(node),
                "source": relative(path),
                "line": node.lineno,
            }
        )
    return jobs


def scan_task_definitions(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call) or call_name(node.func).split(".")[-1] != "TaskDefinition":
            continue
        task_type = literal_string(node.args[0]) if node.args else None
        if not task_type:
            continue
        tasks.append(
            {
                "task_type": task_type,
                "source": relative(path),
                "line": node.lineno,
            }
        )
    return tasks


def scan_models(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    models: list[dict[str, Any]] = []
    for node in tree.body:
        if not isinstance(node, ast.ClassDef):
            continue
        table_name = None
        columns: list[str] = []
        for item in node.body:
            if isinstance(item, ast.Assign):
                for target in item.targets:
                    if isinstance(target, ast.Name) and target.id == "__tablename__":
                        table_name = literal_string(item.value)
                    if isinstance(target, ast.Name) and isinstance(item.value, ast.Call):
                        if call_name(item.value.func).split(".")[-1] in {"Column", "mapped_column"}:
                            columns.append(target.id)
        if table_name:
            models.append(
                {
                    "table": table_name,
                    "model": node.name,
                    "columns": sorted(columns),
                    "source": relative(path),
                    "line": node.lineno,
                }
            )
    return models


def scan_environment(path: Path, tree: ast.Module) -> set[str]:
    names: set[str] = set()
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call) or not node.args:
            continue
        name = call_name(node.func)
        if name in {"os.getenv", "os.environ.get", "environ.get"}:
            value = literal_string(node.args[0])
            if value:
                names.add(value)
    return names


def scan_tests(path: Path, tree: ast.Module) -> list[dict[str, Any]]:
    tests: list[dict[str, Any]] = []
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith("test_"):
            tests.append({"name": node.name, "source": relative(path), "line": node.lineno})
    return tests


def flatten_config_keys(value: Any, prefix: str = "") -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    if not isinstance(value, dict):
        return entries
    for key in sorted(value):
        child = value[key]
        path = f"{prefix}.{key}" if prefix else key
        lowered = key.lower()
        sensitive = any(part in lowered for part in SENSITIVE_CONFIG_PARTS)
        entries.append(
            {
                "key": path,
                "type": type(child).__name__,
                "sensitive_candidate": sensitive,
            }
        )
        entries.extend(flatten_config_keys(child, path))
    return entries


def scan_compose_services() -> list[dict[str, Any]]:
    path = ROOT / "docker-compose.yml"
    services: list[dict[str, Any]] = []
    in_services = False
    current: dict[str, Any] | None = None
    for number, line in enumerate(path.read_text(encoding="utf-8-sig").splitlines(), 1):
        if line == "services:":
            in_services = True
            continue
        if in_services and line and not line.startswith(" "):
            break
        match = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
        if match and in_services:
            current = {"name": match.group(1), "source": relative(path), "line": number}
            services.append(current)
            continue
        if current:
            image = re.match(r"^    image:\s*(.+)$", line)
            if image:
                current["image"] = image.group(1).strip()
    return services


def scan_text_environment() -> set[str]:
    names: set[str] = set()
    for path in (ROOT / "docker-compose.yml", ROOT / "deploy.env.example", ROOT / ".env.web.example"):
        if not path.exists():
            continue
        content = path.read_text(encoding="utf-8-sig")
        names.update(re.findall(r"\$\{([A-Z][A-Z0-9_]*)", content))
        names.update(re.findall(r"^([A-Z][A-Z0-9_]*)=", content, flags=re.MULTILINE))
    return names


def scan_frontend() -> tuple[list[dict[str, Any]], list[str]]:
    router_path = ROOT / "web/src/router.ts"
    routes: list[dict[str, Any]] = []
    if router_path.exists():
        for number, line in enumerate(router_path.read_text(encoding="utf-8-sig").splitlines(), 1):
            match = re.search(r"\bpath:\s*[\"']([^\"']*)[\"']", line)
            if match:
                routes.append(
                    {
                        "path": match.group(1),
                        "source": relative(router_path),
                        "line": number,
                    }
                )
    views_root = ROOT / "web/src/views"
    views = [relative(path) for path in sorted(views_root.rglob("*.vue"))] if views_root.exists() else []
    return routes, views


def inventory() -> dict[str, Any]:
    routes: list[dict[str, Any]] = []
    handlers: list[dict[str, Any]] = []
    jobs: list[dict[str, Any]] = []
    tasks: list[dict[str, Any]] = []
    models: list[dict[str, Any]] = []
    tests: list[dict[str, Any]] = []
    environment = scan_text_environment()

    for path in python_files():
        tree = parse_python(path)
        if tree is None:
            continue
        routes.extend(scan_http_routes(path, tree))
        handlers.extend(scan_bot_handlers(path, tree))
        jobs.extend(scan_scheduler_jobs(path, tree))
        tasks.extend(scan_task_definitions(path, tree))
        models.extend(scan_models(path, tree))
        environment.update(scan_environment(path, tree))

    for path in sorted((ROOT / "scripts").glob("test_*.py")):
        if path.name == "test_v2_inventory.py":
            continue
        tree = parse_python(path)
        if tree is not None:
            tests.extend(scan_tests(path, tree))

    config = json.loads((ROOT / "config_example.json").read_text(encoding="utf-8-sig"))
    migrations = [relative(path) for path in sorted((ROOT / "bot/sql_helper/alembic/versions").glob("*.py"))]

    key = lambda item: (item.get("source", ""), item.get("line", 0), item.get("handler", ""))
    routes.sort(key=key)
    handlers.sort(key=key)
    jobs.sort(key=key)
    tasks.sort(key=lambda item: item["task_type"])
    models.sort(key=lambda item: item["table"].lower())
    tests.sort(key=key)

    frontend_routes, frontend_views = scan_frontend()
    result: dict[str, Any] = {
        "schema_version": 1,
        "source_revision": "working-tree",
        "compose_services": scan_compose_services(),
        "http_routes": routes,
        "bot_handlers": handlers,
        "scheduler_jobs": jobs,
        "task_definitions": tasks,
        "database_models": models,
        "migrations": migrations,
        "configuration_keys": flatten_config_keys(config),
        "environment_variables": sorted(environment),
        "frontend_routes": frontend_routes,
        "frontend_views": frontend_views,
        "characterization_tests": tests,
    }
    result["summary"] = {
        "compose_services": len(result["compose_services"]),
        "http_routes": len(routes),
        "bot_handlers": len(handlers),
        "bot_commands": len({command for item in handlers for command in item["commands"]}),
        "scheduler_jobs": len(jobs),
        "task_definitions": len(tasks),
        "database_models": len(models),
        "migrations": len(migrations),
        "configuration_keys": len(result["configuration_keys"]),
        "environment_variables": len(environment),
        "frontend_routes": len(frontend_routes),
        "frontend_views": len(frontend_views),
        "characterization_tests": len(tests),
    }
    return result


def canonical_bytes(value: dict[str, Any]) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true", help="fail when the committed snapshot differs")
    args = parser.parse_args()

    generated = canonical_bytes(inventory())
    output = args.output if args.output.is_absolute() else ROOT / args.output
    if args.check:
        if not output.exists():
            print(f"missing inventory snapshot: {relative(output)}", file=sys.stderr)
            return 1
        current = output.read_bytes()
        if current != generated:
            print(
                "v2 inventory drift detected; run python scripts/inventory_v2.py and review the diff",
                file=sys.stderr,
            )
            print(f"expected sha256={hashlib.sha256(current).hexdigest()}", file=sys.stderr)
            print(f"actual   sha256={hashlib.sha256(generated).hexdigest()}", file=sys.stderr)
            return 1
        print(f"v2 inventory is current: {relative(output)}")
        return 0

    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(generated)
    summary = json.loads(generated)["summary"]
    print(f"wrote {relative(output)}")
    for name, count in summary.items():
        print(f"  {name}: {count}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
