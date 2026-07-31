<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { CheckCircle2, EyeOff, Flag, Search, Star, XCircle } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { MediaReview } from "@/types";

const sessionStore = useSessionStore();
const items = ref<MediaReview[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const status = ref("");
const selected = ref<MediaReview | null>(null);
const busy = ref(false);
const form = reactive({ status: "published", admin_note: "" });
let searchTimer: number | undefined;
const canUpdate = computed(() => sessionStore.session?.permissions.some((p) => p === "*" || p === "reviews:*" || p === "reviews:update"));
function label(value: MediaReview["status"]) { return ({ pending: "待审核", published: "已发布", rejected: "已拒绝", hidden: "已隐藏" } as Record<string, string>)[value]; }
function tone(value: MediaReview["status"]): "warning" | "success" | "danger" | "muted" { return value === "published" ? "success" : value === "rejected" ? "danger" : value === "hidden" ? "muted" : "warning"; }
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100" }); if (search.value) params.set("search", search.value); if (status.value) params.set("status", status.value);
  try { const result = await api<{ items: MediaReview[]; total: number }>(`/admin/reviews?${params}`); items.value = result.items; total.value = result.total; } finally { loading.value = false; }
}
async function open(item: MediaReview) { selected.value = await api<MediaReview>(`/admin/reviews/${item.id}`); form.status = item.status === "pending" ? "published" : item.status; form.admin_note = item.admin_note || ""; }
async function moderate() {
  if (!selected.value) return; busy.value = true;
  try { selected.value = await api<MediaReview>(`/admin/reviews/${selected.value.id}`, { method: "PATCH", body: JSON.stringify(form) }); await load(); }
  finally { busy.value = false; }
}
watch(search, () => { window.clearTimeout(searchTimer); searchTimer = window.setTimeout(load, 350); }); watch(status, load); onMounted(load);
useRealtimeEvents(["review.created", "review.reported", "review.updated"], () => load(), true);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="CONTENT MODERATION" title="影评中心" description="审核评分与短评，处理剧透标记和用户举报，维护社区内容质量。" :icon="Star"><template #meta><span class="date-chip"><Star :size="16" /> {{ formatNumber(total) }} 篇影评</span></template></AdminPageHeader>
    <section class="panel table-panel"><FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索作品、内容或 Telegram ID" /></label><label class="select-box"><select v-model="status"><option value="">全部状态</option><option value="pending">待审核</option><option value="published">已发布</option><option value="rejected">已拒绝</option><option value="hidden">已隐藏</option></select></label></FilterBar><AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无影评" min-width="960px"><template #head><tr><th>作品</th><th>用户</th><th>评分</th><th>内容</th><th>点赞</th><th>举报</th><th>提交时间</th><th>状态</th><th /></tr></template><template #body><tr v-for="item in items" :key="item.id" @click="open(item)"><td><div class="table-primary"><strong>{{ item.media_title }}</strong><small>{{ item.media_key }} · {{ item.media_year || "—" }}</small></div></td><td>TG · {{ item.tg }}</td><td><span class="review-score"><Star :size="14" fill="currentColor" /> {{ item.rating }}</span></td><td><span class="cell-sub">{{ item.content }}</span></td><td>{{ item.like_count }}</td><td><span :class="{ 'negative-text': item.report_count }">{{ item.report_count }}</span></td><td>{{ formatDate(item.created_at) }}</td><td><StatusBadge :label="label(item.status)" :tone="tone(item.status)" /></td><td><button class="text-button">审核</button></td></tr></template></AdminDataTable></section>
    <DetailDrawer :open="Boolean(selected)" title="影评审核" eyebrow="REVIEW DETAIL" :description="selected?.media_title || ''" @close="selected = null"><template v-if="selected"><div class="review-detail-score"><span><Star :size="22" fill="currentColor" /></span><strong>{{ selected.rating }} / 10</strong><small>TG {{ selected.tg }} · {{ formatDate(selected.created_at) }}</small></div><div class="review-detail-copy"><span v-if="selected.spoiler" class="soft-badge">包含剧透</span><p>{{ selected.content }}</p></div><dl class="detail-list boxed"><div><dt>点赞</dt><dd>{{ selected.like_count }}</dd></div><div><dt>举报</dt><dd>{{ selected.report_count }}</dd></div><div><dt>当前状态</dt><dd>{{ label(selected.status) }}</dd></div><div><dt>审核人</dt><dd>{{ selected.moderated_by || "—" }}</dd></div></dl><section v-if="selected.reports?.length" class="review-report-list"><span class="section-kicker">REPORTS</span><article v-for="report in selected.reports" :key="report.id"><strong>{{ report.reason }} · TG {{ report.tg }}</strong><p>{{ report.detail || "未填写补充说明" }}</p><small>{{ formatDate(report.created_at) }}</small></article></section><form v-if="canUpdate" class="request-editor" @submit.prevent="moderate"><label><span>审核结论</span><select v-model="form.status"><option value="published">通过并发布</option><option value="rejected">拒绝</option><option value="hidden">隐藏内容</option></select></label><label><span>审核备注</span><textarea v-model.trim="form.admin_note" maxlength="1000" placeholder="拒绝或隐藏时建议说明原因" /></label><button class="primary-button wide" :disabled="busy"><CheckCircle2 v-if="form.status === 'published'" :size="16" /><XCircle v-else-if="form.status === 'rejected'" :size="16" /><EyeOff v-else :size="16" /> 保存审核结果</button></form><p v-if="selected.report_count" class="device-note"><Flag :size="15" /> 这篇内容已被 {{ selected.report_count }} 名用户举报，请重点复核。</p></template></DetailDrawer>
  </div>
</template>
