<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Activity, Play, Plus, RefreshCw, Trash2, Workflow } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { AutomationRule, AutomationRun } from "@/types";

const session = useSessionStore();
const rules = ref<AutomationRule[]>([]);
const runs = ref<AutomationRun[]>([]);
const loading = ref(true);
const editorOpen = ref(false);
const editingId = ref<string | null>(null);
const busy = ref(false);
const error = ref("");
const form = reactive({ name: "", description: "", trigger_type: "event" as "event" | "interval", trigger_value: "risk.*", conditions: "{}", action_type: "enqueue_task" as "enqueue_task" | "telegram_alert" | "create_risk_event", action_value: "monitor.diagnostics", action_title: "Sakura 自动化告警", cooldown_seconds: 60, enabled: true, revision: undefined as number | undefined });
const canManage = computed(() => session.session?.permissions.some((item) => item === "*" || item === "automation:*" || item === "automation:manage"));
const successRuns = computed(() => runs.value.filter((item) => item.status === "succeeded").length);

async function load() {
  loading.value = true;
  try {
    const result = await api<{ items: AutomationRule[]; runs: AutomationRun[] }>("/admin/automations");
    rules.value = result.items;
    runs.value = result.runs;
  } finally {
    loading.value = false;
  }
}

function openEditor(item?: AutomationRule) {
  editingId.value = item?.id ?? null;
  const action = item?.actions[0] || {};
  Object.assign(form, item ? { name: item.name, description: item.description || "", trigger_type: item.trigger_type, trigger_value: item.trigger_value, conditions: JSON.stringify(item.conditions, null, 2), action_type: String(action.type || "enqueue_task"), action_value: String(action.task_type || action.event_type || "monitor.diagnostics"), action_title: String(action.title || "Sakura 自动化告警"), cooldown_seconds: item.cooldown_seconds, enabled: item.enabled, revision: item.revision } : { name: "", description: "", trigger_type: "event", trigger_value: "risk.*", conditions: "{}", action_type: "enqueue_task", action_value: "monitor.diagnostics", action_title: "Sakura 自动化告警", cooldown_seconds: 60, enabled: true, revision: undefined });
  error.value = "";
  editorOpen.value = true;
}

function actionPayload() {
  if (form.action_type === "enqueue_task") return { type: "enqueue_task", task_type: form.action_value, payload: {} };
  if (form.action_type === "telegram_alert") return { type: "telegram_alert", title: form.action_title, body: form.action_value };
  return { type: "create_risk_event", event_type: form.action_value, severity: "warning" };
}

async function save() {
  error.value = "";
  let conditions: Record<string, unknown>;
  try { conditions = JSON.parse(form.conditions || "{}"); } catch { error.value = "条件必须是合法 JSON"; return; }
  busy.value = true;
  try {
    await api(editingId.value ? `/admin/automations/${editingId.value}` : "/admin/automations", { method: editingId.value ? "PATCH" : "POST", body: JSON.stringify({ name: form.name, description: form.description, trigger_type: form.trigger_type, trigger_value: form.trigger_value, conditions, actions: [actionPayload()], cooldown_seconds: form.cooldown_seconds, enabled: form.enabled, revision: form.revision }) });
    editorOpen.value = false;
    await load();
  } catch (value) { error.value = value instanceof Error ? value.message : "保存失败"; } finally { busy.value = false; }
}

async function remove(item: AutomationRule) {
  if (!window.confirm(`删除自动化“${item.name}”？`)) return;
  await api(`/admin/automations/${item.id}`, { method: "DELETE" });
  await load();
}

async function evaluate() {
  busy.value = true;
  try { await api("/admin/automations/evaluate", { method: "POST" }); await load(); } finally { busy.value = false; }
}

onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="AUTOMATION ENGINE" title="自动化中心" description="基于系统事件或时间间隔，自动创建任务、风险事件和 Telegram 告警；每次执行均有去重和运行记录。" :icon="Workflow"><template #actions><button v-if="canManage" class="secondary-button" :disabled="busy" @click="evaluate"><Play :size="16" />立即扫描</button><button v-if="canManage" class="primary-button" @click="openEditor()"><Plus :size="16" />新建自动化</button></template></AdminPageHeader>
    <div class="metric-grid compact-grid"><MetricCard label="自动化规则" :value="formatNumber(rules.length)" caption="启用与停用规则" :icon="Workflow" /><MetricCard label="已启用" :value="formatNumber(rules.filter(item => item.enabled).length)" caption="由独立 Worker 执行" :icon="Activity" tone="green" /><MetricCard label="成功执行" :value="formatNumber(successRuns)" caption="最近 100 次运行" :icon="RefreshCw" tone="cyan" /></div>
    <section class="panel rule-grid"><article v-for="item in rules" :key="item.id" class="automation-card"><header><div><span class="trigger-chip">{{ item.trigger_type === 'event' ? '事件' : '间隔' }}</span><h3>{{ item.name }}</h3></div><StatusBadge :label="item.enabled ? '运行中' : '已停用'" :tone="item.enabled ? 'success' : 'muted'" /></header><p>{{ item.description || "暂无说明" }}</p><dl><div><dt>触发条件</dt><dd><code>{{ item.trigger_value }}</code></dd></div><div><dt>执行动作</dt><dd>{{ item.actions.map(action => action.type).join('、') }}</dd></div><div><dt>冷却</dt><dd>{{ item.cooldown_seconds }} 秒</dd></div><div><dt>上次执行</dt><dd>{{ formatDate(item.last_run_at) }}</dd></div></dl><footer><button v-if="canManage" class="text-button" @click="openEditor(item)">编辑</button><button v-if="canManage" class="text-button danger-text" @click="remove(item)"><Trash2 :size="14" />删除</button></footer></article><div v-if="!loading && !rules.length" class="empty-inline"><Workflow :size="28" /><strong>还没有自动化规则</strong><p>从诊断、风险或业务事件开始创建第一条规则。</p></div></section>
    <section class="panel table-panel"><div class="section-heading"><div><span class="section-kicker">RUN HISTORY</span><h2>执行记录</h2></div></div><AdminDataTable :loading="loading" :empty="!runs.length" empty-title="暂无执行记录" empty-description="规则触发后会保留结果。" min-width="760px"><template #head><tr><th>规则</th><th>来源事件</th><th>状态</th><th>开始时间</th><th>错误</th></tr></template><template #body><tr v-for="run in runs" :key="run.id"><td>{{ rules.find(item => item.id === run.rule_id)?.name || run.rule_id }}</td><td>{{ run.event_id || "定时触发" }}</td><td><StatusBadge :label="run.status" :tone="run.status === 'succeeded' ? 'success' : run.status === 'failed' ? 'danger' : 'info'" /></td><td>{{ formatDate(run.started_at) }}</td><td>{{ run.error_message || "—" }}</td></tr></template></AdminDataTable></section>
    <div v-if="editorOpen" class="modal-layer"><form class="modal-card" @submit.prevent="save"><header><div><span class="section-kicker">AUTOMATION RULE</span><h2>{{ editingId ? "编辑自动化" : "新建自动化" }}</h2></div><button type="button" class="icon-button" @click="editorOpen = false">×</button></header><label><span>名称</span><input v-model.trim="form.name" required maxlength="120" /></label><label><span>说明</span><input v-model.trim="form.description" maxlength="500" /></label><div class="form-grid"><label><span>触发方式</span><select v-model="form.trigger_type"><option value="event">系统事件</option><option value="interval">时间间隔</option></select></label><label><span>{{ form.trigger_type === 'event' ? '事件模式' : '间隔秒数' }}</span><input v-model.trim="form.trigger_value" required :placeholder="form.trigger_type === 'event' ? '例如 risk.*' : '例如 3600'" /></label></div><label><span>条件 JSON</span><textarea v-model="form.conditions" class="code-area" /></label><div class="form-grid"><label><span>执行动作</span><select v-model="form.action_type"><option value="enqueue_task">创建后台任务</option><option value="telegram_alert">Telegram 告警</option><option value="create_risk_event">创建风险事件</option></select></label><label><span>{{ form.action_type === 'enqueue_task' ? '任务类型' : form.action_type === 'telegram_alert' ? '告警正文' : '风险事件类型' }}</span><input v-model.trim="form.action_value" required /></label></div><label v-if="form.action_type === 'telegram_alert'"><span>告警标题</span><input v-model.trim="form.action_title" /></label><div class="form-grid"><label><span>冷却秒数</span><input v-model.number="form.cooldown_seconds" type="number" min="0" max="604800" /></label><label class="check-row"><input v-model="form.enabled" type="checkbox" /><span>立即启用</span></label></div><p v-if="error" class="form-error">{{ error }}</p><footer><button type="button" class="secondary-button" @click="editorOpen = false">取消</button><button class="primary-button" :disabled="busy">{{ busy ? "保存中…" : "保存自动化" }}</button></footer></form></div>
  </div>
</template>

<style scoped>
.rule-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:14px;padding:18px}.automation-card{padding:18px;border:1px solid var(--border);border-radius:16px;background:var(--surface-subtle)}.automation-card header,.automation-card footer{display:flex;justify-content:space-between;align-items:center;gap:12px}.automation-card h3{margin:7px 0 0}.automation-card>p{color:var(--text-muted);min-height:42px}.automation-card dl{display:grid;grid-template-columns:1fr 1fr;gap:10px}.automation-card dl div{padding:10px;border-radius:10px;background:var(--surface)}.automation-card dt{font-size:11px;color:var(--text-muted)}.automation-card dd{margin:5px 0 0;font-size:13px}.automation-card footer{margin-top:14px;justify-content:flex-end}.trigger-chip{font-size:10px;font-weight:800;letter-spacing:.08em;color:var(--primary)}.danger-text{color:var(--danger)}.code-area{font-family:ui-monospace,monospace;min-height:100px}.empty-inline{grid-column:1/-1;text-align:center;padding:42px;color:var(--text-muted)}.empty-inline strong{display:block;margin-top:8px;color:var(--text)}
</style>
