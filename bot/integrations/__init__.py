"""External integration gateways shared by Web, Bot and standalone workers."""

from .telegram_gateway import TelegramGateway, TelegramMessage

__all__ = ["TelegramGateway", "TelegramMessage"]
