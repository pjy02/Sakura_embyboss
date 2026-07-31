<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ChevronLeft, ChevronRight, History, Search, SlidersHorizontal } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import type { PlaybackSession } from "@/types";

const items = ref<PlaybackSession[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const activeOnly = ref(false);
const offset = ref(0);
const limit = 30;
let searchTimer: number | undefined;
const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)));
const currentPage = computed(() => Math.floor(offset.value / limit) + 1);

function playedFor(item: PlaybackSession) {
  const start = new Date(`${item.started_at}Z`).getTime();
  const end = new Date(`${item.ended_at || item.last_seen_at}Z`).getTime();
  const minutes = Math.max(0, Math.round((end - start) / 60_000));
  return minutes < 60 ? `${minutes} 分钟` : `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分`;
}

async function load() {
  loading.value = true;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset.value),
    active_only: String(activeOnly.value),
  });
  if (search.value) params.set("search", search.value);
  try {
    const result = await api<{ items: PlaybackSession[]; total: number }>(`/admin/playback/history?${params}`);
    items.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit);
  load();
}
watch(search, () => {
  window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(() => {
    offset.value = 0;
    load();
  }, 350);
});
watch(activeOnly, () => {
  offset.value = 0;
  load();
});
onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="PLAYBACK ARCHIVE"
      title="播放历史"
      description="从运营后台同步的播放快照中检索用户、媒体、客户端、设备与 IP。"
      :icon="History"
    >
      <template #meta><span class="date-chip">{{ formatNumber(total) }} 条记录</span></template>
    </AdminPageHeader>

    <section class="panel table-panel">
      <FilterBar>
        <label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索用户、媒体、设备、客户端或 IP" /></label>
        <label class="select-box"><SlidersHorizontal :size="17" /><select v-model="activeOnly"><option :value="false">全部记录</option><option :value="true">仅活跃会话</option></select></label>
      </FilterBar>
      <AdminDataTable
        :loading="loading"
        :empty="!items.length"
        empty-title="暂无播放记录"
        empty-description="打开在线播放页面后，在线会话会自动同步到历史记录。"
        min-width="980px"
      >
        <template #head><tr><th>用户 / 媒体</th><th>客户端</th><th>设备与 IP</th><th>进度</th><th>播放时长</th><th>开始时间</th><th>状态</th></tr></template>
        <template #body>
          <tr v-for="item in items" :key="item.id">
            <td><div class="table-primary"><strong>{{ item.emby_user_name || "未知用户" }}</strong><small>{{ item.series_name ? `${item.series_name} · ` : "" }}{{ item.item_name || "未知媒体" }}</small></div></td>
            <td>{{ item.client_name || "—" }}<small class="cell-sub">{{ item.app_version || "" }}</small></td>
            <td>{{ item.device_name || "—" }}<small class="cell-sub">{{ item.remote_address || "未知 IP" }}</small></td>
            <td class="strong-cell">{{ item.progress_percent.toFixed(1) }}%</td>
            <td>{{ playedFor(item) }}</td>
            <td>{{ formatDate(item.started_at) }}</td>
            <td><StatusBadge :label="item.ended_at ? '已结束' : item.is_paused ? '已暂停' : '播放中'" :tone="item.ended_at ? 'muted' : item.is_paused ? 'warning' : 'success'" /></td>
          </tr>
        </template>
        <template #footer><div class="pagination"><span>第 {{ currentPage }} / {{ pages }} 页</span><div><button :disabled="offset === 0" @click="page(-1)"><ChevronLeft :size="16" /> 上一页</button><button :disabled="currentPage >= pages" @click="page(1)">下一页 <ChevronRight :size="16" /></button></div></div></template>
      </AdminDataTable>
    </section>
  </div>
</template>
