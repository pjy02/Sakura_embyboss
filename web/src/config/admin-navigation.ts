import type { Component } from "vue";
import {
  CircleDollarSign,
  Clapperboard,
  FileClock,
  Gauge,
  HardDrive,
  HeartHandshake,
  ListVideo,
  MessageSquareText,
  Network,
  ReceiptText,
  ServerCog,
  Settings2,
  ShieldCheck,
  Star,
  UserCog,
  Users,
  Workflow,
} from "lucide-vue-next";

export interface AdminNavigationItem {
  to: string;
  label: string;
  description: string;
  icon: Component;
  permission?: string;
  disabled?: boolean;
  badge?: string;
  keywords?: string[];
}

export interface AdminNavigationSection {
  label: string;
  items: AdminNavigationItem[];
}

export const adminNavigation: AdminNavigationSection[] = [
  {
    label: "总览",
    items: [
      {
        to: "/",
        label: "仪表盘",
        description: "运营指标、系统状态与最近操作",
        icon: Gauge,
        keywords: ["首页", "概览", "dashboard"],
      },
    ],
  },
  {
    label: "用户运营",
    items: [
      {
        to: "/users",
        label: "站点账号",
        description: "检索用户、账户状态、积分与角色",
        icon: Users,
        permission: "users:read",
        keywords: ["用户", "Emby", "Telegram"],
      },
      {
        to: "/playback/live",
        label: "在线播放",
        description: "查看当前播放会话与线路",
        icon: Clapperboard,
        permission: "playback:read",
      },
      {
        to: "/playback/history",
        label: "播放历史",
        description: "查询历史播放、客户端与 IP",
        icon: ListVideo,
        permission: "playback:read",
      },
      {
        to: "/devices",
        label: "设备管理",
        description: "设备关联、信任状态与风险识别",
        icon: HardDrive,
        permission: "devices:read",
      },
    ],
  },
  {
    label: "线路与系统",
    items: [
      {
        to: "/lines",
        label: "线路管理",
        description: "线路健康、权重、启停与维护状态",
        icon: Network,
        permission: "lines:read",
      },
      {
        to: "/tasks",
        label: "系统任务",
        description: "后台任务、定时作业与 Worker",
        icon: Workflow,
        permission: "tasks:read",
      },
      {
        to: "/system/status",
        label: "服务状态",
        description: "Bot、Web、数据库与外部服务",
        icon: ServerCog,
        disabled: true,
        badge: "规划中",
      },
    ],
  },
  {
    label: "交易与服务",
    items: [
      {
        to: "/billing/recharge",
        label: "充值中心",
        description: "充值产品、支付订单与人工补单",
        icon: CircleDollarSign,
        permission: "billing:read",
      },
      {
        to: "/billing/ledger",
        label: "账单记录",
        description: "支付、积分、退款与冲正流水",
        icon: ReceiptText,
        permission: "billing:read",
      },
      {
        to: "/tickets",
        label: "工单管理",
        description: "用户问题、分派、回复与 SLA",
        icon: HeartHandshake,
        permission: "tickets:read",
      },
    ],
  },
  {
    label: "内容社区",
    items: [
      {
        to: "/requests",
        label: "求片订阅",
        description: "审核求片并同步 MoviePilot 状态",
        icon: MessageSquareText,
        permission: "requests:read",
      },
      {
        to: "/reviews",
        label: "影评中心",
        description: "评分、短评、举报与内容审核",
        icon: Star,
        disabled: true,
        badge: "规划中",
      },
    ],
  },
  {
    label: "安全管理",
    items: [
      {
        to: "/roles",
        label: "角色权限",
        description: "后台角色与最小权限边界",
        icon: UserCog,
        permission: "roles:read",
      },
      {
        to: "/audit",
        label: "操作记录",
        description: "追踪敏感操作与审计请求",
        icon: FileClock,
        permission: "audit:read",
      },
      {
        to: "/risk",
        label: "风险事件",
        description: "异常登录、共享设备与安全告警",
        icon: ShieldCheck,
        disabled: true,
        badge: "规划中",
      },
      {
        to: "/settings",
        label: "系统设置",
        description: "功能开关、通知与运行参数",
        icon: Settings2,
        permission: "settings:read",
        disabled: true,
        badge: "规划中",
      },
    ],
  },
];

export const allAdminNavigationItems = adminNavigation.flatMap((section) => section.items);
