<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  Activity,
  CircleStop,
  Clock3,
  MonitorPlay,
  Pause,
  Play,
  RefreshCw,
  Radio,
  Router,
} from "lucide-vue-next";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { LivePlaybackResponse, PlaybackSession } from "@/types";

const sessionStore = useSessionStore();
const response = ref<LivePlaybackResponse | null>(null);
const loading = ref(true);
const refreshing = ref(false);
const selected = ref<PlaybackSession | null>(null);
const stopping = ref(false);
const error = ref("");
let refreshTimer: number | undefined;

const canStop = computed(() =>
  sessionStore.session?.permissions.some(
    (item) => item === "*" || item === "playback:*" || item === "playback:stop",
  ),
);

function duration(ticks: number) {
  const seconds = Math.max(0, Math.floor(ticks / 10_000_000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;
  return hours ? `${hours}:${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}` : `${minutes}:${String(rest).padStart(2, "0")}`;
}

async function load(silent = false) {
  if (silent) refreshing.value = true;
  else loading.value = true;
  error.value = "";
  try {
    response.value = await api<LivePlaybackResponse>("/admin/playback/live");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "在线会话加载失败";
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
}

async function stopPlayback() {
  if (!selected.value) return;
  stopping.value = true;
  error.value = "";
  try {
    await api(`/admin/playback/${encodeURIComponent(selected.value.session_id)}/stop`, {
      method: "POST",
      body: JSON.stringify({ reason: "管理员从 Web 运营后台终止播放" }),
    });
    selected.value = null;
    await load(true);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "终止播放失败";
  } finally {
    stopping.value = false;
  }
}

onMounted(async () => {
  await load();
  refreshTimer = window.setInterval(() => load(true), 15_000);
});
onBeforeUnmount(() => window.clearInterval(refreshTimer));
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="LIVE PLAYBACK"
      title="在线播放"
      description="实时查看 Emby 播放会话、进度、客户端与网络来源，异常会话可直接终止。"
      :icon="MonitorPlay"
    >
      <template #meta>
        <span class="date-chip"><Radio :size="16" /> {{ response?.total || 0 }} 个活跃会话</span>
      </template>
      <template #actions>
        <button class="secondary-button" :disabled="refreshing" @click="load(true)">
          <RefreshCw :size="16" :class="{ spin: refreshing }" /> 刷新
        </button>
      </template>
    </AdminPageHeader>

    <div v-if="error || response?.error" class="error-banner">{{ error || response?.error }}</div>
    <LoadingBlock v-if="loading" />
    <EmptyState
      v-else-if="!response?.items.length"
      title="当前没有在线播放"
      :description="response?.source === 'unavailable' ? '暂时无法连接 Emby，请检查站点地址与 API Key。' : '新的播放开始后会自动显示。'"
    />
    <section v-else class="playback-grid">
      <article v-for="item in response.items" :key="item.id" class="panel playback-card">
        <header>
          <div class="playback-art"><Play v-if="!item.is_paused" :size="21" /><Pause v-else :size="21" /></div>
          <div>
            <span class="section-kicker">{{ item.item_type || "MEDIA" }}</span>
            <h2>{{ item.item_name || "未知媒体" }}</h2>
            <p v-if="item.series_name">{{ item.series_name }}</p>
          </div>
          <StatusBadge :label="item.is_paused ? '已暂停' : '播放中'" :tone="item.is_paused ? 'warning' : 'success'" />
        </header>
        <div class="playback-progress">
          <div><span>{{ duration(item.position_ticks) }}</span><span>{{ duration(item.runtime_ticks) }}</span></div>
          <i><b :style="{ width: `${Math.min(100, item.progress_percent)}%` }" /></i>
        </div>
        <dl class="playback-meta">
          <div><dt><Activity :size="15" /> 用户</dt><dd>{{ item.emby_user_name || item.emby_user_id || "未知" }}</dd></div>
          <div><dt><Router :size="15" /> 客户端</dt><dd>{{ item.client_name || "未知" }} · {{ item.device_name || "未知设备" }}</dd></div>
          <div><dt><Radio :size="15" /> 网络</dt><dd>{{ item.remote_address || "未知 IP" }}</dd></div>
          <div><dt><Clock3 :size="15" /> 同步</dt><dd>{{ formatDate(item.last_seen_at) }}</dd></div>
        </dl>
        <footer>
          <StatusBadge v-if="item.is_transcoding" label="转码" tone="warning" />
          <span v-else class="direct-play">Direct Play</span>
          <button v-if="canStop" class="danger-button" @click="selected = item"><CircleStop :size="16" /> 终止播放</button>
        </footer>
      </article>
    </section>

    <ConfirmDialog
      :open="Boolean(selected)"
      title="确认终止播放？"
      description="客户端会立即停止播放并收到管理员提示，此操作会写入审计日志。"
      confirm-label="终止会话"
      tone="danger"
      :busy="stopping"
      @close="selected = null"
      @confirm="stopPlayback"
    >
      {{ selected?.emby_user_name || "未知用户" }} · {{ selected?.item_name || "未知媒体" }}
    </ConfirmDialog>
  </div>
</template>
