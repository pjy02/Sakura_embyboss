import os
from typing import Any, Awaitable, Callable


TaskHandler = Callable[[dict], Awaitable[dict]]


async def sync_favorites_handler(_payload: dict) -> dict:
    from bot.scheduler.sync_favorites import sync_favorites

    result = await sync_favorites()
    return result if isinstance(result, dict) else {"completed": True}


async def sync_moviepilot_handler(_payload: dict) -> dict:
    from bot.scheduler.sync_mp_download import sync_download_tasks

    result = await sync_download_tasks()
    return result if isinstance(result, dict) else {"completed": True}


async def partition_access_handler(_payload: dict) -> dict:
    from bot.scheduler.partition_access import check_partition_access

    result = await check_partition_access()
    return result if isinstance(result, dict) else {"completed": True}


async def expired_accounts_handler(_payload: dict) -> dict:
    from bot.scheduler.check_ex import check_expired

    result = await check_expired()
    return result if isinstance(result, dict) else {"completed": True}


async def backup_database_handler(_payload: dict) -> dict:
    from bot.scheduler.backup_db import DbBackupUtils

    backup_file = await DbBackupUtils.backup_db()
    if not backup_file:
        raise RuntimeError("Database backup did not produce an output file")
    return {
        "completed": True,
        "filename": os.path.basename(str(backup_file)),
    }


async def registration_account_handler(payload: dict) -> dict:
    from bot.application.registration_service import RegistrationService

    result = await RegistrationService().execute(payload)
    if payload.get("channel") == "telegram":
        await _notify_registration_result(payload, result)
    return result


async def diagnostics_handler(_payload: dict) -> dict:
    from bot.application.operations_center_service import DiagnosticService

    return await DiagnosticService().run()


async def telegram_alert_handler(payload: dict) -> dict:
    from bot.application.operations_center_service import AlertService

    return await AlertService().deliver(str(payload["alert_id"]))


async def telegram_notification_handler(payload: dict) -> dict:
    from bot import bot

    title = str(payload.get("title") or "Sakura 通知")[:200]
    body = str(payload.get("body") or "")[:2000]
    if not body:
        raise ValueError("Telegram 通知正文不能为空")
    await bot.send_message(int(payload["tg"]), f"🔔 {title}\n\n{body}", parse_mode=None)
    return {"ok": True, "recipient_tg": int(payload["tg"])}


async def users_batch_handler(payload: dict) -> dict:
    from bot.application.operations_center_service import AccountLifecycleService

    return await AccountLifecycleService().execute_batch(payload)


async def _notify_registration_result(payload: dict, result: dict) -> None:
    chat_id = payload.get("notification_chat_id")
    message_id = payload.get("notification_message_id")
    if not chat_id:
        return
    if result.get("ok"):
        text = (
            "账号创建成功\n\n"
            f"用户名：{result.get('username')}\n"
            f"Emby 密码：{result.get('emby_password')}\n"
            f"安全码：{payload.get('safety_code')}\n"
            f"到期时间：{result.get('expires_at')}\n\n"
            "请妥善保存密码，之后可在 Bot 或 Web 用户中心管理账号。"
        )
    else:
        text = (
            "账号创建失败\n\n"
            f"{result.get('message') or '请稍后重新提交注册任务。'}"
        )
    try:
        from bot import bot

        if message_id:
            await bot.edit_message_text(
                int(chat_id),
                int(message_id),
                text,
                parse_mode=None,
            )
        else:
            await bot.send_message(int(chat_id), text, parse_mode=None)
    except Exception:
        try:
            from bot import bot

            await bot.send_message(int(chat_id), text, parse_mode=None)
        except Exception:
            pass


TASK_HANDLERS: dict[str, TaskHandler] = {
    "sync.favorites": sync_favorites_handler,
    "sync.moviepilot": sync_moviepilot_handler,
    "maintenance.partition_access": partition_access_handler,
    "maintenance.expired_accounts": expired_accounts_handler,
    "maintenance.backup_database": backup_database_handler,
    "registration.account": registration_account_handler,
    "monitor.diagnostics": diagnostics_handler,
    "alert.telegram": telegram_alert_handler,
    "notification.telegram": telegram_notification_handler,
    "users.batch": users_batch_handler,
}
