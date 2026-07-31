from __future__ import annotations

from typing import Any, Optional

import aiohttp


class TelegramMessage:
    def __init__(self, gateway: "TelegramGateway", payload: dict):
        self._gateway = gateway
        self.raw = payload
        self.chat_id = int((payload.get("chat") or {}).get("id") or 0)
        self.message_id = int(payload.get("message_id") or 0)

    async def forward(self, chat_id: int):
        if not self.chat_id or not self.message_id:
            raise RuntimeError("Telegram 消息缺少转发所需的消息编号")
        result = await self._gateway.request(
            "forwardMessage",
            {
                "chat_id": int(chat_id),
                "from_chat_id": self.chat_id,
                "message_id": self.message_id,
            },
        )
        return TelegramMessage(self._gateway, result)


class TelegramGateway:
    """Small Bot API gateway that does not require an MTProto client process."""

    def __init__(self, token: Optional[str] = None, timeout_seconds: int = 15):
        if token is None:
            from bot import bot_token

            token = bot_token
        self._token = str(token or "").strip()
        self._timeout_seconds = timeout_seconds

    async def request(self, method: str, payload: dict[str, Any]) -> dict:
        if not self._token:
            raise RuntimeError("Telegram Bot Token 未配置")
        timeout = aiohttp.ClientTimeout(total=self._timeout_seconds)
        url = f"https://api.telegram.org/bot{self._token}/{method}"
        async with aiohttp.ClientSession(timeout=timeout) as session:
            async with session.post(url, json=payload) as response:
                try:
                    data = await response.json(content_type=None)
                except Exception as error:
                    text = (await response.text())[:500]
                    raise RuntimeError(
                        f"Telegram Bot API 返回了无效响应：HTTP {response.status} {text}"
                    ) from error
        if response.status >= 400 or not data.get("ok"):
            description = data.get("description") if isinstance(data, dict) else None
            raise RuntimeError(
                f"Telegram Bot API 调用失败：{description or f'HTTP {response.status}'}"
            )
        return data.get("result") or {}

    async def send_message(
        self,
        chat_id: int,
        text: str,
        *,
        parse_mode: Optional[str] = None,
    ) -> TelegramMessage:
        payload: dict[str, Any] = {"chat_id": int(chat_id), "text": str(text)}
        if parse_mode:
            payload["parse_mode"] = parse_mode
        return TelegramMessage(self, await self.request("sendMessage", payload))

    async def edit_message_text(
        self,
        chat_id: int,
        message_id: int,
        text: str,
        *,
        parse_mode: Optional[str] = None,
    ) -> TelegramMessage:
        payload: dict[str, Any] = {
            "chat_id": int(chat_id),
            "message_id": int(message_id),
            "text": str(text),
        }
        if parse_mode:
            payload["parse_mode"] = parse_mode
        return TelegramMessage(self, await self.request("editMessageText", payload))
