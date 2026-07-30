import json
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import APIRouter, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, HTMLResponse, JSONResponse, Response
from sqlalchemy import text
from starlette.middleware.trustedhost import TrustedHostMiddleware

from backend.api import (
    admin_router,
    auth_router,
    events_router,
    me_router,
    tasks_router,
)
from backend.event_relay import EventRelay
from backend.middleware import SecurityHeadersMiddleware
from backend.settings import WebSettings, get_settings
from bot.sql_helper import Session
from bot.application import ReliabilityService


PROJECT_ROOT = Path(__file__).resolve().parents[1]
WEB_DIST_ROOT = PROJECT_ROOT / "web" / "dist"


def _placeholder_html(title: str, area: str) -> str:
    return f"""<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title}</title></head>
<body><main><h1>{title}</h1><p>{area}前端资源尚未构建，请先执行 Web 构建。</p></main></body>
</html>"""


def _spa_response(area: str, asset_path: str = ""):
    root = (WEB_DIST_ROOT / area).resolve()
    requested = (root / asset_path).resolve() if asset_path else root / "index.html"
    if root not in requested.parents and requested != root:
        raise HTTPException(status_code=404)
    if asset_path and requested.is_file():
        return FileResponse(requested)
    index = root / "index.html"
    if index.is_file():
        return FileResponse(index, media_type="text/html")
    title = "Sakura 管理中心" if area == "admin" else "Sakura 用户中心"
    return HTMLResponse(_placeholder_html(title, title))


def _runtime_script(settings: WebSettings, area: str) -> Response:
    content = {
        "apiBase": "/api/v1",
        "area": area,
        "basePath": (
            f"/{settings.admin_path}" if area == "admin" else f"/{settings.user_path}"
        ),
        "portalPath": f"/{settings.user_path}",
        "adminPath": f"/{settings.admin_path}",
        "botUsername": settings.bot_username,
        "csrfCookieName": settings.csrf_cookie_name,
    }
    return Response(
        content=f"window.__SAKURA_CONFIG__={json.dumps(content)};",
        media_type="application/javascript",
        headers={"Cache-Control": "no-store"},
    )


def create_app(settings: WebSettings | None = None) -> FastAPI:
    settings = settings or get_settings()
    relay = EventRelay()

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        await relay.start()
        try:
            yield
        finally:
            await relay.stop()

    app = FastAPI(
        title="Sakura EmbyBoss API",
        version="2.0.0",
        docs_url="/api/docs" if settings.docs_enabled else None,
        redoc_url="/api/redoc" if settings.docs_enabled else None,
        openapi_url="/api/openapi.json" if settings.docs_enabled else None,
        lifespan=lifespan,
    )
    app.state.settings = settings
    app.state.event_relay = relay
    app.add_middleware(SecurityHeadersMiddleware)
    app.add_middleware(
        TrustedHostMiddleware,
        allowed_hosts=list(settings.trusted_hosts),
    )
    if settings.cors_origins:
        app.add_middleware(
            CORSMiddleware,
            allow_origins=list(settings.cors_origins),
            allow_credentials=True,
            allow_methods=["GET", "POST", "PUT", "PATCH", "DELETE"],
            allow_headers=["Content-Type", "X-CSRF-Token", "Idempotency-Key"],
        )

    api_v1 = APIRouter(prefix="/api/v1")
    api_v1.include_router(auth_router)
    api_v1.include_router(me_router)
    api_v1.include_router(admin_router)
    api_v1.include_router(tasks_router)
    api_v1.include_router(events_router)
    app.include_router(api_v1)

    @app.get("/healthz", tags=["health"])
    def health():
        return {"status": "ok"}

    @app.get("/readyz", tags=["health"])
    def readiness():
        try:
            with Session() as session:
                session.execute(text("SELECT 1"))
            reliability = ReliabilityService().status()
            return {
                "status": "ready",
                "components": {
                    "database": "ready",
                    "task_worker": reliability["status"],
                    "event_relay": "running",
                },
            }
        except Exception:
            return JSONResponse(
                status_code=503,
                content={
                    "status": "not_ready",
                    "components": {"database": "unavailable"},
                },
            )

    @app.get(
        f"/{settings.admin_path}/runtime-config.js",
        include_in_schema=False,
    )
    def admin_runtime_config():
        return _runtime_script(settings, "admin")

    @app.get(
        f"/{settings.user_path}/runtime-config.js",
        include_in_schema=False,
    )
    def user_runtime_config():
        return _runtime_script(settings, "portal")

    @app.get(f"/{settings.user_path}", include_in_schema=False)
    @app.get(f"/{settings.user_path}/", include_in_schema=False)
    def user_portal():
        return _spa_response("portal")

    @app.get(f"/{settings.admin_path}", include_in_schema=False)
    @app.get(f"/{settings.admin_path}/", include_in_schema=False)
    def admin_portal():
        return _spa_response("admin")

    @app.get(f"/{settings.user_path}/{{asset_path:path}}", include_in_schema=False)
    def user_portal_assets(asset_path: str):
        return _spa_response("portal", asset_path)

    @app.get(f"/{settings.admin_path}/{{asset_path:path}}", include_in_schema=False)
    def admin_portal_assets(asset_path: str):
        return _spa_response("admin", asset_path)

    def hidden_management_path():
        raise HTTPException(status_code=404)

    for decoy in ("admin", "manage", "dashboard"):
        if decoy not in {settings.admin_path, settings.user_path}:
            app.add_api_route(
                f"/{decoy}",
                hidden_management_path,
                include_in_schema=False,
            )

    if settings.legacy_api_enabled:
        from backend.legacy import create_legacy_router

        app.include_router(create_legacy_router())
    return app


app = create_app()
