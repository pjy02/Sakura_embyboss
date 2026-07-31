<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { Clapperboard, Search, SlidersHorizontal, Sparkles } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { MediaRequest } from "@/types";

const sessionStore = useSessionStore();
const items = ref<MediaRequest[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const status = ref("");
const selected = ref<MediaRequest | null>(null);
const busy = ref(false);
const form = reactive({ status: "submitted", priority: "normal", progress: 0, download_id: "", external_ref: "", cost_coins: 0, admin_note: "" });
let searchTimer: number | undefined;
const canUpdate = computed(() => sessionStore.session?.permissions.some((item) => item === "*" || item === "requests:*" || item === "requests:update"));
function tone(value: MediaRequest["status"]): "info" | "warning" | "success" | "danger" | "muted" {
  if (value === "completed") return "success"; if (value === "rejected") return "danger"; if (value === "canceled") return "muted"; if (value === "downloading") return "warning"; return "info";
}
function label(value: MediaRequest["status"]) { return ({ submitted: "待审核", reviewing: "审核中", approved: "已批准", searching: "搜索中", downloading: "下载中", completed: "已入库", rejected: "已拒绝", canceled: "已取消" } as Record<string, string>)[value]; }
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" }); if (search.value) params.set("search", search.value); if (status.value) params.set("status", status.value);
  try { const result = await api<{ items: MediaRequest[]; total: number }>(`/admin/requests?${params}`); items.value = result.items; total.value = result.total; } finally { loading.value = false; }
}
function open(item: MediaRequest) {
  selected.value = item;
  Object.assign(form, { status: item.status, priority: item.priority, progress: item.progress, download_id: item.download_id || "", external_ref: item.external_ref || "", cost_coins: item.cost_coins, admin_note: item.admin_note || "" });
}
async function save() {
  if (!selected.value) return; busy.value = true;
  try {
    const updated = await api<MediaRequest>(`/admin/requests/${selected.value.id}`, { method: "PATCH", body: JSON.stringify({ ...form, download_id: form.download_id || null, external_ref: form.external_ref || null, admin_note: form.admin_note || null }) });
    selected.value = updated; await load();
  } finally { busy.value = false; }
}
watch(search, () => { window.clearTimeout(searchTimer); searchTimer = window.setTimeout(load, 350); }); watch(status, load); onMounted(load);
useRealtimeEvents(["request.created", "request.updated"], () => load(), true);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="MEDIA REQUESTS" title="求片订阅" description="统一查看 Web 提交和 Telegram MoviePilot 下载记录，审核并跟踪搜索、下载和入库状态。" :icon="Clapperboard">
      <template #meta><span class="date-chip"><Sparkles :size="16" /> {{ formatNumber(total) }} 条求片</span></template>
    </AdminPageHeader>
    <section class="panel table-panel">
      <FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索标题、求片号、下载 ID 或 Telegram ID" /></label><label class="select-box"><SlidersHorizontal :size="17" /><select v-model="status"><option value="">全部状态</option><option value="submitted">待审核</option><option value="reviewing">审核中</option><option value="approved">已批准</option><option value="searching">搜索中</option><option value="downloading">下载中</option><option value="completed">已入库</option><option value="rejected">已拒绝</option><option value="canceled">已取消</option></select></label></FilterBar>
      <AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无求片" min-width="980px">
        <template #head><tr><th>求片</th><th>用户</th><th>类型</th><th>来源</th><th>积分</th><th>进度</th><th>更新时间</th><th>状态</th><th /></tr></template>
        <template #body><tr v-for="item in items" :key="item.id" @click="open(item)"><td><div class="table-primary"><strong>{{ item.title }}{{ item.year ? ` (${item.year})` : "" }}</strong><small>{{ item.request_no }}</small></div></td><td>TG · {{ item.tg }}</td><td>{{ item.media_type }}</td><td>{{ item.source === "telegram" ? "Bot" : "Web" }}</td><td>{{ formatNumber(item.cost_coins) }}</td><td><div class="request-progress"><i><b :style="{ width: `${item.progress}%` }" /></i><span>{{ item.progress }}%</span></div></td><td>{{ formatDate(item.updated_at) }}</td><td><StatusBadge :label="label(item.status)" :tone="tone(item.status)" /></td><td><button class="text-button">处理</button></td></tr></template>
      </AdminDataTable>
    </section>
    <DetailDrawer :open="Boolean(selected)" title="求片详情" eyebrow="REQUEST DETAIL" :description="selected?.request_no || ''" @close="selected = null">
      <template v-if="selected"><div class="request-hero"><span><Clapperboard :size="22" /></span><div><h3>{{ selected.title }}</h3><p>{{ selected.year || "年份未知" }} · {{ selected.media_type }} · TG {{ selected.tg }}</p></div></div><dl class="detail-list boxed"><div><dt>来源</dt><dd>{{ selected.source }}</dd></div><div><dt>下载 ID</dt><dd>{{ selected.download_id || "—" }}</dd></div><div><dt>消耗积分</dt><dd>{{ selected.cost_coins }}</dd></div><div><dt>提交时间</dt><dd>{{ formatDate(selected.created_at) }}</dd></div></dl><p v-if="selected.description" class="device-note">{{ selected.description }}</p>
        <form v-if="canUpdate" class="request-editor" @submit.prevent="save"><div class="form-grid"><label><span>状态</span><select v-model="form.status"><option value="submitted">待审核</option><option value="reviewing">审核中</option><option value="approved">已批准</option><option value="searching">搜索中</option><option value="downloading">下载中</option><option value="completed">已入库</option><option value="rejected">已拒绝</option><option value="canceled">已取消</option></select></label><label><span>优先级</span><select v-model="form.priority"><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></label><label><span>下载进度</span><input v-model.number="form.progress" type="number" min="0" max="100" /></label><label><span>积分成本</span><input v-model.number="form.cost_coins" type="number" min="0" /></label></div><label><span>下载 ID</span><input v-model.trim="form.download_id" /></label><label><span>外部参考</span><input v-model.trim="form.external_ref" /></label><label><span>管理员备注</span><textarea v-model.trim="form.admin_note" maxlength="1000" /></label><button class="primary-button wide" :disabled="busy">保存处理结果</button></form>
      </template>
    </DetailDrawer>
  </div>
</template>
