"""Compatibility launcher for running the Web API beside the Bot.

New deployments should run ``python -m backend.main`` as a separate process.
The explicit scheduler below keeps existing single-process installations
working without causing import-time side effects in the standalone API.
"""

import asyncio
import os

from bot import LOGGER, api as config_api


class Web:
    def __init__(self):
        self.app = None
        self.server = None
        self.task = None

    async def start(self):
        embedded_enabled = os.getenv("SAKURA_EMBEDDED_WEB_ENABLED")
        enabled = (
            config_api.status
            if embedded_enabled is None
            else embedded_enabled.strip().lower() in {"1", "true", "yes", "on"}
        )
        if not enabled:
            LOGGER.info("【Web API】未启用，跳过内嵌服务")
            return
        try:
            from backend.app import app
        except Exception as error:
            LOGGER.error(f"【Web API】配置或初始化失败: {error}")
            return
        import uvicorn

        self.app = app
        settings = app.state.settings
        self.server = uvicorn.Server(
            uvicorn.Config(
                self.app,
                host=settings.host,
                port=settings.port,
                log_level="info",
            )
        )
        LOGGER.warning(
            "【Web API】当前以内嵌兼容模式运行；生产环境建议使用 python -m backend.main 独立启动"
        )
        await self.server.serve()

    def schedule(self):
        if self.task and not self.task.done():
            return self.task
        loop = asyncio.get_event_loop()
        self.task = loop.create_task(self.start(), name="sakura-embedded-web")
        return self.task

    def stop(self):
        if self.server:
            self.server.should_exit = True


web = Web()


def schedule_embedded_web():
    return web.schedule()
