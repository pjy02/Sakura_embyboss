from bot.application import MediaRequestService
from bot import LOGGER, bot, config
from bot.func_helper.moviepilot import (
    get_download_task,
    get_history_transfer_task_by_title_download_id,
)
from bot.func_helper.scheduler import scheduler
from bot.sql_helper.sql_request_record import (
    sql_get_request_record_by_download_id,
    sql_get_request_record_by_transfer_state,
    sql_update_request_status,
)


media_request_service = MediaRequestService()


async def sync_download_tasks():
    """同步MoviePilot下载任务状态到数据库"""
    try:
        # 获取所有下载任务
        download_tasks = await get_download_task()
        download_count = 0
        if download_tasks is not None:
            # 更新每个任务的状态
            for task in download_tasks:
                download_id = task['download_id']
                download_state = task['state']
                progress = task['progress']
                left_time = task.get('left_time', '未知')
                record = sql_get_request_record_by_download_id(download_id)
                synced = media_request_service.sync_download(
                    download_id,
                    download_state=download_state,
                    transfer_state=None,
                    progress=progress,
                )
                if record is None and synced is None:
                    continue

                # 保留原 Bot 请求表兼容，同时新求片表已在上方同步。
                if record is not None and download_state == 'downloading':
                    sql_update_request_status(
                        download_id=download_id,
                        download_state='downloading',
                        progress=progress,
                        left_time=left_time
                    )
                elif record is not None and download_state == 'completed':
                    sql_update_request_status(
                        download_id=download_id,
                        download_state='completed',
                        progress=100,
                        left_time='0'
                    )
                elif record is not None and download_state == 'failed':
                    sql_update_request_status(
                        download_id=download_id,
                        download_state='failed',
                        progress=progress,
                        left_time='失败'
                    )
                elif record is not None and download_state == 'pending':
                    sql_update_request_status(
                        download_id=download_id,
                        download_state='pending',
                        progress=0,
                        left_time='等待中'
                    )
                download_count += 1

        # 合并旧 Bot 记录和新 Web 求片记录，按下载 ID 去重检查入库状态。
        transfer_candidates = {}
        for record in sql_get_request_record_by_transfer_state() or []:
            transfer_candidates[record.download_id] = {
                "legacy": record,
                "tg": record.tg,
                "title": record.request_name,
            }
        for item in media_request_service.transfer_candidates():
            if not item["download_id"]:
                continue
            transfer_candidates.setdefault(
                item["download_id"],
                {
                    "legacy": None,
                    "tg": item["tg"],
                    "title": item["title"],
                },
            )

        transfer_count = 0
        for download_id, candidate in transfer_candidates.items():
            transfer_state = await get_history_transfer_task_by_title_download_id(
                "",
                download_id,
                count=100,
            )
            if transfer_state is None:
                continue
            transfer_text = str(transfer_state).lower()
            transfer_succeeded = transfer_text in {
                "true",
                "1",
                "success",
                "completed",
            }
            if transfer_succeeded:
                try:
                    await bot.send_message(
                        chat_id=candidate["tg"],
                        text=f"💯恭喜您点播的「{candidate['title']}」已成功入库！",
                    )
                except Exception as exc:
                    LOGGER.error(
                        f"[MoviePilot] 发送通知到{candidate['tg']}失败: {str(exc)}"
                    )
            if candidate["legacy"] is not None:
                sql_update_request_status(
                    download_id=download_id,
                    transfer_state=transfer_state,
                    download_state='completed',
                    progress=100,
                    left_time='0',
                )
            media_request_service.sync_download(
                download_id,
                download_state='completed',
                transfer_state=transfer_state,
                progress=100,
            )
            transfer_count += 1
        if download_count > 0 or transfer_count > 0:
            LOGGER.info(f"[MoviePilot] 同步了 {download_count} 个下载任务状态, {transfer_count} 个转移任务状态")
        return {
            "download_tasks": download_count,
            "transfer_tasks": transfer_count,
        }
    except Exception as e:
        LOGGER.error(f"[MoviePilot] 同步下载任务状态时出错: {str(e)}")
        raise


# 如果MoviePilot功能开启，添加定时任务
if config.moviepilot.status:
    scheduler.add_job(sync_download_tasks, 'interval',
                     seconds=60, id='sync_download_tasks')
