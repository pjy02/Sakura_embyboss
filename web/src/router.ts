import { createRouter, createWebHistory } from "vue-router";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";

const LoginView = () => import("@/views/LoginView.vue");
const RegisterView = () => import("@/views/RegisterView.vue");
const AppShell = () => import("@/components/AppShell.vue");

const childRoutes =
  runtime.area === "admin"
    ? [
        {
          path: "",
          name: "admin-home",
          component: () => import("@/views/admin/AdminHome.vue"),
          meta: { title: "仪表盘", section: "总览" },
        },
        {
          path: "users",
          name: "users",
          component: () => import("@/views/admin/UsersView.vue"),
          meta: { title: "站点账号", section: "用户运营", permission: "users:read" },
        },
        {
          path: "memberships",
          name: "memberships",
          component: () => import("@/views/admin/MembershipsView.vue"),
          meta: { title: "会员与标签", section: "用户运营", permission: "users:read" },
        },
        {
          path: "invitation-codes",
          name: "invitation-codes",
          component: () => import("@/views/admin/InvitationCodesView.vue"),
          meta: { title: "邀请码中心", section: "用户运营", permission: "codes:read" },
        },
        {
          path: "operations",
          name: "operations-center",
          component: () => import("@/views/admin/OperationsCenterView.vue"),
          meta: { title: "批量运营", section: "用户运营", permission: "users:read" },
        },
        {
          path: "playback/live",
          name: "playback-live",
          component: () => import("@/views/admin/PlaybackLiveView.vue"),
          meta: { title: "在线播放", section: "用户运营", permission: "playback:read" },
        },
        {
          path: "playback/history",
          name: "playback-history",
          component: () => import("@/views/admin/PlaybackHistoryView.vue"),
          meta: { title: "播放历史", section: "用户运营", permission: "playback:read" },
        },
        {
          path: "devices",
          name: "devices",
          component: () => import("@/views/admin/DevicesView.vue"),
          meta: { title: "设备管理", section: "用户运营", permission: "devices:read" },
        },
        {
          path: "lines",
          name: "lines",
          component: () => import("@/views/admin/LinesView.vue"),
          meta: { title: "线路管理", section: "线路与系统", permission: "lines:read" },
        },
        {
          path: "tasks",
          name: "tasks",
          component: () => import("@/views/admin/TasksView.vue"),
          meta: { title: "系统任务", section: "线路与系统", permission: "tasks:read" },
        },
        {
          path: "system/status",
          name: "system-status",
          component: () => import("@/views/admin/SystemStatusView.vue"),
          meta: { title: "服务状态", section: "线路与系统", permission: "tasks:read" },
        },
        {
          path: "system/diagnostics",
          name: "diagnostics",
          component: () => import("@/views/admin/DiagnosticsView.vue"),
          meta: { title: "诊断中心", section: "线路与系统", permission: "tasks:read" },
        },
        {
          path: "billing/recharge",
          name: "recharge",
          component: () => import("@/views/admin/RechargeView.vue"),
          meta: { title: "充值中心", section: "交易与服务", permission: "billing:read" },
        },
        {
          path: "billing/ledger",
          name: "billing-ledger",
          component: () => import("@/views/admin/BillingLedgerView.vue"),
          meta: { title: "账单记录", section: "交易与服务", permission: "billing:read" },
        },
        {
          path: "tickets",
          name: "tickets",
          component: () => import("@/views/admin/TicketsView.vue"),
          meta: { title: "工单管理", section: "交易与服务", permission: "tickets:read" },
        },
        {
          path: "requests",
          name: "requests",
          component: () => import("@/views/admin/RequestsView.vue"),
          meta: { title: "求片订阅", section: "内容社区", permission: "requests:read" },
        },
        {
          path: "reviews",
          name: "reviews",
          component: () => import("@/views/admin/ReviewsView.vue"),
          meta: { title: "影评中心", section: "内容社区", permission: "reviews:read" },
        },
        {
          path: "notifications",
          name: "notifications",
          component: () => import("@/views/admin/NotificationsView.vue"),
          meta: { title: "通知中心", section: "内容社区", permission: "notifications:read" },
        },
        {
          path: "roles",
          name: "roles",
          component: () => import("@/views/admin/RolesView.vue"),
          meta: { title: "角色权限", section: "安全管理", permission: "roles:read" },
        },
        {
          path: "audit",
          name: "audit",
          component: () => import("@/views/admin/AuditView.vue"),
          meta: { title: "操作记录", section: "安全管理", permission: "audit:read" },
        },
        {
          path: "risk",
          name: "risk",
          component: () => import("@/views/admin/RiskEventsView.vue"),
          meta: { title: "风险事件", section: "安全管理", permission: "security:read" },
        },
        {
          path: "settings",
          name: "settings",
          component: () => import("@/views/admin/SystemSettingsView.vue"),
          meta: { title: "系统设置", section: "安全管理", permission: "settings:read" },
        },
      ]
    : [
        {
          path: "",
          name: "portal-home",
          component: () => import("@/views/portal/PortalHome.vue"),
          meta: { title: "我的首页" },
        },
        {
          path: "points",
          name: "points",
          component: () => import("@/views/portal/PointsView.vue"),
          meta: { title: "积分明细" },
        },
        {
          path: "account",
          name: "account",
          component: () => import("@/views/portal/AccountView.vue"),
          meta: { title: "账户安全" },
        },
        {
          path: "billing",
          name: "portal-billing",
          component: () => import("@/views/portal/BillingView.vue"),
          meta: { title: "充值与账单" },
        },
        {
          path: "tickets",
          name: "portal-tickets",
          component: () => import("@/views/portal/TicketsView.vue"),
          meta: { title: "我的工单" },
        },
        {
          path: "requests",
          name: "portal-requests",
          component: () => import("@/views/portal/RequestsView.vue"),
          meta: { title: "我的求片" },
        },
        {
          path: "reviews",
          name: "portal-reviews",
          component: () => import("@/views/portal/ReviewsView.vue"),
          meta: { title: "影评社区" },
        },
        {
          path: "notifications",
          name: "portal-notifications",
          component: () => import("@/views/portal/NotificationsView.vue"),
          meta: { title: "通知中心" },
        },
      ];

export const router = createRouter({
  history: createWebHistory(`${runtime.basePath}/`),
  routes: [
    { path: "/login", name: "login", component: LoginView, meta: { public: true } },
    ...(runtime.area === "portal"
      ? [
          {
            path: "/register",
            name: "register",
            component: RegisterView,
            meta: { public: true, title: "注册中心" },
          },
        ]
      : []),
    {
      path: "/",
      component: AppShell,
      children: childRoutes,
    },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
  scrollBehavior: () => ({ top: 0 }),
});

router.beforeEach(async (to) => {
  const sessionStore = useSessionStore();
  if (!sessionStore.checked) await sessionStore.load();
  if (to.meta.public) {
    if (sessionStore.authenticated && to.name === "login") return { path: "/" };
    return true;
  }
  if (!sessionStore.authenticated) return { name: "login", query: { next: to.fullPath } };
  if (runtime.area === "admin" && !sessionStore.isAdministrator) {
    sessionStore.session = null;
    return { name: "login", query: { forbidden: "1" } };
  }
  const required = typeof to.meta.permission === "string" ? to.meta.permission : "";
  if (
    required &&
    !sessionStore.session?.permissions.some(
      (permission) =>
        permission === "*" ||
        permission === required ||
        permission === `${required.split(":")[0]}:*`,
    )
  ) {
    return { path: "/" };
  }
  return true;
});
