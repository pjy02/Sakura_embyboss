<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import {
  Activity,
  ChevronDown,
  CircleUserRound,
  Coins,
  House,
  LogOut,
  Menu,
  ShieldCheck,
  Sparkles,
  UserCog,
  Users,
  X,
} from "lucide-vue-next";
import BrandMark from "@/components/BrandMark.vue";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";

const sessionStore = useSessionStore();
const route = useRoute();
const router = useRouter();
const mobileOpen = ref(false);
const accountOpen = ref(false);
const busy = ref(false);

const isAdmin = runtime.area === "admin";
function hasPermission(required?: string) {
  if (!required) return true;
  return Boolean(
    sessionStore.session?.permissions.some(
      (permission) =>
        permission === "*" ||
        permission === required ||
        permission === `${required.split(":")[0]}:*`,
    ),
  );
}

const nav = computed(() =>
  (isAdmin
    ? [
        { to: "/", label: "运营概览", icon: House },
        { to: "/users", label: "用户管理", icon: Users },
        { to: "/roles", label: "角色权限", icon: UserCog, permission: "roles:read" },
        { to: "/audit", label: "审计日志", icon: Activity, permission: "audit:read" },
      ]
    : [
        { to: "/", label: "我的首页", icon: House },
        { to: "/points", label: "积分明细", icon: Coins },
        { to: "/account", label: "账户安全", icon: ShieldCheck },
      ]
  ).filter((item) => hasPermission("permission" in item ? item.permission : undefined)),
);

function active(path: string) {
  return path === "/" ? route.path === "/" : route.path.startsWith(path);
}

async function logout() {
  busy.value = true;
  try {
    await sessionStore.logout();
    await router.replace("/login");
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="app-shell">
    <div class="ambient ambient-one" />
    <div class="ambient ambient-two" />

    <aside class="sidebar" :class="{ open: mobileOpen }">
      <div class="sidebar-top">
        <BrandMark />
        <button class="icon-button mobile-only" aria-label="关闭菜单" @click="mobileOpen = false">
          <X :size="20" />
        </button>
      </div>
      <div class="workspace-label">
        <span><Sparkles :size="14" /></span>
        <div>
          <small>当前空间</small>
          <strong>{{ isAdmin ? "管理控制台" : "个人中心" }}</strong>
        </div>
      </div>
      <nav class="side-nav">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          :class="{ active: active(item.to) }"
          @click="mobileOpen = false"
        >
          <component :is="item.icon" :size="19" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
      <div class="sidebar-foot">
        <div class="privacy-note">
          <ShieldCheck :size="16" />
          <span>身份与操作均由服务端验证</span>
        </div>
        <small>SAKURA WEB · 2.0</small>
      </div>
    </aside>

    <button v-if="mobileOpen" class="nav-backdrop" aria-label="关闭菜单" @click="mobileOpen = false" />

    <main class="main-area">
      <header class="topbar">
        <button class="icon-button mobile-only" aria-label="打开菜单" @click="mobileOpen = true">
          <Menu :size="21" />
        </button>
        <div class="topbar-title">
          <span class="status-dot" />
          <span>{{ isAdmin ? "系统运行正常" : "与 Sakura 保持连接" }}</span>
        </div>
        <div class="account-menu">
          <button class="account-trigger" @click="accountOpen = !accountOpen">
            <span class="mini-avatar"><CircleUserRound :size="19" /></span>
            <span class="account-copy">
              <strong>{{ sessionStore.session?.tg }}</strong>
              <small>{{ sessionStore.session?.roles.join(" · ") }}</small>
            </span>
            <ChevronDown :size="16" />
          </button>
          <div v-if="accountOpen" class="account-popover">
            <div>
              <small>登录方式</small>
              <strong>{{ sessionStore.session?.auth_method === "telegram" ? "Telegram" : "Emby" }}</strong>
            </div>
            <button :disabled="busy" @click="logout">
              <LogOut :size="16" />
              {{ busy ? "正在退出…" : "退出登录" }}
            </button>
          </div>
        </div>
      </header>
      <section class="page-container">
        <RouterView />
      </section>
    </main>
  </div>
</template>
