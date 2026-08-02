"""
对用户分数调整
score +-

对用户sakura_b币调整
coins +-
"""
import asyncio
from pyrogram import filters
from pyrogram.errors import BadRequest
from bot import bot, prefixes, LOGGER, sakura_b
from bot.application import PointService
from bot.application.point_service import MAX_INT_VALUE, MIN_INT_VALUE
from bot.domain import Actor
from bot.func_helper.filters import admins_on_filter
from bot.func_helper.msg_utils import sendMessage, deleteMessage
from bot.func_helper.fix_bottons import group_f


point_service = PointService()


async def get_user_input(msg):
    gm_name = msg.sender_chat.title if msg.sender_chat else f'管理员 [{msg.from_user.first_name}]({msg.from_user.id})'
    if msg.reply_to_message is None:
        try:
            uid = int(msg.command[1])
            b = int(msg.command[2])
            first = await bot.get_chat(uid)
        except (IndexError, KeyError, BadRequest, ValueError, AttributeError):
            await deleteMessage(msg)
            return None, None, None, gm_name
    else:
        try:
            first = msg.reply_to_message.from_user
            uid = first.id
            b = int(msg.command[1])
        except (IndexError, ValueError, AttributeError):
            await deleteMessage(msg)
            return None, None, None, gm_name
    return uid, b, first, gm_name


@bot.on_message(filters.command('score', prefixes=prefixes) & admins_on_filter)
async def score_user(_, msg):
    uid, b, first, gm_name = await get_user_input(msg)
    if not first:
        return await sendMessage(msg,
                                 "🔔 **使用格式：**[命令符]score [id] [加减分数]\n\n或回复某人[命令符]score [+/-分数] 请确认对象正确",
                                 timer=60)
    result = point_service.adjust(
        tg=uid,
        amount=b,
        balance_type="registration_days",
        reason="admin_score_adjustment",
        actor=Actor.telegram(msg.from_user.id if msg.from_user else msg.sender_chat.id, gm_name),
        allow_negative=True,
        idempotency_key=f"telegram:{msg.chat.id}:{msg.id}:score",
    )
    if result.status == "user_not_found":
        return await sendMessage(msg, f"数据库中没有[ta](tg://user?id={uid}) 。请先私聊我", buttons=group_f)
    if result.status == "overflow":
        return await sendMessage(msg, f"❌ 操作失败！计算结果超出安全范围（{MIN_INT_VALUE} 到 {MAX_INT_VALUE}）。", timer=60)
    if result.ok:
        balance = result.data["balance"]
        await asyncio.gather(sendMessage(msg,
                                         f"· 🎯 {gm_name} 调节了 [{first.first_name}](tg://user?id={uid}) 积分： {b}"
                                         f"\n· 🎟️ 实时积分: **{balance}**"),
                             msg.delete())
        LOGGER.info(f"【admin】[积分]：{gm_name} 对 {first.first_name}-{uid}  {b}分  ")
    else:
        await sendMessage(msg, '⚠️ 数据库操作失败，请检查')
        LOGGER.info(f"【admin】[积分]：{gm_name} 对 {first.first_name}-{uid} 数据操作失败")


@bot.on_message(filters.command('coins', prefixes=prefixes) & admins_on_filter)
async def coins_user(_, msg):
    uid, b, first, gm_name = await get_user_input(msg)
    if not first:
        return await sendMessage(msg,
                                 "🔔 **使用格式：**[命令符]coins [id] [+/-币]\n\n或回复某人[命令符]coins [+/-币] 请确认对象正确",
                                 timer=60)

    result = point_service.adjust(
        tg=uid,
        amount=b,
        balance_type="coins",
        reason="admin_coin_adjustment",
        actor=Actor.telegram(msg.from_user.id if msg.from_user else msg.sender_chat.id, gm_name),
        allow_negative=True,
        idempotency_key=f"telegram:{msg.chat.id}:{msg.id}:coins",
    )
    if result.status == "user_not_found":
        return await sendMessage(msg, f"数据库中没有[ta](tg://user?id={uid}) 。请先私聊我", buttons=group_f)
    if result.status == "overflow":
        return await sendMessage(msg, f"❌ 操作失败！计算结果超出安全范围（{MIN_INT_VALUE} 到 {MAX_INT_VALUE}）。", timer=60)
    if result.ok:
        balance = result.data["balance"]
        await asyncio.gather(sendMessage(msg,
                                         f"· 🎯 {gm_name} 调节了 [{first.first_name}](tg://user?id={uid}) {sakura_b}： {b}"
                                         f"\n· 🎟️ 实时{sakura_b}: **{balance}**"),
                             msg.delete())
        LOGGER.info(
            f"【admin】[{sakura_b}]- {gm_name} 对 {first.first_name}-{uid}  {b}{sakura_b}")
    else:
        await sendMessage(msg, '⚠️ 数据库操作失败，请检查')
        LOGGER.info(f"【admin】[{sakura_b}]：{gm_name} 对 {first.first_name}-{uid} 数据操作失败")
