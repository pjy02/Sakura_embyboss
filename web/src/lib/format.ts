const dateFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function formatDate(value?: string | null, fallback = "暂无记录") {
  if (!value) return fallback;
  const normalized = /[zZ]|[+-]\d\d:\d\d$/.test(value) ? value : `${value}Z`;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? value : dateFormatter.format(date);
}

export function formatNumber(value?: number | null) {
  return new Intl.NumberFormat("zh-CN").format(value || 0);
}

export function daysUntil(value?: string | null) {
  if (!value) return null;
  const normalized = /[zZ]|[+-]\d\d:\d\d$/.test(value) ? value : `${value}Z`;
  return Math.ceil((new Date(normalized).getTime() - Date.now()) / 86_400_000);
}

export function initials(name?: string | null, tg?: number) {
  return (name?.trim().slice(0, 2) || String(tg || "SK").slice(-2)).toUpperCase();
}

export function levelLabel(level?: string) {
  return ({ a: "白名单", b: "高级会员", c: "正式会员", d: "普通会员" } as Record<string, string>)[
    level || ""
  ] || "未分级";
}

export function actionLabel(action: string) {
  const labels: Record<string, string> = {
    "auth.telegram.claim": "Telegram 确认登录",
    "auth.telegram.approve": "批准网页登录",
    "auth.telegram.reject": "拒绝网页登录",
    "auth.session.create": "创建登录会话",
    "auth.session.revoke": "退出当前会话",
    "auth.session.revoke_all": "退出全部会话",
    "points.adjust": "调整用户积分",
    "role.assign": "分配管理角色",
    "role.remove": "移除管理角色",
    "playback.stop": "终止播放会话",
    "device.update": "更新设备状态",
    "line.create": "新增线路",
    "line.update": "更新线路",
    "line.probe": "探测线路",
    "billing.product.create": "创建充值商品",
    "billing.product.update": "更新充值商品",
    "billing.order.create": "提交充值订单",
    "billing.order.approve": "确认充值入账",
    "billing.order.reject": "拒绝充值订单",
    "billing.order.cancel": "取消充值订单",
    "ticket.create": "创建工单",
    "ticket.reply": "回复工单",
    "ticket.update": "更新工单",
    "request.create": "提交求片",
    "request.cancel": "取消求片",
    "request.update": "更新求片状态",
    "request.import": "导入 Bot 求片",
    "review.create": "提交影评",
    "review.update": "修改影评",
    "review.delete": "删除影评",
    "review.report": "举报影评",
    "review.moderate": "审核影评",
    "notification.broadcast": "发送通知",
    "role.create": "创建角色",
    "role.update": "更新角色权限",
    "role.delete": "删除角色",
    "security.event.update": "处置风险事件",
    "setting.update": "修改系统设置",
    "setting.rollback": "回滚系统设置",
  };
  return labels[action] || action;
}
