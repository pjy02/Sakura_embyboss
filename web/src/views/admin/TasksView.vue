<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  Activity,
  Ban,
  CheckCircle2,
  CircleAlert,
  Clock3,
  DatabaseBackup,
  LoaderCircle,
  Play,
  Radio,
  RefreshCw,
  RotateCcw,
  ServerCog,
  X,
} from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import { api, idempotencyKey } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { JobRun, OperationTask, SystemStatus, TaskDefinition } from "@/types";

const sessionStore = useSessionStore();
const tasks = ref<OperationTask[]>([]);
const definitions = ref<TaskDefinition[]>([]);
const jobs = ref<JobRun[]>([]);
const system = ref<SystemStatus | null>(null);
const loading = ref(true);
const tab = ref<"tasks" | "jobs">("tasks");
const statusFilter = ref("");
const connected = ref(false);
const selectedDefinition = ref<TaskDefinition | null>(null);
const confirmed = ref(false);
const busy = ref("");
const error = ref("");
let stream: EventSource | null = null;
let timer: number | undefined;

const canUpdate = computed(() =>
  sessionStore.session?.permissions.some(
    (item) => item === "*" || item === "tasks:*" || item === "tasks:update",
  ),
);
const activeCount = computed(
  () => (system.value?.task_counts.pending || 0) + (system.value?.task_counts.retrying || 0) + (system.value?.task_counts.running || 0),
);

const statusLabels: Record<string, string> = {
  pending: "等待执行",
  retrying: "等待重试",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  canceled: "已取消",
  missed: "错过执行",
};

async function load(silent = false) {
  if (!silent) loading.value = true;
  const suffix = statusFilter.value ? `?status=${statusFilter.value}` : "";
  try {
    const [taskResult, definitionResult, jobResult, statusResult] = await Promise.all([
      api<{ items: OperationTask[] }>(`/admin/tasks${suffix}`),
      api<{ items: TaskDefinition[] }>("/admin/task-definitions"),
      api<{ items: JobRun[] }>("/admin/jobs?limit=30"),
      api<SystemStatus>("/admin/system/status"),
    ]);
    tasks.value = taskResult.items;
    definitions.value = definitionResult.items;
    jobs.value = jobResult.items;
    system.value = statusResult;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "任务状态加载失败";
  } finally {
    loading.value = false;
  }
}

async function enqueue() {
  if (!selectedDefinition.value) return;
  busy.value = "enqueue";
  error.value = "";
  try {
    await api("/admin/tasks", {
      method: "POST",
      idempotencyKey: idempotencyKey(selectedDefinition.value.task_type),
      body: JSON.stringify({
        task_type: selectedDefinition.value.task_type,
        payload: {},
        confirm: selectedDefinition.value.risk === "normal" || confirmed.value,
      }),
    });
    selectedDefinition.value = null;
    confirmed.value = false;
    await load(true);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "任务创建失败";
  } finally {
    busy.value = "";
  }
}

async function taskAction(task: OperationTask, action: "cancel" | "retry") {
  busy.value = `${action}:${task.id}`;
  error.value = "";
  try {
    await api(`/admin/tasks/${task.id}/${action}`, { method: "POST" });
    await load(true);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "任务操作失败";
  } finally {
    busy.value = "";
  }
}

function openDefinition(item: TaskDefinition) {
  selectedDefinition.value = item;
  confirmed.value = item.risk === "normal";
  error.value = "";
}

function connectEvents() {
  stream?.close();
  stream = new EventSource("/api/v1/admin/events/stream");
  stream.onopen = () => (connected.value = true);
  stream.onerror = () => (connected.value = false);
  const refresh = () => load(true);
  stream.addEventListener("task.created", refresh);
  stream.addEventListener("task.updated", refresh);
}

function statusIcon(status: string) {
  if (status === "succeeded") return CheckCircle2;
  if (status === "running") return LoaderCircle;
  if (status === "failed") return CircleAlert;
  if (status === "canceled") return Ban;
  return Clock3;
}

onMounted(async () => {
  await load();
  connectEvents();
  timer = window.setInterval(() => load(true), 15_000);
});
onBeforeUnmount(() => {
  stream?.close();
  window.clearInterval(timer);
});
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="RELIABLE OPERATIONS"
      title="系统任务"
      description="耗时操作在后台可靠执行；断电或重启后可自动恢复，并实时同步进度与结果。"
      :icon="ServerCog"
    >
      <template #actions><button class="secondary-button" @click="load()"><RefreshCw :size="16" /> 刷新状态</button></template>
    </AdminPageHeader>

    <div v-if="error" class="error-banner"><CircleAlert :size="17" /> {{ error }}</div>

    <section class="reliability-strip">
      <div>
        <span class="reliability-icon" :class="system?.status"><ServerCog :size="21" /></span>
        <div><small>任务执行器</small><strong>{{ system?.status === "healthy" ? "运行正常" : "等待 Worker 连接" }}</strong></div>
      </div>
      <div><small>活跃任务</small><strong>{{ activeCount }}</strong></div>
      <div><small>失败任务</small><strong :class="{ 'negative-text': system?.task_counts.failed }">{{ system?.task_counts.failed || 0 }}</strong></div>
      <div class="live-connection"><Radio :size="15" /><span>{{ connected ? "实时通道已连接" : "实时通道重连中" }}</span><i :class="{ connected }" /></div>
    </section>

    <section v-if="canUpdate" class="task-launch-grid">
      <article v-for="item in definitions" :key="item.task_type" :data-risk="item.risk">
        <span><DatabaseBackup v-if="item.task_type.includes('backup')" :size="20" /><Activity v-else :size="20" /></span>
        <div><strong>{{ item.label }}</strong><p>{{ item.description }}</p><small>超时 {{ Math.round(item.timeout_seconds / 60) }} 分钟 · 最多重试 {{ item.max_retries }} 次</small></div>
        <button class="secondary-button" @click="openDefinition(item)"><Play :size="15" /> 运行</button>
      </article>
    </section>

    <div class="segmented-tabs">
      <button :class="{ active: tab === 'tasks' }" @click="tab = 'tasks'">后台任务</button>
      <button :class="{ active: tab === 'jobs' }" @click="tab = 'jobs'">定时任务记录</button>
    </div>

    <section class="panel table-panel">
      <template v-if="tab === 'tasks'">
        <div class="panel-heading">
          <div><span class="section-kicker">TASK QUEUE</span><h2>执行队列</h2></div>
          <select v-model="statusFilter" class="compact-select" @change="load()">
            <option value="">全部状态</option><option value="pending,running,retrying">进行中</option>
            <option value="succeeded">已完成</option><option value="failed">失败</option><option value="canceled">已取消</option>
          </select>
        </div>
        <LoadingBlock v-if="loading" />
        <EmptyState v-else-if="!tasks.length" title="当前没有任务" description="从上方选择一个任务开始执行。" />
        <div v-else class="task-list">
          <article v-for="task in tasks" :key="task.id">
            <span class="task-status-icon" :data-status="task.status">
              <component :is="statusIcon(task.status)" :class="{ spin: task.status === 'running' }" :size="18" />
            </span>
            <div class="task-main">
              <div><strong>{{ task.label }}</strong><span class="status-badge" :class="task.status">{{ statusLabels[task.status] }}</span></div>
              <p>{{ formatDate(task.created_at) }} · {{ task.owner_kind }} {{ task.owner_id }}</p>
              <div v-if="task.status === 'running'" class="progress-track"><i :style="{ width: `${Math.max(task.progress, 4)}%` }" /></div>
              <small v-if="task.error_message" class="task-error">{{ task.error_message }}</small>
            </div>
            <div class="task-meta"><code>{{ task.id.slice(0, 8) }}</code><span v-if="task.retry_count">重试 {{ task.retry_count }}/{{ task.max_retries }}</span></div>
            <div v-if="canUpdate" class="task-actions">
              <button v-if="['pending','retrying','running'].includes(task.status)" :disabled="Boolean(busy)" title="取消任务" @click="taskAction(task, 'cancel')"><X :size="15" /></button>
              <button v-if="['failed','canceled'].includes(task.status)" :disabled="Boolean(busy)" title="重新执行" @click="taskAction(task, 'retry')"><RotateCcw :size="15" /></button>
            </div>
          </article>
        </div>
      </template>

      <template v-else>
        <div class="panel-heading"><div><span class="section-kicker">SCHEDULER HISTORY</span><h2>定时任务运行记录</h2></div></div>
        <LoadingBlock v-if="loading" />
        <EmptyState v-else-if="!jobs.length" title="还没有运行记录" description="定时任务完成、失败或错过执行后会显示在这里。" />
        <div v-else class="responsive-table">
          <table>
            <thead><tr><th>任务</th><th>触发方式</th><th>状态</th><th>计划时间</th><th>完成时间</th><th>错误</th></tr></thead>
            <tbody><tr v-for="job in jobs" :key="job.id"><td class="strong-cell">{{ job.job_name }}</td><td>{{ job.trigger_kind }}</td><td><span class="status-badge" :class="job.status">{{ statusLabels[job.status] || job.status }}</span></td><td>{{ formatDate(job.started_at) }}</td><td>{{ formatDate(job.finished_at) }}</td><td class="job-error">{{ job.error_message || "—" }}</td></tr></tbody>
          </table>
        </div>
      </template>
    </section>

    <div v-if="selectedDefinition" class="modal-layer">
      <form class="modal-card" @submit.prevent="enqueue">
        <header><div><span class="section-kicker">RUN BACKGROUND TASK</span><h2>{{ selectedDefinition.label }}</h2></div><button type="button" class="icon-button" @click="selectedDefinition = null"><X :size="19" /></button></header>
        <p class="modal-context">{{ selectedDefinition.description }}</p>
        <div class="task-confirm-note" :data-risk="selectedDefinition.risk">
          <CircleAlert :size="18" /><span>{{ selectedDefinition.risk === "danger" ? "此任务可能修改用户和 Emby 账户状态，请确认当前适合执行。" : selectedDefinition.risk === "warning" ? "任务会修改外部或持久化数据，请确认后执行。" : "任务将进入可靠队列，可在执行列表查看结果。" }}</span>
        </div>
        <label v-if="selectedDefinition.risk !== 'normal'" class="check-row"><input v-model="confirmed" type="checkbox" /><span>我已了解影响并确认执行</span></label>
        <footer><button type="button" class="secondary-button" @click="selectedDefinition = null">取消</button><button class="primary-button" :disabled="!confirmed || busy === 'enqueue'"><LoaderCircle v-if="busy === 'enqueue'" class="spin" :size="16" /><Play v-else :size="16" />加入执行队列</button></footer>
      </form>
    </div>
  </div>
</template>
