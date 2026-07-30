import asyncio
import random
from datetime import datetime, timezone, timedelta

from pyrogram import filters

from bot import bot, _open, sakura_b
from bot.application import PointService
from bot.domain import Actor
from bot.func_helper.filters import user_in_group_on_filter
from bot.func_helper.msg_utils import callAnswer, sendMessage, deleteMessage


point_service = PointService()


@bot.on_callback_query(filters.regex('checkin') & user_in_group_on_filter)
async def user_in_checkin(_, call):
    now = datetime.now(timezone(timedelta(hours=8)))
    today = now.strftime("%Y-%m-%d")
    if _open.checkin:
        reward = random.randint(_open.checkin_reward[0], _open.checkin_reward[1])
        result = point_service.check_in(
            tg=call.from_user.id,
            reward=reward,
            occurred_at=now,
            actor=Actor.telegram(call.from_user.id, call.from_user.first_name),
            maximum_level=_open.checkin_lv,
            idempotency_key=f"telegram:{call.from_user.id}:checkin:{today}",
        )
        if result.status == "user_not_found":
            await callAnswer(call, '🧮 未查询到数据库', True)
        elif result.status == "level_denied":
            await callAnswer(call, '❌ 您无权签到，如有异议，请不要有异议。', True)
        elif result.ok:
            balance = result.data["balance"]
            actual_reward = result.data["reward"]
            text = f'🎉 **签到成功** | {actual_reward} {sakura_b}\n💴 **当前持有** | {balance} {sakura_b}\n⏳ **签到日期** | {today}'
            await asyncio.gather(deleteMessage(call), sendMessage(call, text=text))
        elif result.status == "already_checked_in":
            await callAnswer(call, '⭕ 您今天已经签到过了！签到是无聊的活动哦。', True)
        else:
            await callAnswer(call, '⚠️ 签到失败，请稍后重试。', True)
    else:
        await callAnswer(call, '❌ 未开启签到功能，等待！', True)
