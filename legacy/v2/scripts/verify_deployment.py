#!/usr/bin/env python3
"""Verify public health, runtime configuration and SPA assets after deployment."""

import argparse
import json
import time
from dataclasses import dataclass
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin
from urllib.request import Request, urlopen


@dataclass(frozen=True)
class HttpResult:
    status: int
    headers: dict[str, str]
    body: str


def fetch(url: str, timeout: float) -> HttpResult:
    request = Request(url, headers={"User-Agent": "Sakura-Deployment-Verify/1.0"})
    with urlopen(request, timeout=timeout) as response:
        return HttpResult(
            status=response.status,
            headers={key.lower(): value for key, value in response.headers.items()},
            body=response.read().decode("utf-8", errors="replace"),
        )


def verify_result(name: str, result: HttpResult, *, contains: str) -> list[str]:
    errors = []
    if not 200 <= result.status < 300:
        errors.append(f"{name} 返回 HTTP {result.status}")
    if contains not in result.body:
        errors.append(f"{name} 响应缺少 {contains!r}")
    if result.headers.get("x-frame-options", "").upper() != "DENY":
        errors.append(f"{name} 缺少 X-Frame-Options: DENY")
    if result.headers.get("x-content-type-options", "").lower() != "nosniff":
        errors.append(f"{name} 缺少 X-Content-Type-Options: nosniff")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="验证 Sakura EmbyBoss 上线结果")
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--admin-path", required=True)
    parser.add_argument("--user-path", default="app")
    parser.add_argument("--wait-seconds", type=int, default=120)
    parser.add_argument("--timeout", type=float, default=10)
    args = parser.parse_args()
    base_url = args.base_url.rstrip("/") + "/"

    deadline = time.monotonic() + max(0, args.wait_seconds)
    readiness = None
    while time.monotonic() <= deadline:
        try:
            readiness = fetch(urljoin(base_url, "readyz"), args.timeout)
            if readiness.status == 200:
                payload = json.loads(readiness.body)
                if payload.get("status") == "ready":
                    break
        except (HTTPError, URLError, TimeoutError, OSError, json.JSONDecodeError):
            pass
        time.sleep(3)
    if readiness is None or readiness.status != 200:
        print("[ERROR] readyz 未在等待时间内就绪")
        return 1

    targets = (
        ("healthz", "healthz", '"status":"ok"'),
        ("readyz", "readyz", '"status":"ready"'),
        (
            "用户端运行配置",
            f"{args.user_path.strip('/')}/runtime-config.js",
            f'"basePath": "/{args.user_path.strip("/")}"',
        ),
        (
            "管理端运行配置",
            f"{args.admin_path.strip('/')}/runtime-config.js",
            f'"basePath": "/{args.admin_path.strip("/")}"',
        ),
        ("用户中心", args.user_path.strip("/") + "/", 'id="app"'),
        ("管理后台", args.admin_path.strip("/") + "/", 'id="app"'),
    )
    errors: list[str] = []
    for name, path, contains in targets:
        try:
            result = fetch(urljoin(base_url, path), args.timeout)
            compact_body = result.body.replace(" ", "").replace("\n", "")
            expected = contains.replace(" ", "")
            errors.extend(
                verify_result(
                    name,
                    HttpResult(result.status, result.headers, compact_body),
                    contains=expected,
                )
            )
        except (HTTPError, URLError, TimeoutError, OSError) as exc:
            errors.append(f"{name} 无法访问：{exc}")

    for error in errors:
        print(f"[ERROR] {error}")
    if errors:
        print(f"\n上线验证失败：{len(errors)} 项")
        return 1
    print("[OK] 健康检查、运行配置、安全响应头和前端资源均正常")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
