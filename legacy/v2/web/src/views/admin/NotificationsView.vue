<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { BellRing, CirclePlus, Send, Users, X } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { UserNotification } from "@/types";

const sessionStore = useSessionStore();
const items = ref<UserNotification[]>([]);
const total = ref(0);
const loading = ref(true);
const modalOpen = ref(false);
const busy = ref(false);
const result = ref("");
const error = ref("");
const form = reactive({ target_tg: "", category: "system", title: "", body: "", severity: "info", action_url: "" });
const canSend = computed(() => sessionStore.session?.permissions.some((p) => p === "*" || p === "notifications:*" || p === "notifications:send"));
function categoryLabel(value: string) { return ({ system: "系统", billing: "账单", ticket: "工单", request: "求片", review: "影评" } as Record<string, string>)[value] || value; }
async function load() { loading.value = true; try { const data = await api<{ items: UserNotification[]; total: number }>("/admin/notifications?limit=100"); items.value = data.items; total.value = data.total; } finally { loading.value = false; } }
async function broadcast() {
  const targetTg = form.target_tg.trim();
  result.value = ""; error.value = "";
  if (targetTg && !/^[1-9]\d*$/.test(targetTg)) {
    error.value = "Telegram ID 必须是正整数";
    return;
  }
  busy.value = true;
  try {
    const data = await api<{ created: number; recipients: number }>("/admin/notifications/broadcast", { method: "POST", body: JSON.stringify({ ...form, target_tg: targetTg ? Number(targetTg) : null, action_url: form.action_url || null }) });
    result.value = `已为 ${data.created} 名用户创建通知`; Object.assign(form, { target_tg: "", category: "system", title: "", body: "", severity: "info", action_url: "" }); await load();
  } catch (e) { error.value = e instanceof Error ? e.message : "通知发送失败"; }
  finally { busy.value = false; }
}
onMounted(load);
useRealtimeEvents(["notification.created"], () => load(), true);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="MESSAGE OPERATIONS" title="通知中心" description="发送系统公告或定向通知，并追踪站内消息创建与阅读状态。" :icon="BellRing"><template #actions><button v-if="canSend" class="primary-button" @click="modalOpen = true"><CirclePlus :size="17" /> 发送通知</button></template><template #meta><span class="date-chip"><Users :size="16" /> {{ formatNumber(total) }} 条记录</span></template></AdminPageHeader>
    <p v-if="result" class="success-banner">{{ result }}</p>
    <section class="panel table-panel"><AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无通知记录" min-width="920px"><template #head><tr><th>通知</th><th>用户</th><th>分类</th><th>级别</th><th>阅读状态</th><th>创建时间</th></tr></template><template #body><tr v-for="item in items" :key="item.id"><td><div class="table-primary"><strong>{{ item.title }}</strong><small>{{ item.body }}</small></div></td><td>TG · {{ item.tg }}</td><td>{{ categoryLabel(item.category) }}</td><td><StatusBadge :label="item.severity" :tone="item.severity === 'danger' ? 'danger' : item.severity === 'warning' ? 'warning' : item.severity === 'success' ? 'success' : 'info'" /></td><td><StatusBadge :label="item.read_at ? '已读' : '未读'" :tone="item.read_at ? 'muted' : 'info'" /></td><td>{{ formatDate(item.created_at) }}</td></tr></template></AdminDataTable></section>
    <div v-if="modalOpen" class="modal-layer"><form class="modal-card broadcast-card" @submit.prevent="broadcast"><header><div><span class="section-kicker">BROADCAST</span><h2>发送站内通知</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header><label><span>目标 Telegram ID</span><input v-model.trim="form.target_tg" inputmode="numeric" pattern="[0-9]*" placeholder="留空表示全部用户" /></label><p v-if="!form.target_tg" class="device-note"><Users :size="15" /> 当前通知将发送给全部现有用户。</p><div class="form-grid"><label><span>分类</span><select v-model="form.category"><option value="system">系统公告</option><option value="billing">充值账单</option><option value="ticket">服务工单</option><option value="request">求片进度</option><option value="review">影评状态</option></select></label><label><span>级别</span><select v-model="form.severity"><option value="info">普通</option><option value="success">成功</option><option value="warning">提醒</option><option value="danger">重要</option></select></label></div><label><span>标题</span><input v-model.trim="form.title" required minlength="2" maxlength="200" /></label><label><span>正文</span><textarea v-model.trim="form.body" required minlength="2" maxlength="2000" /></label><label><span>站内跳转路径（可选）</span><input v-model.trim="form.action_url" maxlength="500" placeholder="/billing、/tickets 等" /></label><p v-if="error" class="form-error">{{ error }}</p><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="busy"><Send :size="16" /> {{ busy ? "发送中…" : "确认发送" }}</button></footer></form></div>
  </div>
</template>
