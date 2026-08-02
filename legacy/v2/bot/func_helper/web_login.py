from pyrogram.types import InlineKeyboardButton, InlineKeyboardMarkup
from starlette.concurrency import run_in_threadpool

from backend.dependencies import get_auth_service
from bot.func_helper.msg_utils import sendMessage


async def handle_web_login_start(
    message,
    raw_token: str,
    expected_purpose: str = "login",
):
    result = await run_in_threadpool(
        get_auth_service().claim_telegram_login,
        raw_token=raw_token,
        tg=message.from_user.id,
        display_name=message.from_user.first_name,
        expected_purpose=expected_purpose,
    )
    messages = {
        "invalid_request": "❌ 登录请求不存在，请从网页重新发起。",
        "expired": "⌛ 登录请求已过期，请从网页重新发起。",
        "identity_mismatch": "🚫 此登录请求指定了其他 Telegram 账号。",
        "user_not_found": "❌ 当前 Telegram 账号尚未登记，无法登录用户中心。",
        "approved": "✅ 此登录请求已经确认。",
        "rejected": "❌ 此登录请求已经被拒绝。",
        "consumed": "ℹ️ 此登录请求已经使用。",
    }
    if not result.ok:
        return await sendMessage(
            message,
            messages.get(result.status, "⚠️ 登录请求状态异常，请重新发起。"),
            timer=60,
        )

    request_id = result.data["request_id"]
    is_registration = expected_purpose == "registration"
    keyboard = InlineKeyboardMarkup(
        [
            [
                InlineKeyboardButton(
                    "✅ 确认注册" if is_registration else "✅ 确认登录",
                    callback_data=f"web-login-approve:{request_id}",
                ),
                InlineKeyboardButton(
                    "❌ 拒绝",
                    callback_data=f"web-login-reject:{request_id}",
                ),
            ]
        ]
    )
    text = (
        "🌸 **Web 注册身份确认**\n\n"
        "浏览器正在申请使用当前 Telegram 身份注册 Sakura Emby 账号。\n"
        "如果这是你本人发起的操作，请点击“确认注册”；否则请拒绝。"
        if is_registration
        else
        "🔐 **Web 登录确认**\n\n"
        "有人正在浏览器中登录 Sakura 管理系统。\n"
        "如果这是你本人发起的操作，请点击“确认登录”；否则请拒绝。"
    )
    return await message.reply(
        text,
        reply_markup=keyboard,
    )
