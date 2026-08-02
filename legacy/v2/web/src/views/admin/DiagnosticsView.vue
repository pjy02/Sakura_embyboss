<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Activity, BellRing, CheckCircle2, CirclePlus, RefreshCw, Save, ShieldCheck, Stethoscope, TriangleAlert, X } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api, idempotencyKey } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { AlertDelivery, DiagnosticSummary, OperationTask, RiskRule } from "@/types";

const session = useSessionStore();
const summary = ref<DiagnosticSummary | null>(null);
const rules = ref<RiskRule[]>([]);
const alerts = ref<AlertDelivery[]>([]);
const loading = ref(true);
const running = ref(false);
const saving = ref(false);
const error = ref("");
const notice = ref("");
const modalOpen = ref(false);
const editing = ref<RiskRule | null>(null);
const form = reactive({ name: "", event_pattern: "", severity: "warning" as RiskRule["severity"], threshold_count: 1, window_minutes: 10, cooldown_minutes: 30, enabled: true, telegram_alert: true });

function allowed(permission: string) {
  return Boolean(session.session?.permissions.some((item) => item === "*" || item === permission || item === `${permission.split(":")[0]}:*`));
}
const canReadSecurity = computed(() => allowed("security:read"));
const canManageRules = computed(() => allowed("security:manage"));
const canRun = computed(() => allowed("tasks:update"));
const healthyCount = computed(() => summary.value?.services.filter((item) => item.status === "healthy").length || 0);
const failedCount = computed(() => summary.value?.services.filter((item) => item.status === "unhealthy").length || 0);
const failedAlerts = computed(() => alerts.value.filter((item) => item.status === "failed").length);

async function load(silent = false) {
  if (!silent) loading.value = true;
  error.value = "";
  try {
    summary.value = await api<DiagnosticSummary>("/admin/diagnostics?history_limit=60");
    if (canReadSecurity.value) {
      const [ruleResult, alertResult] = await Promise.all([
        api<{ items: RiskRule[] }>("/admin/risk/rules"),
        api<{ items: AlertDelivery[] }>("/admin/alerts/deliveries?limit=30"),
      ]);
      rules.value = ruleResult.items;
      alerts.value = alertResult.items;
    }
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "诊断数据读取失败";
  } finally {
    loading.value = false;
  }
}

async function runDiagnostics() {
  running.value = true;
  notice.value = "";
  error.value = "";
  try {
    const task = await api<OperationTask>("/admin/diagnostics/run", { method: "POST", idempotencyKey: idempotencyKey("diagnostics") });
    notice.value = `诊断任务已进入队列：${task.id}`;
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "诊断任务创建失败";
  } finally {
    running.value = false;
  }
}

function openRule(rule?: RiskRule) {
  editing.value = rule || null;
  Object.assign(form, rule ? {
    name: rule.name,
    event_pattern: rule.event_pattern,
    severity: rule.severity,
    threshold_count: rule.threshold_count,
    window_minutes: rule.window_minutes,
    cooldown_minutes: rule.cooldown_minutes,
    enabled: rule.enabled,
    telegram_alert: rule.telegram_alert,
  } : { name: "", event_pattern: "", severity: "warning", threshold_count: 1, window_minutes: 10, cooldown_minutes: 30, enabled: true, telegram_alert: true });
  modalOpen.value = true;
}

async function saveRule() {
  saving.value = true;
  error.value = "";
  try {
    const payload = { ...form };
    if (editing.value) {
      await api(`/admin/risk/rules/${editing.value.id}`, { method: "PATCH", body: JSON.stringify({ ...payload, expected_revision: editing.value.revision }) });
    } else {
      await api("/admin/risk/rules", { method: "POST", body: JSON.stringify(payload) });
    }
    modalOpen.value = false;
    await load(true);
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "风险规则保存失败";
  } finally {
    saving.value = false;
  }
}

async function toggleRule(rule: RiskRule, field: "enabled" | "telegram_alert") {
  const payload = { name: rule.name, event_pattern: rule.event_pattern, severity: rule.severity, threshold_count: rule.threshold_count, window_minutes: rule.window_minutes, cooldown_minutes: rule.cooldown_minutes, enabled: rule.enabled, telegram_alert: rule.telegram_alert, expected_revision: rule.revision, [field]: !rule[field] };
  try {
    await api(`/admin/risk/rules/${rule.id}`, { method: "PATCH", body: JSON.stringify(payload) });
    await load(true);
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "风险规则更新失败";
  }
}

function serviceLabel(name: string) {
  return ({ database: "业务数据库", emby: "Emby 服务", telegram: "Telegram API", moviepilot: "MoviePilot" } as Record<string, string>)[name] || name;
}
function alertTone(status: AlertDelivery["status"]): "success" | "warning" | "danger" {
  return status === "sent" ? "success" : status === "failed" ? "danger" : "warning";
}

onMounted(load);
useRealtimeEvents(["task.updated", "security.created", "security.rule.updated", "service.probe.recovered"], () => load(true), true);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="RELIABILITY CONTROL" title="诊断中心" description="主动探测数据库、Emby、Telegram 与外部集成，并把异常自动关联到风险规则和 Telegram 告警。" :icon="Stethoscope">
      <template #meta><StatusBadge v-if="summary" :label="!summary.services.length ? '等待首次探测' : summary.status === 'healthy' ? '全部探测正常' : '发现异常服务'" :tone="!summary.services.length ? 'warning' : summary.status === 'healthy' ? 'success' : 'danger'" /></template>
      <template #actions><button v-if="canRun" class="primary-button" :disabled="running" @click="runDiagnostics"><RefreshCw :size="16" :class="{ spin: running }" />{{ running ? "正在提交…" : "立即诊断" }}</button></template>
    </AdminPageHeader>

    <div v-if="error" class="error-banner"><TriangleAlert :size="17" />{{ error }}</div>
    <div v-if="notice" class="diagnostic-notice"><CheckCircle2 :size="17" />{{ notice }}</div>

    <section class="stats-grid admin-stats">
      <MetricCard label="健康服务" :value="formatNumber(healthyCount)" caption="最近一次探测成功" :icon="CheckCircle2" tone="green" featured />
      <MetricCard label="异常服务" :value="formatNumber(failedCount)" caption="已自动进入风险判定" :icon="TriangleAlert" :tone="failedCount ? 'red' : 'green'" />
      <MetricCard label="启用规则" :value="formatNumber(rules.filter((item) => item.enabled).length)" caption="按阈值与冷却时间触发" :icon="ShieldCheck" tone="violet" />
      <MetricCard label="告警失败" :value="formatNumber(failedAlerts)" caption="任务队列会按策略重试" :icon="BellRing" :tone="failedAlerts ? 'red' : 'cyan'" />
    </section>

    <section class="probe-grid">
      <article v-for="service in summary?.services || []" :key="service.service_name" class="panel probe-card" :data-status="service.status">
        <span><Activity :size="21" /></span><div><small>{{ service.service_kind.toUpperCase() }}</small><strong>{{ serviceLabel(service.service_name) }}</strong><p>{{ service.message || "尚无探测说明" }}</p></div>
        <StatusBadge :label="service.status === 'healthy' ? '正常' : '异常'" :tone="service.status === 'healthy' ? 'success' : 'danger'" />
        <footer><span>{{ service.latency_ms ?? "—" }} ms</span><time>{{ formatDate(service.checked_at) }}</time></footer>
      </article>
      <article v-if="!loading && !summary?.services.length" class="panel probe-empty">尚未执行诊断，点击“立即诊断”生成首份服务快照。</article>
    </section>

    <section v-if="canReadSecurity" class="panel rules-panel">
      <div class="panel-heading"><div><span class="section-kicker">RISK AUTOMATION</span><h2>风险规则</h2></div><button v-if="canManageRules" class="secondary-button" @click="openRule()"><CirclePlus :size="16" />新增规则</button></div>
      <div class="rule-list">
        <article v-for="rule in rules" :key="rule.id">
          <button class="rule-main" type="button" :disabled="!canManageRules" @click="openRule(rule)"><span :data-severity="rule.severity"><ShieldCheck :size="18" /></span><div><strong>{{ rule.name }}</strong><code>{{ rule.event_pattern }}</code><p>{{ rule.window_minutes }} 分钟内达到 {{ rule.threshold_count }} 次，冷却 {{ rule.cooldown_minutes }} 分钟</p></div></button>
          <label><input type="checkbox" :checked="rule.telegram_alert" :disabled="!canManageRules" @change="toggleRule(rule, 'telegram_alert')" /> Telegram</label>
          <label><input type="checkbox" :checked="rule.enabled" :disabled="!canManageRules" @change="toggleRule(rule, 'enabled')" /> 启用</label>
        </article>
      </div>
    </section>

    <section class="diagnostic-columns">
      <article class="panel table-panel">
        <div class="panel-heading section-pad"><div><span class="section-kicker">PROBE HISTORY</span><h2>探测历史</h2></div></div>
        <AdminDataTable :loading="loading" :empty="!summary?.history.length" empty-title="暂无探测记录" min-width="650px">
          <template #head><tr><th>服务</th><th>状态</th><th>耗时</th><th>响应</th><th>检查时间</th></tr></template>
          <template #body><tr v-for="probe in summary?.history || []" :key="probe.id"><td><strong>{{ serviceLabel(probe.service_name) }}</strong></td><td><StatusBadge :label="probe.status === 'healthy' ? '正常' : '异常'" :tone="probe.status === 'healthy' ? 'success' : 'danger'" /></td><td>{{ probe.latency_ms ?? "—" }} ms</td><td>{{ probe.message || probe.status_code || "—" }}</td><td>{{ formatDate(probe.checked_at) }}</td></tr></template>
        </AdminDataTable>
      </article>
      <article v-if="canReadSecurity" class="panel alert-panel">
        <div class="panel-heading section-pad"><div><span class="section-kicker">ALERT DELIVERY</span><h2>告警送达</h2></div></div>
        <div v-if="!alerts.length" class="alert-empty">暂无安全告警送达记录</div>
        <div v-else class="alert-list"><article v-for="item in alerts.slice(0, 12)" :key="item.id"><span><BellRing :size="16" /></span><div><strong>{{ item.event_type || `风险事件 #${item.security_event_id}` }}</strong><p>TG {{ item.recipient_tg }} · 尝试 {{ item.attempt_count }} 次</p><small v-if="item.error_message">{{ item.error_message }}</small></div><StatusBadge :label="item.status === 'sent' ? '已送达' : item.status === 'failed' ? '失败' : '等待中'" :tone="alertTone(item.status)" /></article></div>
      </article>
    </section>

    <div v-if="modalOpen" class="modal-layer"><form class="modal-card" @submit.prevent="saveRule">
      <header><div><span class="section-kicker">RISK RULE</span><h2>{{ editing ? "编辑风险规则" : "新增风险规则" }}</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header>
      <label><span>规则名称</span><input v-model.trim="form.name" required minlength="2" maxlength="100" /></label>
      <label><span>事件匹配</span><input v-model.trim="form.event_pattern" required placeholder="例如 auth.* 或 service.probe.failed" /></label>
      <div class="rule-form-grid"><label><span>风险级别</span><select v-model="form.severity"><option value="info">关注</option><option value="warning">警告</option><option value="danger">高危</option></select></label><label><span>触发次数</span><input v-model.number="form.threshold_count" type="number" min="1" /></label><label><span>统计窗口（分钟）</span><input v-model.number="form.window_minutes" type="number" min="1" /></label><label><span>冷却时间（分钟）</span><input v-model.number="form.cooldown_minutes" type="number" min="1" /></label></div>
      <div class="rule-checks"><label><input v-model="form.enabled" type="checkbox" />启用规则</label><label><input v-model="form.telegram_alert" type="checkbox" />触发后发送 Telegram 告警</label></div>
      <footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="saving"><Save :size="16" />{{ saving ? "保存中…" : "保存规则" }}</button></footer>
    </form></div>
  </div>
</template>

<style scoped>
.diagnostic-notice { display:flex; align-items:center; gap:9px; padding:12px 15px; color:var(--green); border:1px solid rgba(113,211,155,.2); border-radius:12px; background:rgba(113,211,155,.06); }
.probe-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:14px; }
.probe-card { display:grid; grid-template-columns:auto minmax(0,1fr) auto; gap:12px; padding:18px; }
.probe-card > span { display:grid; place-items:center; width:42px; height:42px; color:var(--green); border-radius:13px; background:rgba(113,211,155,.08); }
.probe-card[data-status="unhealthy"] > span { color:var(--red); background:rgba(255,116,125,.08); }
.probe-card small { color:var(--muted-2); font-size:8px; letter-spacing:.14em; }.probe-card strong{display:block;margin-top:4px}.probe-card p{overflow:hidden;margin-top:6px;color:var(--muted);font-size:10px;text-overflow:ellipsis;white-space:nowrap}.probe-card footer{grid-column:1/-1;display:flex;justify-content:space-between;padding-top:12px;color:var(--muted-2);font-size:9px;border-top:1px solid var(--border)}
.probe-empty{grid-column:1/-1;padding:35px;color:var(--muted);text-align:center}.rules-panel>.panel-heading,.section-pad{padding:19px 21px}.rule-list{display:grid}.rule-list>article{display:grid;grid-template-columns:minmax(0,1fr) auto auto;align-items:center;gap:18px;padding:14px 20px;border-top:1px solid var(--border)}.rule-main{display:flex;align-items:center;gap:12px;min-width:0;padding:0;text-align:left;border:0;background:none;cursor:pointer}.rule-main:disabled{cursor:default}.rule-main>span{display:grid;place-items:center;width:38px;height:38px;color:var(--orange);border-radius:11px;background:rgba(241,168,91,.08)}.rule-main>span[data-severity="danger"]{color:var(--red);background:rgba(255,116,125,.08)}.rule-main>span[data-severity="info"]{color:var(--cyan);background:rgba(89,213,209,.08)}.rule-main strong{display:block}.rule-main code{display:inline-block;margin-top:4px;color:var(--pink-strong);font-size:10px}.rule-main p{margin-top:4px;color:var(--muted);font-size:10px}.rule-list label,.rule-checks label{display:flex;align-items:center;gap:7px;color:var(--muted);font-size:10px;white-space:nowrap}
.diagnostic-columns{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(320px,.65fr);gap:16px}.alert-list{display:grid}.alert-list>article{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:10px;align-items:center;padding:13px 18px;border-top:1px solid var(--border)}.alert-list>article>span{color:var(--pink)}.alert-list p,.alert-list small{display:block;margin-top:4px;color:var(--muted);font-size:9px}.alert-list small{color:var(--red)}.alert-empty{padding:35px;color:var(--muted);text-align:center;border-top:1px solid var(--border)}
.rule-form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.rule-checks{display:flex;flex-wrap:wrap;gap:18px;padding:12px 0}
@media(max-width:1150px){.probe-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.diagnostic-columns{grid-template-columns:1fr}}@media(max-width:680px){.probe-grid{grid-template-columns:1fr}.rule-list>article{grid-template-columns:1fr}.rule-form-grid{grid-template-columns:1fr}}
</style>
