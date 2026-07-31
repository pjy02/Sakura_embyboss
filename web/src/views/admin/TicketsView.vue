<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { HeartHandshake, MessageCircle, Search, Send, UserCheck } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { SupportTicket } from "@/types";

const sessionStore = useSessionStore();
const items = ref<SupportTicket[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const status = ref("");
const selected = ref<SupportTicket | null>(null);
const reply = ref("");
const internal = ref(false);
const busy = ref(false);
let searchTimer: number | undefined;
const canUpdate = computed(() => sessionStore.session?.permissions.some((item) => item === "*" || item === "tickets:*" || item === "tickets:update"));

function tone(value: SupportTicket["status"]): "info" | "warning" | "success" | "muted" {
  if (value === "resolved") return "success";
  if (value === "closed") return "muted";
  if (value === "pending_staff") return "warning";
  return "info";
}
function statusLabel(value: SupportTicket["status"]) {
  return ({ open: "新工单", pending_user: "等待用户", pending_staff: "等待处理", resolved: "已解决", closed: "已关闭" } as Record<string, string>)[value];
}
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" });
  if (search.value) params.set("search", search.value);
  if (status.value) params.set("status", status.value);
  try {
    const result = await api<{ items: SupportTicket[]; total: number }>(`/admin/tickets?${params}`);
    items.value = result.items; total.value = result.total;
  } finally { loading.value = false; }
}
async function open(ticket: SupportTicket) {
  selected.value = await api<SupportTicket>(`/admin/tickets/${ticket.id}`);
}
async function sendReply() {
  if (!selected.value || !reply.value.trim()) return;
  busy.value = true;
  try {
    await api(`/admin/tickets/${selected.value.id}/messages`, { method: "POST", body: JSON.stringify({ body: reply.value, internal: internal.value }) });
    reply.value = ""; internal.value = false;
    await open(selected.value); await load();
  } finally { busy.value = false; }
}
async function updateTicket(data: Record<string, unknown>) {
  if (!selected.value) return;
  busy.value = true;
  try {
    await api(`/admin/tickets/${selected.value.id}`, { method: "PATCH", body: JSON.stringify(data) });
    await open(selected.value); await load();
  } finally { busy.value = false; }
}
function changeStatus(event: Event) {
  updateTicket({ status: (event.target as HTMLSelectElement).value });
}
async function syncTickets() {
  await load();
  if (selected.value) await open(selected.value);
}
watch(search, () => { window.clearTimeout(searchTimer); searchTimer = window.setTimeout(load, 350); });
watch(status, load);
onMounted(load);
useRealtimeEvents(["ticket.created", "ticket.updated"], () => syncTickets(), true);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="SUPPORT DESK" title="工单管理" description="集中处理账户、播放、充值、求片和技术问题，保留用户与客服的完整对话。" :icon="HeartHandshake">
      <template #meta><span class="date-chip"><MessageCircle :size="16" /> {{ formatNumber(total) }} 个工单</span></template>
    </AdminPageHeader>
    <section class="panel table-panel">
      <FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索工单号、主题或 Telegram ID" /></label><label class="select-box"><select v-model="status"><option value="">全部状态</option><option value="open">新工单</option><option value="pending_staff">等待处理</option><option value="pending_user">等待用户</option><option value="resolved">已解决</option><option value="closed">已关闭</option></select></label></FilterBar>
      <AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无工单" min-width="900px">
        <template #head><tr><th>工单</th><th>用户</th><th>分类</th><th>优先级</th><th>最后回复</th><th>负责人</th><th>状态</th><th /></tr></template>
        <template #body><tr v-for="ticket in items" :key="ticket.id" @click="open(ticket)"><td><div class="table-primary"><strong>{{ ticket.subject }}</strong><small>{{ ticket.ticket_no }}</small></div></td><td>TG · {{ ticket.tg }}</td><td>{{ ticket.category }}</td><td><span class="priority-chip" :data-priority="ticket.priority">{{ ticket.priority }}</span></td><td>{{ formatDate(ticket.last_reply_at) }}</td><td>{{ ticket.assignee_tg || "未分派" }}</td><td><StatusBadge :label="statusLabel(ticket.status)" :tone="tone(ticket.status)" /></td><td><button class="text-button">处理</button></td></tr></template>
      </AdminDataTable>
    </section>
    <DetailDrawer :open="Boolean(selected)" title="工单会话" eyebrow="SUPPORT THREAD" :description="selected ? `${selected.ticket_no} · TG ${selected.tg}` : ''" @close="selected = null">
      <template v-if="selected">
        <div class="ticket-summary"><div><h3>{{ selected.subject }}</h3><p>{{ selected.category }} · {{ selected.priority }}</p></div><StatusBadge :label="statusLabel(selected.status)" :tone="tone(selected.status)" /></div>
        <div class="ticket-toolbar" v-if="canUpdate"><button class="secondary-button compact-action" @click="updateTicket({ assignee_tg: sessionStore.session?.tg })"><UserCheck :size="15" /> 分派给我</button><select :value="selected.status" @change="changeStatus"><option value="open">新工单</option><option value="pending_staff">等待处理</option><option value="pending_user">等待用户</option><option value="resolved">已解决</option><option value="closed">已关闭</option></select></div>
        <div class="ticket-thread"><article v-for="message in selected.messages" :key="message.id" :data-sender="message.sender_kind" :class="{ internal: message.internal }"><header><strong>{{ message.internal ? "内部备注" : message.sender_kind === "admin" ? "管理员" : `用户 ${message.sender_tg}` }}</strong><small>{{ formatDate(message.created_at) }}</small></header><p>{{ message.body }}</p></article></div>
        <form v-if="canUpdate && selected.status !== 'closed'" class="ticket-reply" @submit.prevent="sendReply"><textarea v-model.trim="reply" required maxlength="5000" placeholder="输入回复内容…" /><label><input v-model="internal" type="checkbox" /> 仅管理员可见的内部备注</label><button class="primary-button" :disabled="busy || !reply"><Send :size="16" /> 发送</button></form>
      </template>
    </DetailDrawer>
  </div>
</template>
