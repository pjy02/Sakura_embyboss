<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ChevronLeft, ChevronRight, Download, FileClock, RefreshCw, Search, SlidersHorizontal } from "lucide-vue-next";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import EmptyState from "@/components/EmptyState.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { actionLabel, formatDate, formatNumber } from "@/lib/format";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";
import type { AuditLog } from "@/types";

const sessionStore = useSessionStore();
const items = ref<AuditLog[]>([]);
const total = ref(0);
const loading = ref(true);
const offset = ref(0);
const limit = 50;
const selected = ref<AuditLog | null>(null);
const filters = reactive({ search: "", actor_kind: "", outcome: "", resource_type: "", date_from: "", date_to: "" });
const canNext = computed(() => offset.value + limit < total.value);
const canExport = computed(() => sessionStore.session?.permissions.some((p) => p === "*" || p === "audit:export"));
function query(includePaging = true) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (!value) continue;
    params.set(key, value);
  }
  if (includePaging) { params.set("limit", String(limit)); params.set("offset", String(offset.value)); }
  return params;
}
async function load(reset = false) {
  if (reset) offset.value = 0;
  loading.value = true;
  try { const result = await api<{ items: AuditLog[]; total: number }>(`/admin/audit?${query()}`); items.value = result.items; total.value = result.total; }
  finally { loading.value = false; }
}
function page(delta: number) { offset.value = Math.max(0, offset.value + delta * limit); load(); }
function resetFilters() { Object.assign(filters, { search: "", actor_kind: "", outcome: "", resource_type: "", date_from: "", date_to: "" }); load(true); }
function exportCsv() { window.location.href = `${runtime.apiBase}/admin/audit/export?${query(false)}`; }
onMounted(load);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="AUDIT TRAIL" title="操作记录" description="按操作者、资源、结果与时间组合检索敏感操作，并导出可归档的 CSV 记录。" :icon="FileClock"><template #actions><button v-if="canExport" class="secondary-button" @click="exportCsv"><Download :size="16" /> 导出 CSV</button><button class="secondary-button" @click="load()"><RefreshCw :size="16" /> 刷新</button></template><template #meta><span class="date-chip">{{ formatNumber(total) }} 条记录</span></template></AdminPageHeader>
    <section class="panel audit-filter-panel"><FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="filters.search" placeholder="操作、资源、请求 ID 或操作人" @keyup.enter="load(true)" /></label><label class="select-box"><SlidersHorizontal :size="16" /><select v-model="filters.actor_kind"><option value="">全部来源</option><option value="web">Web</option><option value="telegram">Telegram</option><option value="system">系统</option></select></label><label class="select-box"><select v-model="filters.outcome"><option value="">全部结果</option><option value="success">成功</option><option value="failed">失败</option></select></label></FilterBar><div class="audit-advanced-filters"><label><span>资源类型</span><input v-model.trim="filters.resource_type" placeholder="例如 media_review" /></label><label><span>开始时间</span><input v-model="filters.date_from" type="datetime-local" /></label><label><span>结束时间</span><input v-model="filters.date_to" type="datetime-local" /></label><button class="primary-button compact-action" @click="load(true)">应用筛选</button><button class="text-button" @click="resetFilters">重置</button></div></section>
    <section class="panel table-panel"><div class="panel-heading"><div><span class="section-kicker">SECURITY RECORDS</span><h2>审计时间线</h2></div><span class="page-count">第 {{ Math.floor(offset / limit) + 1 }} 页</span></div><LoadingBlock v-if="loading" /><EmptyState v-else-if="!items.length" title="没有匹配的审计记录" /><div v-else class="audit-timeline"><article v-for="item in items" :key="item.id" @click="selected = item"><span class="timeline-icon"><FileClock :size="17" /></span><div class="timeline-main"><div><strong>{{ actionLabel(item.action) }}</strong><StatusBadge :label="item.outcome" :tone="item.outcome === 'success' ? 'success' : 'danger'" /></div><p>{{ item.actor_kind }} · {{ item.actor_id }} 对 {{ item.resource_type }} {{ item.resource_id || "" }} 执行操作</p><small>{{ formatDate(item.created_at) }}<template v-if="item.ip_address"> · IP {{ item.ip_address }}</template></small></div><code>#{{ item.id }}</code></article></div><div class="pagination"><button :disabled="offset === 0" @click="page(-1)"><ChevronLeft :size="16" /> 上一页</button><button :disabled="!canNext" @click="page(1)">下一页 <ChevronRight :size="16" /></button></div></section>
    <DetailDrawer :open="Boolean(selected)" title="审计详情" eyebrow="AUDIT EVENT" :description="selected ? `#${selected.id} · ${formatDate(selected.created_at)}` : ''" @close="selected = null"><template v-if="selected"><dl class="detail-list boxed"><div><dt>操作</dt><dd>{{ actionLabel(selected.action) }}</dd></div><div><dt>原始动作</dt><dd>{{ selected.action }}</dd></div><div><dt>操作者</dt><dd>{{ selected.actor_kind }} · {{ selected.actor_id }}</dd></div><div><dt>结果</dt><dd>{{ selected.outcome }}</dd></div><div><dt>资源</dt><dd>{{ selected.resource_type }} · {{ selected.resource_id || "—" }}</dd></div><div><dt>请求 ID</dt><dd>{{ selected.request_id || "—" }}</dd></div><div><dt>IP 地址</dt><dd>{{ selected.ip_address || "—" }}</dd></div></dl><div class="audit-json"><span class="section-kicker">DETAIL PAYLOAD</span><pre>{{ JSON.stringify(selected.detail, null, 2) }}</pre></div></template></DetailDrawer>
  </div>
</template>
