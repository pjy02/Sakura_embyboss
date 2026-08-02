<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  Activity,
  CheckCircle2,
  Clock3,
  Database,
  Radio,
  RefreshCw,
  ServerCog,
  TriangleAlert,
  Wifi,
} from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import type { SystemStatus, WorkerStatus } from "@/types";

const system = ref<SystemStatus | null>(null);
const loading = ref(true);
const refreshing = ref(false);
const error = ref("");
let timer: number | undefined;

const onlineWorkers = computed(
  () => system.value?.workers.filter((worker) => !worker.stale && worker.status !== "stopped").length || 0,
);
const staleWorkers = computed(() => system.value?.workers.filter((worker) => worker.stale).length || 0);
const pendingTasks = computed(
  () => (system.value?.task_counts.pending || 0) + (system.value?.task_counts.retrying || 0),
);

function workerTone(worker: WorkerStatus): "success" | "warning" | "danger" {
  if (worker.stale) return "warning";
  if (worker.status === "stopped") return "danger";
  return "success";
}

function workerLabel(worker: WorkerStatus) {
  if (worker.stale) return "心跳延迟";
  if (worker.status === "stopped") return "已停止";
  if (worker.status === "busy") return "执行中";
  return "在线";
}

async function load(silent = false) {
  if (silent) refreshing.value = true;
  else loading.value = true;
  error.value = "";
  try {
    system.value = await api<SystemStatus>("/admin/system/status");
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "服务状态读取失败";
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
}

onMounted(() => {
  load();
  timer = window.setInterval(() => load(true), 15_000);
});
onBeforeUnmount(() => window.clearInterval(timer));
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="SERVICE RELIABILITY"
      title="服务状态"
      description="统一查看管理 API、数据库、任务处理器和实时事件中继的运行状态与心跳。"
      :icon="Activity"
    >
      <template #meta>
        <StatusBadge
          v-if="system"
          :label="system.status === 'healthy' ? '全部正常' : '部分服务需关注'"
          :tone="system.status === 'healthy' ? 'success' : 'warning'"
        />
      </template>
      <template #actions>
        <button class="secondary-button" :disabled="refreshing" @click="load(true)">
          <RefreshCw :size="16" :class="{ spin: refreshing }" /> 刷新状态
        </button>
      </template>
    </AdminPageHeader>

    <div v-if="error" class="error-banner"><TriangleAlert :size="17" /> {{ error }}</div>
    <LoadingBlock v-if="loading" />

    <template v-else-if="system">
      <section class="stats-grid status-metrics">
        <MetricCard
          label="整体状态"
          :value="system.status === 'healthy' ? '运行正常' : '需要关注'"
          :caption="`检查于 ${formatDate(system.checked_at)}`"
          :icon="system.status === 'healthy' ? CheckCircle2 : TriangleAlert"
          :tone="system.status === 'healthy' ? 'green' : 'gold'"
          featured
        />
        <MetricCard label="在线工作节点" :value="formatNumber(onlineWorkers)" :caption="`${staleWorkers} 个心跳延迟`" :icon="ServerCog" tone="cyan" />
        <MetricCard label="等待任务" :value="formatNumber(pendingTasks)" :caption="system.oldest_pending_at ? `最早 ${formatDate(system.oldest_pending_at)}` : '当前无积压'" :icon="Clock3" tone="violet" />
        <MetricCard label="失败任务" :value="formatNumber(system.task_counts.failed)" caption="保留记录，可在系统任务中重试" :icon="TriangleAlert" :tone="system.task_counts.failed ? 'red' : 'green'" />
      </section>

      <section class="service-grid">
        <article class="service-card healthy">
          <span><Database :size="21" /></span>
          <div><small>DATABASE</small><strong>数据库连接</strong><p>状态接口查询成功，业务数据库可读。</p></div>
          <StatusBadge label="正常" tone="success" />
        </article>
        <article class="service-card healthy">
          <span><Wifi :size="21" /></span>
          <div><small>ADMIN API</small><strong>管理 API</strong><p>身份、权限与状态接口响应正常。</p></div>
          <StatusBadge label="正常" tone="success" />
        </article>
        <article class="service-card" :class="system.components.task_worker">
          <span><ServerCog :size="21" /></span>
          <div><small>TASK WORKER</small><strong>任务处理器</strong><p>{{ system.components.task_worker === "healthy" ? "至少一个任务节点心跳正常。" : "当前没有活跃任务节点。" }}</p></div>
          <StatusBadge :label="system.components.task_worker === 'healthy' ? '正常' : '需关注'" :tone="system.components.task_worker === 'healthy' ? 'success' : 'warning'" />
        </article>
        <article class="service-card" :class="system.components.event_relay">
          <span><Radio :size="21" /></span>
          <div><small>EVENT RELAY</small><strong>实时事件中继</strong><p>{{ system.components.event_relay === "healthy" ? "事件中继节点心跳正常。" : "当前没有活跃事件中继节点。" }}</p></div>
          <StatusBadge :label="system.components.event_relay === 'healthy' ? '正常' : '需关注'" :tone="system.components.event_relay === 'healthy' ? 'success' : 'warning'" />
        </article>
      </section>

      <section class="status-columns">
        <article class="panel">
          <div class="panel-heading">
            <div><span class="section-kicker">WORKER HEARTBEATS</span><h2>工作节点</h2></div>
            <StatusBadge :label="`${onlineWorkers} 在线 / ${staleWorkers} 延迟`" :tone="staleWorkers ? 'warning' : 'success'" />
          </div>
          <EmptyState
            v-if="!system.workers.length"
            title="暂无工作节点"
            description="任务处理器或事件中继完成首次心跳后会显示在这里。"
          />
          <div v-else class="worker-list">
            <article v-for="worker in system.workers" :key="worker.worker_id">
              <i class="worker-light" :data-tone="workerTone(worker)" />
              <div class="worker-copy">
                <div><strong>{{ worker.worker_id }}</strong><StatusBadge :label="workerLabel(worker)" :tone="workerTone(worker)" /></div>
                <p>{{ worker.worker_kind }} · {{ worker.hostname }} · PID {{ worker.process_id }}</p>
                <small v-if="worker.current_task_id">当前任务 {{ worker.current_task_id }}</small>
              </div>
              <time>{{ formatDate(worker.last_seen_at) }}</time>
            </article>
          </div>
        </article>

        <article class="panel">
          <div class="panel-heading"><div><span class="section-kicker">TASK QUEUE</span><h2>任务队列</h2></div></div>
          <div class="queue-grid">
            <div><span>等待中</span><strong>{{ formatNumber(system.task_counts.pending) }}</strong></div>
            <div><span>重试中</span><strong>{{ formatNumber(system.task_counts.retrying) }}</strong></div>
            <div><span>执行中</span><strong>{{ formatNumber(system.task_counts.running) }}</strong></div>
            <div><span>已完成</span><strong>{{ formatNumber(system.task_counts.succeeded) }}</strong></div>
            <div><span>已失败</span><strong class="negative-text">{{ formatNumber(system.task_counts.failed) }}</strong></div>
            <div><span>已取消</span><strong>{{ formatNumber(system.task_counts.canceled) }}</strong></div>
          </div>
        </article>
      </section>
    </template>
  </div>
</template>

<style scoped>
.status-metrics { grid-template-columns: repeat(4, minmax(0,1fr)); }
.service-grid { display: grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap: 13px; }
.service-card { display: grid; grid-template-columns: auto minmax(0,1fr); gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: 14px; background: var(--surface); }
.service-card > span { display: grid; place-items: center; width: 39px; height: 39px; color: var(--orange); border-radius: 12px; background: rgba(241,168,91,.08); }
.service-card.healthy > span { color: var(--green); background: rgba(113,211,155,.08); }
.service-card > .admin-status-badge { grid-column: 2; }
.service-card small { color: var(--muted-2); font-size: 8px; letter-spacing: .14em; }
.service-card strong { display: block; margin-top: 4px; }
.service-card p { overflow: hidden; margin: 5px 0 0; color: var(--muted); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.status-columns { display: grid; grid-template-columns: minmax(0,1.25fr) minmax(300px,.75fr); gap: 17px; }
.worker-list { display: grid; }
.worker-list > article { display: flex; align-items: center; gap: 12px; padding: 15px 19px; border-top: 1px solid var(--border); }
.worker-light { width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; background: var(--green); box-shadow: 0 0 9px var(--green); }
.worker-light[data-tone="warning"] { background: var(--orange); box-shadow: 0 0 9px var(--orange); }
.worker-light[data-tone="danger"] { background: var(--red); box-shadow: 0 0 9px var(--red); }
.worker-copy { min-width: 0; flex: 1; }
.worker-copy > div { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.worker-copy p, .worker-copy small { margin: 5px 0 0; color: var(--muted); font-size: 11px; }
.worker-list time { color: var(--muted-2); font-size: 10px; white-space: nowrap; }
.queue-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 10px; padding: 18px; border-top: 1px solid var(--border); }
.queue-grid div { padding: 14px; border-radius: 11px; background: rgba(255,255,255,.025); }
.queue-grid span { color: var(--muted); font-size: 10px; }
.queue-grid strong { display: block; margin-top: 7px; font-size: 21px; }
@media (max-width: 1100px) {
  .status-metrics, .service-grid { grid-template-columns: repeat(2,minmax(0,1fr)); }
  .status-columns { grid-template-columns: 1fr; }
}
@media (max-width: 620px) {
  .status-metrics, .service-grid { grid-template-columns: 1fr; }
  .worker-list time { display: none; }
}
</style>
