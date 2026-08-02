import type { OperationId } from './generated/client'

export type NavItem = {
  path: string
  label: string
  eyebrow: string
  description: string
  operation?: OperationId
  permission?: string
  area: 'user' | 'admin'
}

export const navigation: NavItem[] = [
  { path: '/', label: '概览', eyebrow: '今日概览', description: '会员、余额、线路与进行中的服务状态', area: 'user' },
  { path: '/emby', label: '站点账号', eyebrow: '我的 Emby', description: '管理多站点账号、开通进度和认领关系', operation: 'listMyEmbyBindings', area: 'user' },
  { path: '/media', label: '影片与求片', eyebrow: '内容中心', description: '直接搜索 TMDB，并发起或跟踪求片', area: 'user' },
  { path: '/wallet', label: '钱包与账单', eyebrow: '我的资产', description: '余额、充值、会员商品和不可变账本', operation: 'listMyRechargeOrders', area: 'user' },
  { path: '/playback', label: '播放记录', eyebrow: '观看足迹', description: '当前播放会话与历史记录', operation: 'listMyPlaybackHistory', area: 'user' },
  { path: '/devices', label: '设备管理', eyebrow: '登录设备', description: '查看客户端、设备画像和风险状态', operation: 'listMyDevices', area: 'user' },
  { path: '/tickets', label: '工单中心', eyebrow: '服务支持', description: '创建工单并持续跟踪回复', operation: 'listMyTickets', area: 'user' },
  { path: '/reviews', label: '影评社区', eyebrow: '社区内容', description: '浏览与发布影片评价', operation: 'listMyReviews', area: 'user' },
  { path: '/notifications', label: '通知中心', eyebrow: '消息与偏好', description: '查看站内消息并配置投递渠道', operation: 'listMyNotifications', area: 'user' },
  { path: '/account', label: '账号与绑定', eyebrow: '统一身份', description: '查看本地身份、角色并绑定 Telegram', operation: 'getCurrentAccount', area: 'user' },
  { path: '/admin', label: '运营仪表盘', eyebrow: '管理中心', description: '实时任务、风险与自动化执行概览', permission: 'dashboard.read', area: 'admin' },
  { path: '/admin/accounts', label: '账号管理', eyebrow: '统一账号', description: '身份、状态、角色、标签与账号生命周期', operation: 'listAccounts', permission: 'accounts.read', area: 'admin' },
  { path: '/admin/memberships', label: '会员与邀请码', eyebrow: '会员运营', description: '方案、商品、邀请码和批量会员调整', operation: 'listMembershipPlans', permission: 'memberships.read', area: 'admin' },
  { path: '/admin/emby', label: 'Emby 实例', eyebrow: '多实例管理', description: '站点、远端账号、绑定、快照与同步任务', operation: 'listEmbyInstances', permission: 'emby_instances.read', area: 'admin' },
  { path: '/admin/commerce', label: '交易中心', eyebrow: '资金与账本', description: '商品、充值订单、退款和异常核对', operation: 'listRechargeOrders', permission: 'commerce.orders.read', area: 'admin' },
  { path: '/admin/batches', label: '批量运营', eyebrow: '运营任务', description: '标签、会员、通知批次的暂停、重试与审计', operation: 'listBatchOperations', permission: 'batch_operations.read', area: 'admin' },
  { path: '/admin/playback', label: '播放与设备', eyebrow: '会话治理', description: '在线播放、历史、设备画像和访问规则', operation: 'listOnlinePlayback', permission: 'playback.read', area: 'admin' },
  { path: '/admin/risk', label: '风险中心', eyebrow: '安全治理', description: '风险规则、事件、处置、撤销与 Telegram 告警', operation: 'listRiskEvents', permission: 'risk.read', area: 'admin' },
  { path: '/admin/media', label: '影片与求片', eyebrow: '内容运营', description: 'TMDB、Emby 匹配、求片和 MoviePilot 联动', operation: 'listMediaRequests', permission: 'media_requests.read', area: 'admin' },
  { path: '/admin/support', label: '工单与影评', eyebrow: '社区运营', description: '工单处理、内部备注和影评审核', operation: 'listTickets', permission: 'tickets.read', area: 'admin' },
  { path: '/admin/engagement', label: '通知与自动化', eyebrow: '触达中心', description: '广播、偏好、自动化规则和执行记录', operation: 'listBroadcasts', permission: 'broadcasts.read', area: 'admin' },
  { path: '/admin/system', label: '系统与审计', eyebrow: '平台治理', description: '动态设置、凭据、权限、API 客户端和操作日志', operation: 'listDynamicSettings', permission: 'settings.read', area: 'admin' },
]

export function findNavigation(path: string) {
  return navigation.find((item) => item.path === path)
}
