<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { CirclePlus, Clapperboard, X } from "lucide-vue-next";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { MediaRequest } from "@/types";

const items = ref<MediaRequest[]>([]); const loading = ref(true); const modalOpen = ref(false); const cancelTarget = ref<MediaRequest | null>(null); const busy = ref(false);
const form = reactive({ title: "", year: undefined as number | undefined, media_type: "movie", description: "" });
function label(value: MediaRequest["status"]) { return ({ submitted: "待审核", reviewing: "审核中", approved: "已批准", searching: "搜索中", downloading: "下载中", completed: "已入库", rejected: "已拒绝", canceled: "已取消" } as Record<string, string>)[value]; }
function tone(value: MediaRequest["status"]): "info" | "warning" | "success" | "danger" | "muted" { return value === "completed" ? "success" : value === "rejected" ? "danger" : value === "canceled" ? "muted" : value === "downloading" ? "warning" : "info"; }
async function load() { loading.value = true; try { items.value = (await api<{ items: MediaRequest[] }>("/me/requests?limit=100")).items; } finally { loading.value = false; } }
async function createRequest() { busy.value = true; try { await api("/me/requests", { method: "POST", body: JSON.stringify({ ...form, year: form.year || null, description: form.description || null }) }); modalOpen.value = false; Object.assign(form, { title: "", year: undefined, media_type: "movie", description: "" }); await load(); } finally { busy.value = false; } }
async function cancelRequest() { if (!cancelTarget.value) return; busy.value = true; try { await api(`/me/requests/${cancelTarget.value.id}/cancel`, { method: "POST" }); cancelTarget.value = null; await load(); } finally { busy.value = false; } }
onMounted(load);
useRealtimeEvents(["request.created", "request.updated"], () => load());
</script>
<template>
  <div class="page-stack">
    <header class="page-heading"><div><span class="eyebrow">MEDIA WISHLIST</span><h1>我的求片</h1><p>提交希望加入媒体库的作品，并跟踪管理员审核、下载和入库进度。</p></div><button class="primary-button" @click="modalOpen = true"><CirclePlus :size="17" /> 提交求片</button></header>
    <LoadingBlock v-if="loading" /><EmptyState v-else-if="!items.length" title="还没有求片记录" description="提交电影、剧集或其他内容后会显示在这里。" />
    <section v-else class="portal-request-grid"><article v-for="item in items" :key="item.id" class="panel portal-request-card"><header><span><Clapperboard :size="19" /></span><StatusBadge :label="label(item.status)" :tone="tone(item.status)" /></header><h2>{{ item.title }}{{ item.year ? ` (${item.year})` : "" }}</h2><p>{{ item.description || "未填写补充说明" }}</p><div class="request-progress"><i><b :style="{ width: `${item.progress}%` }" /></i><span>{{ item.progress }}%</span></div><dl><div><dt>求片编号</dt><dd>{{ item.request_no }}</dd></div><div><dt>提交时间</dt><dd>{{ formatDate(item.created_at) }}</dd></div><div><dt>来源</dt><dd>{{ item.source === "telegram" ? "Telegram Bot" : "Web" }}</dd></div><div><dt>消耗积分</dt><dd>{{ item.cost_coins }}</dd></div></dl><p v-if="item.admin_note" class="request-note">{{ item.admin_note }}</p><button v-if="['submitted','reviewing'].includes(item.status)" class="text-button danger-text" @click="cancelTarget = item">取消求片</button></article></section>
    <div v-if="modalOpen" class="modal-layer"><form class="modal-card" @submit.prevent="createRequest"><header><div><span class="section-kicker">NEW REQUEST</span><h2>提交求片</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header><label><span>作品名称</span><input v-model.trim="form.title" required maxlength="255" /></label><div class="form-grid"><label><span>年份</span><input v-model.number="form.year" type="number" min="1888" max="2200" placeholder="可选" /></label><label><span>类型</span><select v-model="form.media_type"><option value="movie">电影</option><option value="series">剧集</option><option value="anime">动漫</option><option value="documentary">纪录片</option><option value="other">其他</option></select></label></div><label><span>补充说明</span><textarea v-model.trim="form.description" maxlength="2000" placeholder="版本、字幕或其他要求（可选）" /></label><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="busy">提交求片</button></footer></form></div>
    <ConfirmDialog :open="Boolean(cancelTarget)" title="取消求片？" description="取消后管理员将不再继续处理这条请求。" confirm-label="确认取消" tone="danger" :busy="busy" @close="cancelTarget = null" @confirm="cancelRequest">{{ cancelTarget?.title }}</ConfirmDialog>
  </div>
</template>
