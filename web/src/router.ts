import { createRouter, createWebHistory } from "vue-router";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";
import LoginView from "@/views/LoginView.vue";
import AppShell from "@/components/AppShell.vue";
import PortalHome from "@/views/portal/PortalHome.vue";
import PointsView from "@/views/portal/PointsView.vue";
import AccountView from "@/views/portal/AccountView.vue";
import AdminHome from "@/views/admin/AdminHome.vue";
import UsersView from "@/views/admin/UsersView.vue";
import RolesView from "@/views/admin/RolesView.vue";
import AuditView from "@/views/admin/AuditView.vue";

const childRoutes =
  runtime.area === "admin"
    ? [
        { path: "", name: "admin-home", component: AdminHome },
        { path: "users", name: "users", component: UsersView },
        { path: "roles", name: "roles", component: RolesView, meta: { permission: "roles:read" } },
        { path: "audit", name: "audit", component: AuditView, meta: { permission: "audit:read" } },
      ]
    : [
        { path: "", name: "portal-home", component: PortalHome },
        { path: "points", name: "points", component: PointsView },
        { path: "account", name: "account", component: AccountView },
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
