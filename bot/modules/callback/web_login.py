from pyrogram import filters
from starlette.concurrency import run_in_threadpool

from backend.dependencies import get_auth_service
from bot import bot


@bot.on_callback_query(filters.regex(r"^web-login-(approve|reject):"))
async def web_login_decision(_, call):
    action, request_id = call.data.split(":", 1)
    approve = action == "web-login-approve"
    result = await run_in_threadpool(
        get_auth_service().decide_telegram_login,
        request_id=request_id,
        tg=call.from_user.id,
        approve=approve,
        display_name=call.from_user.first_name,
    )
    if not result.ok:
        messages = {
            "expired": "登录请求已经过期",
            "identity_mismatch": "这不是你的登录请求",
            "approved": "登录请求已经确认",
            "rejected": "登录请求已经拒绝",
            "consumed": "登录请求已经使用",
        }
        return await call.answer(
            messages.get(result.status, "登录请求不存在或状态异常"),
            show_alert=True,
        )

    if approve:
        await call.message.edit(
            "✅ **已确认 Web 登录**\n\n请返回浏览器，页面会自动完成登录。"
        )
        return await call.answer("登录已确认")
    await call.message.edit(
        "❌ **已拒绝 Web 登录**\n\n如果这不是你发起的操作，建议检查账号安全。"
    )
    return await call.answer("登录已拒绝")
