<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import {
  ChevronDown,
  CircleUserRound,
  Coins,
  House,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  ShieldCheck,
  Sparkles,
  X,
} from "lucide-vue-next";
import AdminCommandPalette from "@/components/admin/AdminCommandPalette.vue";
import BrandMark from "@/components/BrandMark.vue";
import {
  adminNavigation,
  type AdminNavigationSection,
} from "@/config/admin-navigation";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";

const sessionStore = useSessionStore();
const route = useRoute();
const router = useRouter();
const mobileOpen = ref(false);
const accountOpen = ref(false);
const searchOpen = ref(false);
const busy = ref(false);
const collapsed = ref(window.localStorage.getItem("sakura-admin-sidebar-collapsed") === "1");

const isAdmin = runtime.area === "admin";
const pageTitle = computed(() => String(route.meta.title || (isAdmin ? "管理控制台" : "个人中心")));
const pageSection = computed(() => String(route.meta.section || (isAdmin ? "Sakura Operations" : "Sakura Portal")));
const permissions = computed(() => sessionStore.session?.permissions || []);

function hasPermission(required?: string) {
  if (!required) return true;
  return permissions.value.some(
    (permission) =>
      permission === "*" ||
      permission === required ||
      permission === `${required.split(":")[0]}:*`,
  );
}

const portalNavigation: AdminNavigationSection[] = [
  {
    label: "个人中心",
    items: [
      { to: "/", label: "我的首页", description: "账户摘要与最近动态", icon: House },
      { to: "/points", label: "积分明细", description: "积分和注册天数流水", icon: Coins },
      { to: "/account", label: "账户安全", description: "会话、登录与账户操作", icon: ShieldCheck },
    ],
  },
];

const navigationSections = computed(() => {
  const source = isAdmin ? adminNavigation : portalNavigation;
  return source
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => hasPermission(item.permission)),
    }))
    .filter((section) => section.items.length);
});

function active(path: string) {
  return path === "/" ? route.path === "/" : route.path.startsWith(path);
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value;
  window.localStorage.setItem("sakura-admin-sidebar-collapsed", collapsed.value ? "1" : "0");
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

function onGlobalKeydown(event: KeyboardEvent) {
  if (!isAdmin) return;
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    searchOpen.value = true;
  }
}

watch(
  () => route.fullPath,
  () => {
    mobileOpen.value = false;
    accountOpen.value = false;
    searchOpen.value = false;
  },
);

onMounted(() => window.addEventListener("keydown", onGlobalKeydown));
onBeforeUnmount(() => window.removeEventListener("keydown", onGlobalKeydown));
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': collapsed && isAdmin }">
    <div class="ambient ambient-one" />
    <div class="ambient ambient-two" />

    <aside class="sidebar" :class="{ open: mobileOpen }">
      <div class="sidebar-top">
        <BrandMark />
        <button class="icon-button mobile-only" type="button" aria-label="关闭菜单" @click="mobileOpen = false">
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

      <nav class="side-nav" aria-label="主导航">
        <section v-for="section in navigationSections" :key="section.label" class="nav-section">
          <small class="nav-section-label">{{ section.label }}</small>
          <div>
            <template v-for="item in section.items" :key="item.to">
              <button
                v-if="item.disabled"
                class="side-nav-item disabled"
                type="button"
                :title="collapsed ? `${item.label} · ${item.badge}` : item.description"
                disabled
              >
                <component :is="item.icon" :size="19" />
                <span class="nav-item-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
                <em v-if="item.badge">{{ item.badge }}</em>
              </button>
              <RouterLink
                v-else
                class="side-nav-item"
                :to="item.to"
                :class="{ active: active(item.to) }"
                :title="collapsed ? item.label : item.description"
                @click="mobileOpen = false"
              >
                <component :is="item.icon" :size="19" />
                <span class="nav-item-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
              </RouterLink>
            </template>
          </div>
        </section>
      </nav>

      <div class="sidebar-foot">
        <div class="privacy-note">
          <ShieldCheck :size="16" />
          <span>身份与操作均由服务端验证</span>
        </div>
        <small>SAKURA WEB · 2.1</small>
      </div>

      <button
        v-if="isAdmin"
        class="sidebar-collapse desktop-only"
        type="button"
        :aria-label="collapsed ? '展开侧边栏' : '收起侧边栏'"
        @click="toggleCollapsed"
      >
        <PanelLeftOpen v-if="collapsed" :size="17" />
        <PanelLeftClose v-else :size="17" />
        <span>{{ collapsed ? "展开" : "收起导航" }}</span>
      </button>
    </aside>

    <button v-if="mobileOpen" class="nav-backdrop" type="button" aria-label="关闭菜单" @click="mobileOpen = false" />

    <main class="main-area">
      <header class="topbar">
        <button class="icon-button mobile-only" type="button" aria-label="打开菜单" @click="mobileOpen = true">
          <Menu :size="21" />
        </button>

        <div class="topbar-context">
          <small>{{ pageSection }}</small>
          <strong>{{ pageTitle }}</strong>
        </div>

        <button v-if="isAdmin" class="global-search-trigger" type="button" @click="searchOpen = true">
          <Search :size="17" />
          <span>搜索用户或功能</span>
          <kbd>⌘ K</kbd>
        </button>

        <div class="system-health-pill">
          <span class="status-dot" />
          <span>{{ isAdmin ? "服务正常" : "已安全连接" }}</span>
        </div>

        <div class="account-menu">
          <button class="account-trigger" type="button" :aria-expanded="accountOpen" @click="accountOpen = !accountOpen">
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
            <button :disabled="busy" type="button" @click="logout">
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

    <AdminCommandPalette
      v-if="isAdmin"
      :open="searchOpen"
      :permissions="permissions"
      @close="searchOpen = false"
    />
  </div>
</template>
