<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  Clock3,
  RefreshCw,
  Search,
  ShieldAlert,
} from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { RiskEvent, RiskSummary } from "@/types";

const sessionStore = useSessionStore();
const items = ref<RiskEvent[]>([]);
const summary = ref<RiskSummary | null>(null);
const selected = ref<RiskEvent | null>(null);
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const search = ref("");
const severity = ref("");
const status = ref("open");
const offset = ref(0);
const limit = 30;
const total = ref(0);
const form = reactive({
  status: "acknowledged" as RiskEvent["status"],
  assigned_to: "",
  resolution_note: "",
});

const canManage = computed(() =>
  sessionStore.session?.permissions.some(
    (permission) =>
      permission === "*" ||
      permission === "security:*" ||
      permission === "security:manage",
  ),
);
const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)));
const currentPage = computed(() => Math.floor(offset.value / limit) + 1);

const eventLabels: Record<string, string> = {
  "auth.telegram.rate_limited": "Telegram 登录请求过多",
  "auth.telegram.identity_mismatch": "Telegram 登录身份不匹配",
  "auth.emby.failed": "Emby 登录失败",
  "device.first_seen": "发现新设备",
  "device.owner_changed": "设备关联账号变化",
  "device.banned_playback": "被封禁设备再次播放",
  "line.offline": "线路离线",
};
const statusLabels: Record<RiskEvent["status"], string> = {
  open: "待处理",
  acknowledged: "调查中",
  resolved: "已解决",
  ignored: "已忽略",
};
const severityLabels = { info: "关注", warning: "警告", danger: "高危" };

function query() {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset.value),
  });
  if (search.value) params.set("search", search.value);
  if (severity.value) params.set("severity", severity.value);
  if (status.value) params.set("status", status.value);
  return params.toString();
}

async function load(silent = false) {
  if (!silent) loading.value = true;
  error.value = "";
  try {
    const [events, stats] = await Promise.all([
      api<{ items: RiskEvent[]; total: number }>(`/admin/risk/events?${query()}`),
      api<RiskSummary>("/admin/risk/summary"),
    ]);
    items.value = events.items;
    total.value = events.total;
    summary.value = stats;
    if (selected.value) {
      selected.value =
        events.items.find((item) => item.id === selected.value?.id) ||
        selected.value;
    }
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "风险事件加载失败";
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  offset.value = 0;
  load();
}

function open(item: RiskEvent) {
  selected.value = item;
  form.status = item.status === "open" ? "acknowledged" : item.status;
  form.assigned_to = item.assigned_to ? String(item.assigned_to) : "";
  form.resolution_note = item.resolution_note || "";
}

async function save() {
  if (!selected.value) return;
  saving.value = true;
  error.value = "";
  try {
    selected.value = await api<RiskEvent>(
      `/admin/risk/events/${selected.value.id}`,
      {
        method: "PATCH",
        body: JSON.stringify({
          status: form.status,
          assigned_to: form.assigned_to ? Number(form.assigned_to) : null,
          resolution_note: form.resolution_note || null,
        }),
      },
    );
    await load(true);
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "风险事件保存失败";
  } finally {
    saving.value = false;
  }
}

function page(direction: number) {
  offset.value = Math.max(0, offset.value + direction * limit);
  load();
}

function statusTone(value: RiskEvent["status"]): "success" | "warning" | "info" | "muted" {
  if (value === "resolved") return "success";
  if (value === "ignored") return "muted";
  if (value === "acknowledged") return "info";
  return "warning";
}

function severityTone(value: RiskEvent["severity"]): "danger" | "warning" | "info" {
  if (value === "danger") return "danger";
  if (value === "warning") return "warning";
  return "info";
}

useRealtimeEvents(["security.created", "security.updated"], () => load(true), true);
load();
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="SECURITY OPERATIONS"
      title="风险事件"
      description="集中查看登录、设备、播放与线路异常，并记录调查和处置结果。"
      :icon="ShieldAlert"
    >
      <template #actions>
        <button class="secondary-button" @click="load()">
          <RefreshCw :size="16" /> 刷新
        </button>
      </template>
    </AdminPageHeader>

    <div v-if="error" class="error-banner">
      <CircleAlert :size="17" /> {{ error }}
    </div>

    <section class="stats-grid admin-stats">
      <MetricCard
        label="待处理风险"
        :value="formatNumber(summary?.open_total)"
        caption="待处理与调查中的事件"
        :icon="ShieldAlert"
        tone="red"
        featured
      />
      <MetricCard
        label="高危事件"
        :value="formatNumber(summary?.severity_counts.danger)"
        caption="尚未关闭的高危事件"
        :icon="AlertTriangle"
        tone="gold"
      />
      <MetricCard
        label="最近 24 小时"
        :value="formatNumber(summary?.recent_24h)"
        caption="新产生的安全事件"
        :icon="Clock3"
        tone="violet"
      />
      <MetricCard
        label="已解决"
        :value="formatNumber(summary?.status_counts.resolved)"
        caption="已完成处置的事件"
        :icon="CheckCircle2"
        tone="green"
      />
    </section>

    <section class="panel table-panel">
      <FilterBar>
        <label class="search-box">
          <Search :size="18" />
          <input
            v-model.trim="search"
            placeholder="搜索事件类型、对象、IP 或详情"
            @keyup.enter="applyFilters"
          />
        </label>
        <label class="select-box">
          <select v-model="severity" @change="applyFilters">
            <option value="">全部级别</option>
            <option value="danger">高危</option>
            <option value="warning">警告</option>
            <option value="info">关注</option>
          </select>
        </label>
        <label class="select-box">
          <select v-model="status" @change="applyFilters">
            <option value="">全部状态</option>
            <option value="open">待处理</option>
            <option value="acknowledged">调查中</option>
            <option value="resolved">已解决</option>
            <option value="ignored">已忽略</option>
          </select>
        </label>
      </FilterBar>

      <LoadingBlock v-if="loading" />
      <div v-else-if="!items.length" class="risk-empty">
        <CheckCircle2 :size="30" />
        <strong>没有匹配的风险事件</strong>
        <p>当前筛选范围内没有需要处理的事件。</p>
      </div>
      <div v-else class="risk-list">
        <button
          v-for="item in items"
          :key="item.id"
          class="risk-row"
          type="button"
          @click="open(item)"
        >
          <span class="risk-mark" :data-severity="item.severity">
            <ShieldAlert :size="18" />
          </span>
          <span class="risk-main">
            <strong>{{ eventLabels[item.event_type] || item.event_type }}</strong>
            <small>
              {{ item.subject_kind || "system" }} ·
              {{ item.subject_id || "—" }}
              <template v-if="item.ip_address"> · {{ item.ip_address }}</template>
            </small>
          </span>
          <StatusBadge
            :label="severityLabels[item.severity]"
            :tone="severityTone(item.severity)"
          />
          <StatusBadge
            :label="statusLabels[item.status]"
            :tone="statusTone(item.status)"
          />
          <time>{{ formatDate(item.created_at) }}</time>
        </button>
      </div>
      <div class="pagination">
        <span>第 {{ currentPage }} / {{ pages }} 页 · {{ total }} 条</span>
        <div>
          <button :disabled="offset === 0" @click="page(-1)">
            <ChevronLeft :size="16" /> 上一页
          </button>
          <button :disabled="currentPage >= pages" @click="page(1)">
            下一页 <ChevronRight :size="16" />
          </button>
        </div>
      </div>
    </section>

    <DetailDrawer
      :open="Boolean(selected)"
      title="风险事件详情"
      eyebrow="INCIDENT DETAIL"
      :description="selected ? eventLabels[selected.event_type] || selected.event_type : ''"
      @close="selected = null"
    >
      <template v-if="selected">
        <div class="risk-detail-heading">
          <StatusBadge
            :label="severityLabels[selected.severity]"
            :tone="severityTone(selected.severity)"
          />
          <StatusBadge
            :label="statusLabels[selected.status]"
            :tone="statusTone(selected.status)"
          />
          <code>#{{ selected.id }}</code>
        </div>
        <dl class="detail-list boxed">
          <div><dt>事件类型</dt><dd>{{ selected.event_type }}</dd></div>
          <div><dt>风险对象</dt><dd>{{ selected.subject_kind || "—" }} · {{ selected.subject_id || "—" }}</dd></div>
          <div><dt>来源 IP</dt><dd>{{ selected.ip_address || "—" }}</dd></div>
          <div><dt>发现时间</dt><dd>{{ formatDate(selected.created_at) }}</dd></div>
          <div><dt>处理人</dt><dd>{{ selected.assigned_to || "未分派" }}</dd></div>
          <div><dt>完成时间</dt><dd>{{ formatDate(selected.resolved_at, "尚未完成") }}</dd></div>
        </dl>
        <section class="risk-evidence">
          <span class="section-kicker">EVENT EVIDENCE</span>
          <pre>{{ JSON.stringify(selected.detail || {}, null, 2) }}</pre>
        </section>
        <form v-if="canManage" class="risk-form" @submit.prevent="save">
          <label>
            <span>处理状态</span>
            <select v-model="form.status">
              <option value="open">待处理</option>
              <option value="acknowledged">调查中</option>
              <option value="resolved">已解决</option>
              <option value="ignored">已忽略</option>
            </select>
          </label>
          <label>
            <span>分派给 Telegram ID</span>
            <input v-model.trim="form.assigned_to" inputmode="numeric" placeholder="留空表示未分派" />
          </label>
          <label>
            <span>调查与处理说明</span>
            <textarea
              v-model.trim="form.resolution_note"
              maxlength="1000"
              placeholder="记录判断依据、处置动作和后续建议"
            />
          </label>
          <button class="primary-button wide" :disabled="saving">
            {{ saving ? "保存中…" : "保存处理结果" }}
          </button>
        </form>
      </template>
    </DetailDrawer>
  </div>
</template>

<style scoped>
.risk-list { display: grid; }
.risk-row { display: grid; grid-template-columns: auto minmax(260px, 1fr) auto auto 150px; gap: 14px; align-items: center; width: 100%; padding: 16px 20px; border: 0; border-top: 1px solid var(--border); color: inherit; text-align: left; background: transparent; cursor: pointer; }
.risk-row:hover { background: color-mix(in srgb, var(--pink) 5%, transparent); }
.risk-mark { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 12px; color: var(--muted); background: rgba(255,255,255,.04); }
.risk-mark[data-severity="danger"] { color: #ff6b7a; background: rgba(255, 68, 91, .1); }
.risk-mark[data-severity="warning"] { color: #e8a733; background: rgba(232, 167, 51, .1); }
.risk-main { display: grid; gap: 5px; min-width: 0; }
.risk-main strong, .risk-main small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.risk-main small, .risk-row time { color: var(--muted); font-size: 12px; }
.risk-empty { display: grid; justify-items: center; gap: 8px; padding: 60px 20px; color: var(--muted); }
.risk-empty strong { color: var(--text); }
.risk-empty p { margin: 0; }
.risk-detail-heading { display: flex; gap: 8px; align-items: center; }
.risk-detail-heading code { margin-left: auto; color: var(--muted); }
.risk-evidence { display: grid; gap: 10px; }
.risk-evidence pre { max-height: 260px; overflow: auto; margin: 0; padding: 14px; border-radius: 14px; color: var(--muted); background: #11131a; font-size: 12px; white-space: pre-wrap; }
.risk-form { display: grid; gap: 15px; }
.risk-form label { display: grid; gap: 7px; color: var(--muted); font-size: 13px; }
.risk-form input, .risk-form select, .risk-form textarea { width: 100%; }
.risk-form textarea { min-height: 120px; resize: vertical; }
@media (max-width: 900px) {
  .risk-row { grid-template-columns: auto 1fr auto; }
  .risk-row > :nth-child(4), .risk-row time { display: none; }
}
</style>
