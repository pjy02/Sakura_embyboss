<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { CirclePlus, Flag, Heart, Pencil, Search, Star, Trash2, X } from "lucide-vue-next";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { MediaReview } from "@/types";

const sessionStore = useSessionStore();
const feed = ref<MediaReview[]>([]);
const mine = ref<MediaReview[]>([]);
const loading = ref(true);
const tab = ref<"feed" | "mine">("feed");
const search = ref("");
const modalOpen = ref(false);
const editing = ref<MediaReview | null>(null);
const deleteTarget = ref<MediaReview | null>(null);
const reportTarget = ref<MediaReview | null>(null);
const busy = ref(false);
const error = ref("");
const revealed = ref(new Set<string>());
const form = reactive({ media_key: "", media_title: "", media_year: undefined as number | undefined, rating: 8, content: "", spoiler: false });
const report = reactive({ reason: "spam", detail: "" });

function statusLabel(status: MediaReview["status"]) {
  return ({ pending: "审核中", published: "已发布", rejected: "未通过", hidden: "已隐藏" } as Record<string, string>)[status];
}
function statusTone(status: MediaReview["status"]): "warning" | "success" | "danger" | "muted" {
  return status === "published" ? "success" : status === "rejected" ? "danger" : status === "hidden" ? "muted" : "warning";
}
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100" });
  if (search.value) params.set("search", search.value);
  try {
    const [published, personal] = await Promise.all([
      api<{ items: MediaReview[] }>(`/me/reviews?${params}`),
      api<{ items: MediaReview[] }>("/me/reviews/mine?limit=100"),
    ]);
    feed.value = published.items;
    mine.value = personal.items;
  } finally { loading.value = false; }
}
function openEditor(review?: MediaReview) {
  editing.value = review || null;
  Object.assign(form, {
    media_key: review?.media_key || "",
    media_title: review?.media_title || "",
    media_year: review?.media_year || undefined,
    rating: review?.rating || 8,
    content: review?.content || "",
    spoiler: review?.spoiler || false,
  });
  error.value = ""; modalOpen.value = true;
}
async function save() {
  busy.value = true; error.value = "";
  try {
    if (editing.value) {
      await api(`/me/reviews/${editing.value.id}`, { method: "PATCH", body: JSON.stringify({ rating: form.rating, content: form.content, spoiler: form.spoiler }) });
    } else {
      await api("/me/reviews", { method: "POST", body: JSON.stringify({ ...form, media_year: form.media_year || null }) });
    }
    modalOpen.value = false; tab.value = "mine"; await load();
  } catch (e) { error.value = e instanceof Error ? e.message : "影评保存失败"; }
  finally { busy.value = false; }
}
async function react(item: MediaReview) {
  const updated = await api<MediaReview>(`/me/reviews/${item.id}/reaction`, { method: "PUT", body: JSON.stringify({ enabled: !item.liked }) });
  Object.assign(item, updated);
}
async function submitReport() {
  if (!reportTarget.value) return;
  busy.value = true;
  try {
    await api(`/me/reviews/${reportTarget.value.id}/report`, { method: "POST", body: JSON.stringify(report) });
    reportTarget.value = null; Object.assign(report, { reason: "spam", detail: "" }); await load();
  } finally { busy.value = false; }
}
async function removeReview() {
  if (!deleteTarget.value) return;
  busy.value = true;
  try { await api(`/me/reviews/${deleteTarget.value.id}`, { method: "DELETE" }); deleteTarget.value = null; await load(); }
  finally { busy.value = false; }
}
function reveal(id: string) {
  revealed.value = new Set([...revealed.value, id]);
}
onMounted(load);
useRealtimeEvents(["review.updated", "review.deleted"], () => load());
</script>

<template>
  <div class="page-stack">
    <header class="page-heading"><div><span class="eyebrow">SAKURA REVIEWS</span><h1>影评社区</h1><p>分享观影感受、评分与短评；新内容经过管理员审核后公开。</p></div><button class="primary-button" @click="openEditor()"><CirclePlus :size="17" /> 写影评</button></header>
    <div class="community-toolbar"><div class="segmented-control"><button :class="{ active: tab === 'feed' }" @click="tab = 'feed'">社区精选</button><button :class="{ active: tab === 'mine' }" @click="tab = 'mine'">我的影评</button></div><label v-if="tab === 'feed'" class="search-box"><Search :size="17" /><input v-model.trim="search" placeholder="搜索作品或影评" @keyup.enter="load" /></label></div>
    <LoadingBlock v-if="loading" />
    <EmptyState v-else-if="!(tab === 'feed' ? feed : mine).length" :title="tab === 'feed' ? '暂无已发布影评' : '你还没有写影评'" />
    <section v-else class="review-grid">
      <article v-for="item in tab === 'feed' ? feed : mine" :key="item.id" class="panel review-card">
        <header><div><span class="review-score"><Star :size="16" fill="currentColor" /> {{ item.rating }}</span><small>{{ item.media_year || "年份未知" }}</small></div><StatusBadge v-if="tab === 'mine'" :label="statusLabel(item.status)" :tone="statusTone(item.status)" /></header>
        <h2>{{ item.media_title }}</h2><small class="review-author">TG {{ item.tg }} · {{ formatDate(item.created_at) }}</small>
        <div v-if="item.spoiler && !revealed.has(item.id)" class="spoiler-cover"><strong>这条影评包含剧透</strong><button class="text-button" @click="reveal(item.id)">仍然查看</button></div><p v-else>{{ item.content }}</p>
        <p v-if="item.admin_note && tab === 'mine'" class="request-note">审核备注：{{ item.admin_note }}</p>
        <footer v-if="tab === 'feed'"><button class="review-action" :class="{ active: item.liked }" @click="react(item)"><Heart :size="16" :fill="item.liked ? 'currentColor' : 'none'" /> {{ item.like_count }}</button><button v-if="item.tg !== sessionStore.session?.tg" class="review-action" @click="reportTarget = item"><Flag :size="15" /> 举报</button></footer>
        <footer v-else><button v-if="['pending','rejected'].includes(item.status)" class="review-action" @click="openEditor(item)"><Pencil :size="15" /> 修改</button><button class="review-action danger-text" @click="deleteTarget = item"><Trash2 :size="15" /> 删除</button></footer>
      </article>
    </section>
    <div v-if="modalOpen" class="modal-layer"><form class="modal-card review-editor-card" @submit.prevent="save"><header><div><span class="section-kicker">WRITE A REVIEW</span><h2>{{ editing ? "修改影评" : "写一篇影评" }}</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header><template v-if="!editing"><label><span>作品名称</span><input v-model.trim="form.media_title" required maxlength="255" /></label><div class="form-grid"><label><span>作品标识</span><input v-model.trim="form.media_key" required maxlength="255" placeholder="Emby ID、TMDB ID 或唯一名称" /></label><label><span>年份</span><input v-model.number="form.media_year" type="number" min="1888" max="2200" /></label></div></template><label><span>评分（1–10）</span><input v-model.number="form.rating" type="range" min="1" max="10" /><strong class="rating-preview">{{ form.rating }} / 10</strong></label><label><span>影评内容</span><textarea v-model.trim="form.content" required minlength="10" maxlength="5000" /></label><label class="check-row"><input v-model="form.spoiler" type="checkbox" /><span>包含剧透内容</span></label><p v-if="error" class="form-error">{{ error }}</p><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="busy">{{ busy ? "提交中…" : "提交审核" }}</button></footer></form></div>
    <div v-if="reportTarget" class="modal-layer"><form class="modal-card" @submit.prevent="submitReport"><header><div><span class="section-kicker">REPORT</span><h2>举报影评</h2></div><button type="button" class="icon-button" @click="reportTarget = null"><X :size="19" /></button></header><label><span>举报原因</span><select v-model="report.reason"><option value="spam">垃圾内容</option><option value="abuse">攻击或辱骂</option><option value="spoiler">未标注剧透</option><option value="irrelevant">内容无关</option><option value="other">其他</option></select></label><label><span>补充说明</span><textarea v-model.trim="report.detail" maxlength="500" /></label><footer><button type="button" class="secondary-button" @click="reportTarget = null">取消</button><button class="danger-button" :disabled="busy">提交举报</button></footer></form></div>
    <ConfirmDialog :open="Boolean(deleteTarget)" title="删除这篇影评？" description="删除后无法恢复，公开页面也将不再显示。" confirm-label="确认删除" tone="danger" :busy="busy" @close="deleteTarget = null" @confirm="removeReview">{{ deleteTarget?.media_title }}</ConfirmDialog>
  </div>
</template>
