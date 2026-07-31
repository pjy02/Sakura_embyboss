<script setup lang="ts">
import { onMounted, ref } from "vue";
import { Bell, BellRing, CheckCheck, Settings2 } from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { NotificationPreference, UserNotification } from "@/types";

const items = ref<UserNotification[]>([]);
const preferences = ref<NotificationPreference[]>([]);
const loading = ref(true);
const unreadOnly = ref(false);
const tab = ref<"inbox" | "preferences">("inbox");
const busy = ref(false);
function categoryLabel(value: string) { return ({ system: "系统", billing: "账单", ticket: "工单", request: "求片", review: "影评" } as Record<string, string>)[value] || value; }
async function load() {
  loading.value = true;
  try {
    const [notifications, settings] = await Promise.all([
      api<{ items: UserNotification[] }>(`/me/notifications?limit=100&unread_only=${unreadOnly.value}`),
      api<{ items: NotificationPreference[] }>("/me/notification-preferences"),
    ]);
    items.value = notifications.items; preferences.value = settings.items;
  } finally { loading.value = false; }
}
async function read(item: UserNotification) {
  if (!item.read_at) {
    item.read_at = (await api<UserNotification>(`/me/notifications/${item.id}/read`, { method: "POST" })).read_at;
    window.dispatchEvent(new Event("sakura:notifications-changed"));
  }
}
async function readAll() { busy.value = true; try { await api("/me/notifications/read-all", { method: "POST" }); window.dispatchEvent(new Event("sakura:notifications-changed")); await load(); } finally { busy.value = false; } }
async function savePreference(item: NotificationPreference) {
  await api("/me/notification-preferences", { method: "PUT", body: JSON.stringify({ category: item.category, web_enabled: item.web_enabled }) });
}
onMounted(load);
useRealtimeEvents(["notification.created"], () => load());
</script>
<template>
  <div class="page-stack">
    <header class="page-heading"><div><span class="eyebrow">NOTIFICATION CENTER</span><h1>通知中心</h1><p>集中查看充值、工单、求片、影评和系统公告。</p></div><button v-if="tab === 'inbox'" class="secondary-button" :disabled="busy" @click="readAll"><CheckCheck :size="17" /> 全部已读</button></header>
    <div class="community-toolbar"><div class="segmented-control"><button :class="{ active: tab === 'inbox' }" @click="tab = 'inbox'"><Bell :size="15" /> 收件箱</button><button :class="{ active: tab === 'preferences' }" @click="tab = 'preferences'"><Settings2 :size="15" /> 通知设置</button></div><label v-if="tab === 'inbox'" class="check-row notification-filter"><input v-model="unreadOnly" type="checkbox" @change="load" /> 只看未读</label></div>
    <LoadingBlock v-if="loading" />
    <template v-else-if="tab === 'inbox'"><EmptyState v-if="!items.length" title="暂无通知" /><section v-else class="notification-list"><article v-for="item in items" :key="item.id" class="panel notification-item" :class="{ unread: !item.read_at }" @click="read(item)"><span><BellRing :size="18" /></span><div><header><strong>{{ item.title }}</strong><StatusBadge :label="categoryLabel(item.category)" :tone="item.severity === 'danger' ? 'danger' : item.severity === 'warning' ? 'warning' : item.severity === 'success' ? 'success' : 'info'" /></header><p>{{ item.body }}</p><small>{{ formatDate(item.created_at) }}<template v-if="!item.read_at"> · 未读</template></small></div><RouterLink v-if="item.action_url" class="text-button" :to="item.action_url">查看</RouterLink></article></section></template>
    <section v-else class="panel preference-panel"><header><div><span class="section-kicker">DELIVERY PREFERENCES</span><h2>站内通知设置</h2></div></header><article v-for="item in preferences" :key="item.category"><div><strong>{{ item.label }}</strong><small>{{ categoryLabel(item.category) }}类业务状态变化；不会影响 Bot 原有消息</small></div><label><input v-model="item.web_enabled" type="checkbox" @change="savePreference(item)" /> 接收站内通知</label></article></section>
  </div>
</template>
