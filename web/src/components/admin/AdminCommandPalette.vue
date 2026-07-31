<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ArrowRight, Command, LoaderCircle, Search, UserRound, X } from "lucide-vue-next";
import { useRouter } from "vue-router";
import { allAdminNavigationItems } from "@/config/admin-navigation";
import { api } from "@/lib/api";
import { initials } from "@/lib/format";
import type { UserProfile } from "@/types";

const props = defineProps<{
  open: boolean;
  permissions: string[];
}>();

const emit = defineEmits<{ close: [] }>();
const router = useRouter();
const query = ref("");
const users = ref<UserProfile[]>([]);
const loading = ref(false);
const error = ref("");
let timer: number | undefined;

function hasPermission(required?: string) {
  if (!required) return true;
  return props.permissions.some(
    (permission) =>
      permission === "*" ||
      permission === required ||
      permission === `${required.split(":")[0]}:*`,
  );
}

const navigationMatches = computed(() => {
  const normalized = query.value.trim().toLowerCase();
  return allAdminNavigationItems
    .filter((item) => !item.disabled && hasPermission(item.permission))
    .filter((item) => {
      if (!normalized) return true;
      return [item.label, item.description, ...(item.keywords || [])]
        .join(" ")
        .toLowerCase()
        .includes(normalized);
    })
    .slice(0, normalized ? 6 : 5);
});

const canSearchUsers = computed(() => hasPermission("users:read"));

async function searchUsers() {
  const value = query.value.trim();
  if (!canSearchUsers.value || value.length < 2) {
    users.value = [];
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams({
      search: value,
      limit: "6",
      offset: "0",
      sort_by: "tg",
      sort_order: "desc",
    });
    users.value = (await api<{ items: UserProfile[] }>(`/admin/users?${params}`)).items;
  } catch (e) {
    users.value = [];
    error.value = e instanceof Error ? e.message : "搜索失败";
  } finally {
    loading.value = false;
  }
}

function navigate(to: string) {
  router.push(to);
  emit("close");
}

function openUser(user: UserProfile) {
  router.push({ path: "/users", query: { user: String(user.tg) } });
  emit("close");
}

function onKeydown(event: KeyboardEvent) {
  if (props.open && event.key === "Escape") emit("close");
}

watch(query, () => {
  window.clearTimeout(timer);
  timer = window.setTimeout(searchUsers, 280);
});

watch(
  () => props.open,
  (open) => {
    if (open) {
      query.value = "";
      users.value = [];
      error.value = "";
      window.setTimeout(() => document.querySelector<HTMLInputElement>("#admin-command-input")?.focus(), 30);
    }
  },
);

window.addEventListener("keydown", onKeydown);
onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown);
  window.clearTimeout(timer);
});
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="command-layer" role="presentation" @click.self="emit('close')">
      <section class="command-palette" role="dialog" aria-modal="true" aria-label="全局搜索">
        <header class="command-search">
          <Search :size="20" />
          <input
            id="admin-command-input"
            v-model.trim="query"
            autocomplete="off"
            placeholder="搜索页面、用户、Telegram ID 或 Emby ID"
          />
          <LoaderCircle v-if="loading" class="spin" :size="17" />
          <button v-else type="button" aria-label="关闭搜索" @click="emit('close')"><X :size="18" /></button>
        </header>

        <div class="command-results">
          <section v-if="navigationMatches.length">
            <small>功能导航</small>
            <button v-for="item in navigationMatches" :key="item.to" type="button" @click="navigate(item.to)">
              <span class="command-result-icon"><component :is="item.icon" :size="18" /></span>
              <span><strong>{{ item.label }}</strong><em>{{ item.description }}</em></span>
              <ArrowRight :size="16" />
            </button>
          </section>

          <section v-if="users.length">
            <small>站点账号</small>
            <button v-for="user in users" :key="user.tg" type="button" @click="openUser(user)">
              <span class="command-user-avatar">{{ initials(user.name, user.tg) }}</span>
              <span>
                <strong>{{ user.name || "未创建 Emby 账户" }}</strong>
                <em>TG {{ user.tg }}<template v-if="user.embyid"> · Emby {{ user.embyid }}</template></em>
              </span>
              <UserRound :size="16" />
            </button>
          </section>

          <div v-if="query.length >= 2 && !loading && !users.length && !navigationMatches.length" class="command-empty">
            <Search :size="25" />
            <strong>没有找到匹配内容</strong>
            <p>尝试用户名、Telegram ID、Emby ID 或功能名称。</p>
          </div>
          <p v-if="error" class="command-error">{{ error }}</p>
        </div>

        <footer>
          <span><Command :size="13" /> K 打开搜索</span>
          <span><kbd>Esc</kbd> 关闭</span>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

