from fastapi import APIRouter, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import HTMLResponse, JSONResponse
from sqlalchemy import text
from starlette.middleware.trustedhost import TrustedHostMiddleware

from backend.api import admin_router, auth_router, me_router
from backend.middleware import SecurityHeadersMiddleware
from backend.settings import WebSettings, get_settings
from bot.sql_helper import Session


def _placeholder_html(title: str, area: str) -> str:
    return f"""<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title}</title></head>
<body><main><h1>{title}</h1><p>{area} API 已就绪，前端界面将在下一阶段接入。</p></main></body>
</html>"""


def create_app(settings: WebSettings | None = None) -> FastAPI:
    settings = settings or get_settings()
    app = FastAPI(
        title="Sakura EmbyBoss API",
        version="2.0.0",
        docs_url="/api/docs" if settings.docs_enabled else None,
        redoc_url="/api/redoc" if settings.docs_enabled else None,
        openapi_url="/api/openapi.json" if settings.docs_enabled else None,
    )
    app.state.settings = settings
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
    app.include_router(api_v1)

    @app.get("/healthz", tags=["health"])
    def health():
        return {"status": "ok"}

    @app.get("/readyz", tags=["health"])
    def readiness():
        try:
            with Session() as session:
                session.execute(text("SELECT 1"))
            return {"status": "ready"}
        except Exception:
            return JSONResponse(status_code=503, content={"status": "not_ready"})

    @app.get(f"/{settings.user_path}", response_class=HTMLResponse, include_in_schema=False)
    def user_portal_placeholder():
        return _placeholder_html("Sakura 用户中心", "用户中心")

    @app.get(
        f"/{settings.admin_path}",
        response_class=HTMLResponse,
        include_in_schema=False,
    )
    def admin_placeholder():
        return _placeholder_html("Sakura 管理中心", "管理中心")

    @app.get(
        f"/{settings.admin_path}/runtime-config.js",
        include_in_schema=False,
    )
    def admin_runtime_config():
        content = (
            "window.__SAKURA_CONFIG__="
            '{"apiBase":"/api/v1","area":"admin"};'
        )
        return HTMLResponse(content=content, media_type="application/javascript")

    @app.get(
        f"/{settings.user_path}/runtime-config.js",
        include_in_schema=False,
    )
    def user_runtime_config():
        content = (
            "window.__SAKURA_CONFIG__="
            '{"apiBase":"/api/v1","area":"portal"};'
        )
        return HTMLResponse(content=content, media_type="application/javascript")

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
