<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Clapperboard, Film, Search, Sparkles, Star, Tv2 } from "lucide-vue-next";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import { api } from "@/lib/api";
import { runtime } from "@/lib/runtime";
import type { MediaCatalogItem } from "@/types";

const query = ref("");
const mediaType = ref<"" | "movie" | "tv">("");
const items = ref<MediaCatalogItem[]>([]);
const loading = ref(true);
const source = ref("cache");
const warning = ref("");
const selected = ref<MediaCatalogItem | null>(null);
const requestOpen = ref(false);
const submitting = ref(false);
const form = reactive({ description: "" });
const admin = computed(() => runtime.area === "admin");

async function loadTrending() {
  loading.value = true;
  warning.value = "";
  try {
    const result = await api<{ items: MediaCatalogItem[]; source: string; warning?: string }>("/media/trending?limit=24");
    items.value = result.items;
    source.value = result.source;
    warning.value = result.warning || "";
  } finally {
    loading.value = false;
  }
}

async function search() {
  if (!query.value.trim()) return loadTrending();
  loading.value = true;
  warning.value = "";
  try {
    const params = new URLSearchParams({ q: query.value.trim() });
    if (mediaType.value) params.set("media_type", mediaType.value);
    const result = await api<{ items: MediaCatalogItem[]; source: string; warning?: string }>(`/media/search?${params}`);
    items.value = result.items;
    source.value = result.source;
    warning.value = result.warning || "";
  } finally {
    loading.value = false;
  }
}

function openRequest(item: MediaCatalogItem) {
  selected.value = item;
  form.description = "";
  requestOpen.value = true;
}

async function submitRequest() {
  if (!selected.value) return;
  submitting.value = true;
  try {
    await api("/me/requests", { method: "POST", body: JSON.stringify({ title: selected.value.title, year: selected.value.year, media_type: selected.value.media_type === "tv" ? "series" : "movie", description: form.description || selected.value.overview, external_ref: selected.value.external_ref }) });
    requestOpen.value = false;
  } finally {
    submitting.value = false;
  }
}

onMounted(loadTrending);
</script>

<template>
  <div class="page-stack media-page">
    <AdminPageHeader eyebrow="MEDIA DISCOVERY" title="TMDB 影片中心" description="搜索电影和剧集，查看统一资料并直接创建求片订阅；无需手动查找 TMDB ID。" :icon="Clapperboard">
      <template #meta><span class="date-chip"><Sparkles :size="16" /> {{ source === "tmdb" ? "TMDB 实时资料" : "本地缓存" }}</span></template>
    </AdminPageHeader>

    <section class="panel discovery-bar"><label class="media-search"><Search :size="20" /><input v-model.trim="query" placeholder="输入片名，例如：沙丘、三体、The Bear" @keyup.enter="search" /></label><select v-model="mediaType"><option value="">电影与剧集</option><option value="movie">仅电影</option><option value="tv">仅剧集</option></select><button class="primary-button" @click="search">搜索影片</button></section>
    <p v-if="warning" class="inline-warning">TMDB 暂时不可用，正在显示本地缓存：{{ warning }}</p>
    <LoadingBlock v-if="loading" />
    <EmptyState v-else-if="!items.length" title="没有找到匹配影片" description="请更换关键词，或让管理员在凭据中心配置 TMDB Read Access Token。" />
    <section v-else class="media-grid">
      <article v-for="item in items" :key="item.external_ref" class="media-card" @click="selected = item">
        <div class="poster"><img v-if="item.poster_url" :src="item.poster_url" :alt="item.title" loading="lazy" /><div v-else class="poster-placeholder"><Film :size="34" /></div><span class="type-chip"><Tv2 v-if="item.media_type === 'tv'" :size="13" /><Film v-else :size="13" />{{ item.media_type === "tv" ? "剧集" : "电影" }}</span></div>
        <div class="media-copy"><div><h3>{{ item.title }}</h3><p>{{ item.year || "年份未知" }}<span v-if="item.vote_average"><Star :size="13" />{{ item.vote_average.toFixed(1) }}</span></p></div><button v-if="!admin" class="secondary-button compact" @click.stop="openRequest(item)">求片</button><button v-else class="text-button" @click.stop="selected = item">查看资料</button></div>
      </article>
    </section>

    <div v-if="selected && !requestOpen" class="modal-layer" @click.self="selected = null"><article class="modal-card media-detail"><header><div><span class="section-kicker">{{ selected.external_ref }}</span><h2>{{ selected.title }}</h2></div><button class="icon-button" @click="selected = null">×</button></header><div class="detail-hero" :style="selected.backdrop_url ? { backgroundImage: `linear-gradient(90deg, rgba(15,16,28,.94), rgba(15,16,28,.72)), url(${selected.backdrop_url})` } : {}"><img v-if="selected.poster_url" :src="selected.poster_url" :alt="selected.title" /><div><p class="detail-meta">{{ selected.media_type === 'tv' ? '剧集' : '电影' }} · {{ selected.year || '年份未知' }} · TMDB {{ selected.provider_id }}</p><p>{{ selected.overview || "暂无剧情简介。" }}</p></div></div><footer><button class="secondary-button" @click="selected = null">关闭</button><button v-if="!admin" class="primary-button" @click="openRequest(selected)">创建求片订阅</button></footer></article></div>

    <div v-if="requestOpen && selected" class="modal-layer" @click.self="requestOpen = false"><form class="modal-card" @submit.prevent="submitRequest"><header><div><span class="section-kicker">REQUEST MEDIA</span><h2>求片：{{ selected.title }}</h2></div><button type="button" class="icon-button" @click="requestOpen = false">×</button></header><div class="request-summary"><img v-if="selected.poster_url" :src="selected.poster_url" :alt="selected.title" /><div><strong>{{ selected.title }}（{{ selected.year || "未知" }}）</strong><p>系统会保存 {{ selected.external_ref }}，管理员可直接交给 MoviePilot 搜索和下载。</p></div></div><label><span>补充要求（可选）</span><textarea v-model.trim="form.description" maxlength="2000" placeholder="例如版本、字幕、分辨率偏好" /></label><footer><button type="button" class="secondary-button" @click="requestOpen = false">取消</button><button class="primary-button" :disabled="submitting">{{ submitting ? "提交中…" : "确认求片" }}</button></footer></form></div>
  </div>
</template>

<style scoped>
.discovery-bar{display:grid;grid-template-columns:1fr 170px auto;gap:12px;padding:16px}.media-search{display:flex;align-items:center;gap:10px;padding:0 14px;border:1px solid var(--border);border-radius:12px;background:var(--surface-subtle)}.media-search input{border:0;background:transparent;width:100%;outline:0}.inline-warning{padding:12px 16px;border-radius:12px;background:color-mix(in srgb,var(--warning) 12%,transparent);color:var(--warning)}.media-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(175px,1fr));gap:18px}.media-card{overflow:hidden;border:1px solid var(--border);border-radius:18px;background:var(--surface);transition:.2s;cursor:pointer}.media-card:hover{transform:translateY(-3px);border-color:color-mix(in srgb,var(--primary) 50%,var(--border));box-shadow:var(--shadow-md)}.poster{position:relative;aspect-ratio:2/3;background:var(--surface-subtle)}.poster img{width:100%;height:100%;object-fit:cover}.poster-placeholder{display:grid;place-items:center;height:100%;color:var(--text-muted)}.type-chip{position:absolute;left:10px;bottom:10px;display:flex;align-items:center;gap:5px;padding:5px 8px;border-radius:999px;background:rgba(12,13,22,.82);color:white;font-size:11px}.media-copy{display:flex;align-items:flex-end;justify-content:space-between;gap:8px;padding:13px}.media-copy h3{margin:0;font-size:14px;line-height:1.35}.media-copy p{display:flex;align-items:center;gap:5px;margin:5px 0 0;color:var(--text-muted);font-size:12px}.compact{padding:7px 10px}.media-detail{width:min(820px,calc(100vw - 32px))}.detail-hero{display:grid;grid-template-columns:150px 1fr;gap:24px;padding:26px;border-radius:16px;background-size:cover;background-position:center}.detail-hero img{width:150px;border-radius:12px}.detail-hero p{line-height:1.7}.detail-meta{color:var(--text-muted)}.request-summary{display:flex;gap:16px;align-items:center;padding:14px;border-radius:14px;background:var(--surface-subtle)}.request-summary img{width:64px;border-radius:8px}.request-summary p{margin:5px 0 0;color:var(--text-muted)}@media(max-width:760px){.discovery-bar{grid-template-columns:1fr}.media-grid{grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.detail-hero{grid-template-columns:1fr}.detail-hero img{width:110px}}
</style>
