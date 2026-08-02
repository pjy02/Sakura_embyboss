<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { Clapperboard, DownloadCloud, Search, SlidersHorizontal, Sparkles } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatFileSize, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { MediaRequest } from "@/types";

type MpResource = Record<string, unknown> & { meta_info?: Record<string, unknown>; torrent_info?: Record<string, unknown> };
const session = useSessionStore();
const items = ref<MediaRequest[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const status = ref("");
const selected = ref<MediaRequest | null>(null);
const busy = ref(false);
const mpOpen = ref(false);
const mpLoading = ref(false);
const mpItems = ref<MpResource[]>([]);
const form = reactive({ status: "submitted", priority: "normal", progress: 0, download_id: "", external_ref: "", cost_coins: 0, admin_note: "" });
let timer: number | undefined;
const canUpdate = computed(() => session.session?.permissions.some((item) => item === "*" || item === "requests:*" || item === "requests:update"));
const canMoviePilot = computed(() => session.session?.permissions.some((item) => item === "*" || item === "media:*" || item === "media:manage"));

function tone(value: MediaRequest["status"]): "info" | "warning" | "success" | "danger" | "muted" { return value === "completed" ? "success" : value === "rejected" ? "danger" : value === "canceled" ? "muted" : value === "downloading" ? "warning" : "info"; }
function label(value: MediaRequest["status"]) { return ({ submitted: "待审核", reviewing: "审核中", approved: "已批准", searching: "搜索中", downloading: "下载中", completed: "已入库", rejected: "已拒绝", canceled: "已取消" } as Record<string, string>)[value] || value; }

async function load() { loading.value = true; const params = new URLSearchParams({ limit: "100", offset: "0" }); if (search.value) params.set("search", search.value); if (status.value) params.set("status", status.value); try { const result = await api<{ items: MediaRequest[]; total: number }>(`/admin/requests?${params}`); items.value = result.items; total.value = result.total; } finally { loading.value = false; } }
function open(item: MediaRequest) { selected.value = item; Object.assign(form, { status: item.status, priority: item.priority, progress: item.progress, download_id: item.download_id || "", external_ref: item.external_ref || "", cost_coins: item.cost_coins, admin_note: item.admin_note || "" }); }
async function save() { if (!selected.value) return; busy.value = true; try { selected.value = await api<MediaRequest>(`/admin/requests/${selected.value.id}`, { method: "PATCH", body: JSON.stringify({ ...form, download_id: form.download_id || null, external_ref: form.external_ref || null, admin_note: form.admin_note || null }) }); await load(); } finally { busy.value = false; } }
async function openMoviePilot() { if (!selected.value) return; mpOpen.value = true; mpLoading.value = true; mpItems.value = []; try { const result = await api<{ items: MpResource[] }>(`/admin/moviepilot/search?q=${encodeURIComponent(selected.value.title)}`); mpItems.value = result.items; } finally { mpLoading.value = false; } }
function mpTitle(item: MpResource) { return String(item.meta_info?.title || item.torrent_info?.title || item.title || selected.value?.title || "未知资源"); }
function mpMeta(item: MpResource) { const meta = item.meta_info || {}; const torrent = item.torrent_info || {}; return [meta.year, meta.resource_pix, meta.video_encode, torrent.seeders ? `${torrent.seeders} 做种` : null].filter(Boolean).join(" · "); }
function mpSize(item: MpResource) { const value = Number(item.torrent_info?.size || item.size || 0); return value ? formatFileSize(value) : "大小未知"; }
async function submitMoviePilot(item: MpResource) { if (!selected.value || !window.confirm(`将“${mpTitle(item)}”提交给 MoviePilot？`)) return; busy.value = true; try { selected.value = await api<MediaRequest>(`/admin/moviepilot/requests/${selected.value.id}/submit`, { method: "POST", body: JSON.stringify({ resource: item, confirm: true }) }); mpOpen.value = false; await load(); } finally { busy.value = false; } }
watch(search, () => { window.clearTimeout(timer); timer = window.setTimeout(load, 350); }); watch(status, load); onMounted(load); useRealtimeEvents(["request.created", "request.updated"], load, true);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="MEDIA REQUESTS" title="求片订阅" description="统一处理 Web 与 Bot 求片，保留 TMDB 标识，并直接搜索和提交 MoviePilot 下载任务。" :icon="Clapperboard"><template #meta><span class="date-chip"><Sparkles :size="16" />{{ formatNumber(total) }} 条求片</span></template></AdminPageHeader>
    <section class="panel table-panel"><FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索标题、求片号、下载 ID 或 Telegram ID" /></label><label class="select-box"><SlidersHorizontal :size="17" /><select v-model="status"><option value="">全部状态</option><option value="submitted">待审核</option><option value="reviewing">审核中</option><option value="approved">已批准</option><option value="searching">搜索中</option><option value="downloading">下载中</option><option value="completed">已入库</option><option value="rejected">已拒绝</option><option value="canceled">已取消</option></select></label></FilterBar><AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无求片" min-width="980px"><template #head><tr><th>求片</th><th>用户</th><th>类型 / 来源</th><th>TMDB / 下载</th><th>进度</th><th>更新时间</th><th>状态</th><th /></tr></template><template #body><tr v-for="item in items" :key="item.id" @click="open(item)"><td><div class="table-primary"><strong>{{ item.title }}{{ item.year ? ` (${item.year})` : '' }}</strong><small>{{ item.request_no }}</small></div></td><td>TG · {{ item.tg }}</td><td>{{ item.media_type }}<small class="cell-sub">{{ item.source === 'telegram' ? 'Bot' : 'Web' }}</small></td><td><small>{{ item.external_ref || '—' }}</small><small class="cell-sub">{{ item.download_id || '未提交下载' }}</small></td><td><div class="request-progress"><i><b :style="{ width: `${item.progress}%` }" /></i><span>{{ item.progress }}%</span></div></td><td>{{ formatDate(item.updated_at) }}</td><td><StatusBadge :label="label(item.status)" :tone="tone(item.status)" /></td><td><button class="text-button">处理</button></td></tr></template></AdminDataTable></section>
    <DetailDrawer :open="Boolean(selected)" title="求片详情" eyebrow="REQUEST DETAIL" :description="selected?.request_no || ''" @close="selected = null"><template v-if="selected"><div class="request-hero"><span><Clapperboard :size="22" /></span><div><h3>{{ selected.title }}</h3><p>{{ selected.year || '年份未知' }} · {{ selected.media_type }} · TG {{ selected.tg }}</p></div></div><dl class="detail-list boxed"><div><dt>外部标识</dt><dd>{{ selected.external_ref || '—' }}</dd></div><div><dt>下载 ID</dt><dd>{{ selected.download_id || '—' }}</dd></div><div><dt>积分成本</dt><dd>{{ selected.cost_coins }}</dd></div><div><dt>提交时间</dt><dd>{{ formatDate(selected.created_at) }}</dd></div></dl><p v-if="selected.description" class="device-note">{{ selected.description }}</p><button v-if="canMoviePilot && !selected.download_id && !['completed','rejected','canceled'].includes(selected.status)" class="moviepilot-button" @click="openMoviePilot"><DownloadCloud :size="18" /><span><strong>在 MoviePilot 搜索资源</strong><small>选择资源后直接建立下载任务并回写状态</small></span></button><form v-if="canUpdate" class="request-editor" @submit.prevent="save"><div class="form-grid"><label><span>状态</span><select v-model="form.status"><option value="submitted">待审核</option><option value="reviewing">审核中</option><option value="approved">已批准</option><option value="searching">搜索中</option><option value="downloading">下载中</option><option value="completed">已入库</option><option value="rejected">已拒绝</option><option value="canceled">已取消</option></select></label><label><span>优先级</span><select v-model="form.priority"><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></label><label><span>下载进度</span><input v-model.number="form.progress" type="number" min="0" max="100" /></label><label><span>积分成本</span><input v-model.number="form.cost_coins" type="number" min="0" /></label></div><label><span>下载 ID</span><input v-model.trim="form.download_id" /></label><label><span>外部标识</span><input v-model.trim="form.external_ref" /></label><label><span>管理员备注</span><textarea v-model.trim="form.admin_note" maxlength="1000" /></label><button class="primary-button wide" :disabled="busy">保存处理结果</button></form></template></DetailDrawer>
    <div v-if="mpOpen" class="modal-layer" @click.self="mpOpen = false"><article class="modal-card mp-modal"><header><div><span class="section-kicker">MOVIEPILOT SEARCH</span><h2>选择下载资源</h2></div><button class="icon-button" @click="mpOpen = false">×</button></header><p class="mp-hint">搜索：{{ selected?.title }}。提交后会自动写入下载 ID 并进入“下载中”。</p><div v-if="mpLoading" class="mp-empty">正在搜索 MoviePilot…</div><div v-else-if="!mpItems.length" class="mp-empty">没有找到资源，请检查 MoviePilot 地址和凭据，或稍后重试。</div><div v-else class="mp-results"><article v-for="(item, index) in mpItems" :key="index"><div><strong>{{ mpTitle(item) }}</strong><p>{{ mpMeta(item) || '暂无版本信息' }}</p><small>{{ mpSize(item) }}</small></div><button class="primary-button compact" :disabled="busy" @click="submitMoviePilot(item)">提交下载</button></article></div><footer><button class="secondary-button" @click="mpOpen = false">取消</button></footer></article></div>
  </div>
</template>

<style scoped>
.moviepilot-button{display:flex;align-items:center;gap:12px;width:100%;margin:14px 0;padding:14px;border:1px solid color-mix(in srgb,var(--primary) 35%,var(--border));border-radius:14px;background:color-mix(in srgb,var(--primary) 7%,var(--surface));color:var(--text);text-align:left}.moviepilot-button svg{color:var(--primary)}.moviepilot-button span,.moviepilot-button strong,.moviepilot-button small{display:block}.moviepilot-button small{margin-top:3px;color:var(--text-muted)}.mp-modal{width:min(760px,calc(100vw - 30px));max-height:85vh}.mp-hint{color:var(--text-muted)}.mp-results{display:grid;gap:10px;overflow:auto}.mp-results article{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px;border:1px solid var(--border);border-radius:13px;background:var(--surface-subtle)}.mp-results p{margin:5px 0;color:var(--text-muted)}.mp-empty{text-align:center;padding:36px;color:var(--text-muted)}.compact{padding:8px 11px;white-space:nowrap}
</style>
