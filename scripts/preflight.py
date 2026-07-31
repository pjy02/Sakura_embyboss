#!/usr/bin/env python3
"""Production configuration preflight without third-party dependencies."""

import argparse
import json
import re
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlparse


PLACEHOLDER_MARKERS = (
    "replace-with",
    "your-dockerhub",
    "example.com",
    "255.255.255.255",
    "xxxxx",
    "xxxbot",
)
RESERVED_PATHS = {"admin", "api", "dashboard", "healthz", "manage", "readyz"}


@dataclass(frozen=True)
class Finding:
    level: str
    message: str


def parse_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line_number, raw_line in enumerate(
        path.read_text(encoding="utf-8").splitlines(),
        1,
    ):
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError(f"{path}:{line_number} 不是 KEY=VALUE 格式")
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip()
        if value[:1] == value[-1:] and value[:1] in {"'", '"'}:
            value = value[1:-1]
        elif " #" in value:
            value = value.split(" #", 1)[0].rstrip()
        if not re.fullmatch(r"[A-Z][A-Z0-9_]*", key):
            raise ValueError(f"{path}:{line_number} 环境变量名无效")
        values[key] = value
    return values


def _placeholder(value: object) -> bool:
    text = str(value or "").strip().lower()
    return not text or any(marker in text for marker in PLACEHOLDER_MARKERS)


def validate(
    env: dict[str, str],
    config: dict,
    *,
    allow_latest: bool = False,
) -> list[Finding]:
    findings: list[Finding] = []

    def error(message: str):
        findings.append(Finding("error", message))

    def warning(message: str):
        findings.append(Finding("warning", message))

    required_env = (
        "SAKURA_IMAGE",
        "MYSQL_ROOT_PASSWORD",
        "MYSQL_PASSWORD",
        "SAKURA_WEB_SESSION_SECRET",
        "SAKURA_PUBLIC_BASE_URL",
        "SAKURA_TRUSTED_HOSTS",
        "WEB_ADMIN_PATH",
        "WEB_USER_PATH",
    )
    for key in required_env:
        if _placeholder(env.get(key)):
            error(f"{key} 未填写或仍是示例值")

    secret = env.get("SAKURA_WEB_SESSION_SECRET", "")
    if secret and len(secret.encode("utf-8")) < 32:
        error("SAKURA_WEB_SESSION_SECRET 至少需要 32 字节")
    if (
        env.get("MYSQL_ROOT_PASSWORD")
        and env.get("MYSQL_ROOT_PASSWORD") == env.get("MYSQL_PASSWORD")
    ):
        error("MYSQL_ROOT_PASSWORD 和 MYSQL_PASSWORD 不能相同")
    for key in ("MYSQL_ROOT_PASSWORD", "MYSQL_PASSWORD"):
        if env.get(key) and len(env[key]) < 16:
            error(f"{key} 至少需要 16 个字符")

    image = env.get("SAKURA_IMAGE", "")
    if image and "/" not in image:
        error("SAKURA_IMAGE 应填写完整仓库名，例如 233bit/sakura_embyboss:2.3.0")
    if image.endswith(":latest") and not allow_latest:
        warning("生产环境建议固定版本标签，便于可靠回滚")

    public_url = urlparse(env.get("SAKURA_PUBLIC_BASE_URL", ""))
    if public_url.scheme != "https" or not public_url.hostname:
        error("SAKURA_PUBLIC_BASE_URL 必须是有效的 HTTPS 地址")
    trusted_hosts = {
        item.strip()
        for item in env.get("SAKURA_TRUSTED_HOSTS", "").split(",")
        if item.strip()
    }
    if "*" in trusted_hosts:
        error("生产环境 SAKURA_TRUSTED_HOSTS 不能使用 *")
    if public_url.hostname and public_url.hostname not in trusted_hosts:
        error("SAKURA_TRUSTED_HOSTS 必须包含公网域名")
    if env.get("SAKURA_COOKIE_SECURE", "true").lower() != "true":
        error("生产环境必须设置 SAKURA_COOKIE_SECURE=true")

    admin_path = env.get("WEB_ADMIN_PATH", "")
    user_path = env.get("WEB_USER_PATH", "")
    path_pattern = re.compile(r"[A-Za-z0-9][A-Za-z0-9_-]{2,63}")
    if not path_pattern.fullmatch(admin_path) or admin_path.lower() in RESERVED_PATHS:
        error("WEB_ADMIN_PATH 必须是 3-64 位非保留随机路径")
    if not path_pattern.fullmatch(user_path):
        error("WEB_USER_PATH 必须是 3-64 位路径")
    if admin_path and admin_path == user_path:
        error("WEB_ADMIN_PATH 和 WEB_USER_PATH 不能相同")

    bind_ip = env.get("SAKURA_WEB_BIND_IP", "127.0.0.1")
    if bind_ip not in {"127.0.0.1", "::1"}:
        warning("Web 当前不是回环绑定，请确认防火墙没有直接暴露 8838 端口")
    if env.get("SAKURA_DOCS_ENABLED", "false").lower() == "true":
        warning("生产环境已开启 API 文档")
    if env.get("SAKURA_LEGACY_API_ENABLED", "false").lower() == "true":
        warning("生产环境已开启旧兼容 API")
    try:
        event_poll_seconds = float(env.get("SAKURA_EVENT_POLL_SECONDS", "1"))
        if not 0.2 <= event_poll_seconds <= 30:
            error("SAKURA_EVENT_POLL_SECONDS 必须在 0.2 到 30 之间")
    except ValueError:
        error("SAKURA_EVENT_POLL_SECONDS 必须是数字")

    required_config = (
        "bot_name",
        "bot_token",
        "owner_api",
        "owner_hash",
        "owner",
        "emby_api",
        "emby_url",
    )
    for key in required_config:
        if _placeholder(config.get(key)):
            error(f"config.json 的 {key} 未填写或仍是示例值")
    if not isinstance(config.get("owner_api"), int) or config.get("owner_api", 0) <= 0:
        error("config.json 的 owner_api 必须是正整数")
    if not isinstance(config.get("owner"), int) or config.get("owner", 0) <= 0:
        error("config.json 的 owner 必须是正整数")
    if not re.fullmatch(r"\d{5,}:[A-Za-z0-9_-]{20,}", str(config.get("bot_token", ""))):
        error("config.json 的 bot_token 格式无效")
    if not re.fullmatch(r"[0-9a-fA-F]{32}", str(config.get("owner_hash", ""))):
        error("config.json 的 owner_hash 应为 32 位 API Hash")
    if config.get("emby_api") and len(str(config["emby_api"])) < 8:
        error("config.json 的 emby_api 长度异常")
    emby_url = urlparse(str(config.get("emby_url", "")))
    if emby_url.scheme not in {"http", "https"} or not emby_url.hostname:
        error("config.json 的 emby_url 不是有效地址")

    return findings


def main() -> int:
    parser = argparse.ArgumentParser(description="检查 Sakura EmbyBoss 上线配置")
    parser.add_argument("--env-file", default=".env")
    parser.add_argument("--config", default="config.json")
    parser.add_argument(
        "--allow-latest",
        action="store_true",
        help="不提示 latest 镜像标签",
    )
    args = parser.parse_args()
    try:
        env = parse_env(Path(args.env_file))
        config = json.loads(Path(args.config).read_text(encoding="utf-8"))
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"✗ 无法读取配置：{exc}")
        return 2

    findings = validate(env, config, allow_latest=args.allow_latest)
    for finding in findings:
        marker = "[ERROR]" if finding.level == "error" else "[WARN]"
        print(f"{marker} {finding.message}")
    errors = [item for item in findings if item.level == "error"]
    if errors:
        print(f"\n上线检查未通过：{len(errors)} 个错误")
        return 1
    print("\n[OK] 上线配置检查通过")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
