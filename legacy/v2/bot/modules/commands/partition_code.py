from datetime import datetime


from bot import LOGGER, partition_libs
from bot.application import PartitionService
from bot.domain import Actor, secret_fingerprint
from bot.func_helper.emby import emby
from bot.sql_helper.sql_partition import sql_get_partition_code


partition_service = PartitionService()


async def _redeem_partition_code(code: str, tg_id: int):
    now = datetime.now()

    record = sql_get_partition_code(code)
    if not record:
        return False, "❌ 分区码无效。"

    libs = partition_libs.get(record.partition, []) if partition_libs else []
    if not libs:
        LOGGER.warning("分区码对应分区未配置库: %s", record.partition)
        return False, "⚠️ 分区未配置库，请联系管理员。"

    actor = Actor.telegram(tg_id)
    reservation = partition_service.reserve_code(
        code_value=code,
        tg=tg_id,
        actor=actor,
        now=now,
    )
    if reservation.status == "no_account":
        return False, "⚠️ 未找到您的 Emby 账户，请先完成注册绑定。"
    if reservation.status == "code_busy":
        return False, "⏳ 分区码正在处理中，请稍后重试。"
    if not reservation.ok:
        return False, "❌ 分区码无效或已被使用。"

    reservation_token = reservation.data["reservation_token"]
    embyid = reservation.data["embyid"]
    remote_ok = await emby.show_folders_by_names(embyid, libs)
    if not remote_ok:
        partition_service.release_reservation(
            reservation_token=reservation_token,
            tg=tg_id,
            actor=actor,
            reason="emby_show_folders_failed",
        )
        return False, "⚠️ Emby 媒体库授权失败，分区码未消耗，请稍后重试。"

    completed = partition_service.complete_redemption(
        reservation_token=reservation_token,
        tg=tg_id,
        actor=actor,
        now=now,
    )
    if not completed.ok:
        LOGGER.error(
            "分区码远程授权成功但数据库核销失败: tg=%s code_fingerprint=%s reservation=%s",
            tg_id,
            secret_fingerprint(code),
            reservation_token,
        )
        return False, "⚠️ 媒体库已授权，但记录确认失败，请联系管理员处理。"

    partition = completed.data["partition"]
    expires_at = completed.data["expires_at"]
    if isinstance(expires_at, str):
        expires_at = datetime.fromisoformat(expires_at)
    libs_text = "、".join(libs)
    return (
        True,
        f"✅ 已激活分区 {partition}\n"
        f"已激活媒体库：{libs_text}\n"
        f"可访问至：{expires_at:%Y-%m-%d %H:%M:%S}",
    )
