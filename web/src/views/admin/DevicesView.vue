<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Ban, CheckCircle2, HardDrive, Search, ShieldCheck, ShieldQuestion } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { KnownDevice } from "@/types";

const sessionStore = useSessionStore();
const items = ref<KnownDevice[]>([]);
const total = ref(0);
const search = ref("");
const risk = ref("");
const loading = ref(true);
const selected = ref<KnownDevice | null>(null);
const pendingAction = ref<"trust" | "ban" | "unban" | null>(null);
const busy = ref(false);
let searchTimer: number | undefined;
const canUpdate = computed(() =>
  sessionStore.session?.permissions.some(
    (item) => item === "*" || item === "devices:*" || item === "devices:update",
  ),
);

function riskTone(device: KnownDevice): "success" | "warning" | "danger" | "muted" {
  if (device.banned || device.risk_level === "high") return "danger";
  if (device.risk_level === "warning") return "warning";
  if (device.trusted) return "success";
  return "muted";
}

function riskLabel(device: KnownDevice) {
  if (device.banned) return "已封禁";
  if (device.trusted) return "已信任";
  if (device.risk_level === "warning") return "需关注";
  return "未验证";
}

async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" });
  if (search.value) params.set("search", search.value);
  if (risk.value) params.set("risk_level", risk.value);
  try {
    const result = await api<{ items: KnownDevice[]; total: number }>(`/admin/devices?${params}`);
    items.value = result.items;
    total.value = result.total;
    if (selected.value) selected.value = result.items.find((item) => item.device_key === selected.value?.device_key) || null;
  } finally {
    loading.value = false;
  }
}

async function applyAction() {
  if (!selected.value || !pendingAction.value) return;
  busy.value = true;
  const body =
    pendingAction.value === "trust"
      ? { trusted: true, banned: false }
      : pendingAction.value === "ban"
        ? { banned: true, trusted: false }
        : { banned: false };
  try {
    selected.value = await api<KnownDevice>(`/admin/devices/${encodeURIComponent(selected.value.device_key)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
    pendingAction.value = null;
    await load();
  } finally {
    busy.value = false;
  }
}

watch(search, () => {
  window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(load, 350);
});
watch(risk, load);
onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="DEVICE INTELLIGENCE"
      title="设备管理"
      description="根据播放活动建立设备画像，识别账号共享与异常切换，并维护信任和封禁状态。"
      :icon="HardDrive"
    >
      <template #meta><span class="date-chip"><HardDrive :size="16" /> {{ formatNumber(total) }} 台已知设备</span></template>
    </AdminPageHeader>

    <section class="panel table-panel">
      <FilterBar>
        <label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索设备、用户、客户端、IP 或设备标识" /></label>
        <label class="select-box"><ShieldQuestion :size="17" /><select v-model="risk"><option value="">全部风险</option><option value="normal">正常</option><option value="warning">需关注</option><option value="high">高风险</option></select></label>
      </FilterBar>
      <AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无设备数据" empty-description="在线播放同步后会自动建立设备画像。" min-width="900px">
        <template #head><tr><th>设备</th><th>关联用户</th><th>最近 IP</th><th>播放次数</th><th>首次出现</th><th>最近活跃</th><th>风险</th><th /></tr></template>
        <template #body>
          <tr v-for="device in items" :key="device.device_key" @click="selected = device">
            <td><div class="table-primary"><strong>{{ device.device_name || "未知设备" }}</strong><small>{{ device.client_name || "未知客户端" }} {{ device.app_version || "" }}</small></div></td>
            <td>{{ device.emby_user_name || "—" }}<small class="cell-sub">{{ device.tg ? `TG · ${device.tg}` : device.emby_user_id || "" }}</small></td>
            <td>{{ device.last_ip || "—" }}</td>
            <td class="strong-cell">{{ formatNumber(device.playback_count) }}</td>
            <td>{{ formatDate(device.first_seen_at) }}</td>
            <td>{{ formatDate(device.last_seen_at) }}</td>
            <td><StatusBadge :label="riskLabel(device)" :tone="riskTone(device)" /></td>
            <td><button class="text-button">查看</button></td>
          </tr>
        </template>
      </AdminDataTable>
    </section>

    <DetailDrawer :open="Boolean(selected)" title="设备详情" eyebrow="DEVICE PROFILE" :description="selected?.device_key || ''" @close="selected = null">
      <template v-if="selected">
        <div class="drawer-profile"><span><HardDrive :size="22" /></span><div><h3>{{ selected.device_name || "未知设备" }}</h3><p>{{ selected.client_name || "未知客户端" }} · {{ selected.app_version || "未知版本" }}</p></div></div>
        <div class="drawer-badges"><StatusBadge :label="riskLabel(selected)" :tone="riskTone(selected)" /></div>
        <dl class="detail-list boxed">
          <div><dt>关联用户</dt><dd>{{ selected.emby_user_name || "—" }}</dd></div>
          <div><dt>Telegram ID</dt><dd>{{ selected.tg || "—" }}</dd></div>
          <div><dt>最近 IP</dt><dd>{{ selected.last_ip || "—" }}</dd></div>
          <div><dt>播放次数</dt><dd>{{ formatNumber(selected.playback_count) }}</dd></div>
          <div><dt>首次出现</dt><dd>{{ formatDate(selected.first_seen_at) }}</dd></div>
          <div><dt>最近活跃</dt><dd>{{ formatDate(selected.last_seen_at) }}</dd></div>
        </dl>
        <p v-if="selected.notes" class="device-note">{{ selected.notes }}</p>
      </template>
      <template #actions>
        <button v-if="canUpdate && selected && !selected.trusted" class="secondary-button" @click="pendingAction = 'trust'"><ShieldCheck :size="16" /> 标记信任</button>
        <button v-if="canUpdate && selected && !selected.banned" class="danger-button" @click="pendingAction = 'ban'"><Ban :size="16" /> 封禁设备</button>
        <button v-else-if="canUpdate && selected" class="secondary-button" @click="pendingAction = 'unban'"><CheckCircle2 :size="16" /> 解除封禁</button>
      </template>
    </DetailDrawer>

    <ConfirmDialog
      :open="Boolean(pendingAction)"
      :title="pendingAction === 'trust' ? '信任这台设备？' : pendingAction === 'ban' ? '封禁这台设备？' : '解除设备封禁？'"
      :description="pendingAction === 'ban' ? '设备会被标记为高风险，供后续客户端过滤与风控策略使用。' : '状态变更会立即写入设备画像和审计日志。'"
      :confirm-label="pendingAction === 'ban' ? '确认封禁' : '确认变更'"
      :tone="pendingAction === 'ban' ? 'danger' : 'normal'"
      :busy="busy"
      @close="pendingAction = null"
      @confirm="applyAction"
    >
      {{ selected?.device_name || selected?.device_key }}
    </ConfirmDialog>
  </div>
</template>
