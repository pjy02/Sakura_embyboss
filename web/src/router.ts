import { createRouter, createWebHistory } from "vue-router";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";

const LoginView = () => import("@/views/LoginView.vue");
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
          path: "tasks",
          name: "tasks",
          component: () => import("@/views/admin/TasksView.vue"),
          meta: { title: "系统任务", section: "线路与系统", permission: "tasks:read" },
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
      ];

export const router = createRouter({
  history: createWebHistory(`${runtime.basePath}/`),
  routes: [
    { path: "/login", name: "login", component: LoginView, meta: { public: true } },
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
