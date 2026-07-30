import os
import re
import secrets
from dataclasses import dataclass
from functools import lru_cache
from typing import Optional

from bot import LOGGER, admins, api as config_api, bot_name, owner


_PATH_PATTERN = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_-]{2,63}$")
_FORBIDDEN_ADMIN_PATHS = {
    "admin",
    "api",
    "app",
    "dashboard",
    "docs",
    "manage",
    "openapi.json",
    "redoc",
}


def _env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int, minimum: int, maximum: int) -> int:
    raw = os.getenv(name)
    value = int(raw) if raw is not None else int(default)
    return max(minimum, min(maximum, value))


def _normalize_path(value: str, *, admin: bool = False) -> str:
    normalized = value.strip().strip("/")
    if not _PATH_PATTERN.fullmatch(normalized):
        raise ValueError(
            "Web paths must contain 3-64 letters, digits, underscores or hyphens"
        )
    if admin and normalized.lower() in _FORBIDDEN_ADMIN_PATHS:
        raise ValueError(
            "WEB_ADMIN_PATH must not use a common or reserved management path"
        )
    return normalized


@dataclass(frozen=True)
class WebSettings:
    host: str
    port: int
    admin_path: str
    user_path: str
    public_base_url: Optional[str]
    cookie_secure: bool
    cookie_name: str
    csrf_cookie_name: str
    session_ttl_hours: int
    login_ttl_seconds: int
    session_secret: str
    cors_origins: tuple[str, ...]
    trusted_hosts: tuple[str, ...]
    docs_enabled: bool
    legacy_api_enabled: bool
    legacy_api_token: Optional[str]
    owner_tg: int
    admin_tg_ids: tuple[int, ...]
    bot_username: str


@lru_cache(maxsize=1)
def get_settings() -> WebSettings:
    cookie_secure = _env_bool("SAKURA_COOKIE_SECURE", config_api.cookie_secure)
    configured_secret = os.getenv("SAKURA_WEB_SESSION_SECRET")
    if configured_secret:
        session_secret = configured_secret
    else:
        if cookie_secure:
            raise ValueError(
                "SAKURA_WEB_SESSION_SECRET is required when secure cookies are enabled"
            )
        session_secret = secrets.token_urlsafe(48)
        LOGGER.warning(
            "SAKURA_WEB_SESSION_SECRET 未配置，正在使用仅当前进程有效的开发密钥"
        )

    admin_path = _normalize_path(
        os.getenv("WEB_ADMIN_PATH", config_api.admin_path),
        admin=True,
    )
    user_path = _normalize_path(
        os.getenv("WEB_USER_PATH", config_api.user_path),
        admin=False,
    )
    if admin_path == user_path:
        raise ValueError("WEB_ADMIN_PATH and WEB_USER_PATH must be different")

    origins = tuple(
        str(item).rstrip("/")
        for item in (config_api.allow_origins or [])
        if str(item) != "*"
    )
    trusted_hosts = tuple(str(item) for item in (config_api.trusted_hosts or ["*"]))
    legacy_token = os.getenv("SAKURA_LEGACY_API_TOKEN")
    legacy_enabled = _env_bool(
        "SAKURA_LEGACY_API_ENABLED",
        config_api.legacy_api_enabled,
    )
    if legacy_enabled and not legacy_token:
        raise ValueError(
            "SAKURA_LEGACY_API_TOKEN is required when legacy API compatibility is enabled"
        )
    if legacy_enabled and len(legacy_token or "") < 24:
        raise ValueError("SAKURA_LEGACY_API_TOKEN must contain at least 24 characters")

    return WebSettings(
        host=os.getenv("SAKURA_WEB_HOST", config_api.http_url or "0.0.0.0"),
        port=_env_int(
            "SAKURA_WEB_PORT",
            int(config_api.http_port or 8838),
            1,
            65535,
        ),
        admin_path=admin_path,
        user_path=user_path,
        public_base_url=(
            os.getenv("SAKURA_PUBLIC_BASE_URL")
            or config_api.public_base_url
            or None
        ),
        cookie_secure=cookie_secure,
        cookie_name=os.getenv("SAKURA_SESSION_COOKIE", "sakura_session"),
        csrf_cookie_name=os.getenv("SAKURA_CSRF_COOKIE", "sakura_csrf"),
        session_ttl_hours=_env_int(
            "SAKURA_SESSION_TTL_HOURS",
            config_api.session_ttl_hours,
            1,
            24 * 30,
        ),
        login_ttl_seconds=_env_int(
            "SAKURA_LOGIN_TTL_SECONDS",
            config_api.login_ttl_seconds,
            60,
            900,
        ),
        session_secret=session_secret,
        cors_origins=origins,
        trusted_hosts=trusted_hosts,
        docs_enabled=_env_bool("SAKURA_DOCS_ENABLED", config_api.docs_enabled),
        legacy_api_enabled=legacy_enabled,
        legacy_api_token=legacy_token,
        owner_tg=int(owner),
        admin_tg_ids=tuple(int(item) for item in admins),
        bot_username=bot_name.lstrip("@"),
    )
