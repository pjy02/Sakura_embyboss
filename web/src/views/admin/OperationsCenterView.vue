<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import type { Component } from "vue";
import { BellRing, CalendarPlus, CheckCircle2, CircleDollarSign, Coins, History, PauseCircle, PlayCircle, RefreshCw, Send, Trash2, TriangleAlert, UserRoundCog, UsersRound } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api, idempotencyKey } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { AccountLifecycleEvent, BillingReconciliation, OperationTask } from "@/types";

type BatchAction = "suspend" | "restore" | "extend" | "grant_coins" | "grant_registration_days" | "notify" | "clear_account";

const actionOptions: Array<{ value: BatchAction; label: string; description: string; icon: Component; danger?: boolean }> = [
  { value: "suspend", label: "暂停账号", description: "禁用 Emby 登录与播放权限", icon: PauseCircle, danger: true },
  { value: "restore", label: "恢复账号", description: "重新启用 Emby 账号", icon: PlayCircle },
  { value: "extend", label: "延长有效期", description: "从当前到期日或今天继续延期", icon: CalendarPlus },
  { value: "grant_coins", label: "调整积分", description: "批量增加或扣减用户积分", icon: Coins },
  { value: "grant_registration_days", label: "调整注册天数", description: "批量调整注册资格天数", icon: CircleDollarSign },
  { value: "notify", label: "发送通知", description: "联动站内通知与 Telegram 偏好", icon: BellRing },
  { value: "clear_account", label: "清理 Emby 账号", description: "删除 Emby 账号并保留 Telegram 档案", icon: Trash2, danger: true },
];

const session = useSessionStore();
const history = ref<AccountLifecycleEvent[]>([]);
const reconciliation = ref<BillingReconciliation | null>(null);
const loading = ref(true);
const submitting = ref(false);
const error = ref("");
const notice = ref("");
const targetText = ref("");
const form = reactive({ action: "extend" as BatchAction, days: 30, amount: 100, reason: "批量运营调整", title: "系统通知", body: "", severity: "info", confirm: false });

function allowed(permission: string) {
  return Boolean(session.session?.permissions.some((item) => item === "*" || item === permission || item === `${permission.split(":")[0]}:*`));
}
const canUpdate = computed(() => allowed("users:update"));
const canReadBilling = computed(() => allowed("billing:read"));
const telegramTargets = computed(() =>
  Array.from(
    new Set(
      (targetText.value.match(/(?:^|[\s,;])\d+(?=$|[\s,;])/g) || [])
        .map((item) => Number(item.trim()))
        .filter((item) => item > 0),
    ),
  ),
);
const accountTargets = computed(() =>
  Array.from(
    new Set(
      targetText.value.match(
        /\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b/gi,
      ) || [],
    ),
  ),
);
const targetCount = computed(() => telegramTargets.value.length + accountTargets.value.length);
const selectedAction = computed(() => actionOptions.find((item) => item.value === form.action) || actionOptions[0]);
const succeeded = computed(() => history.value.filter((item) => item.status === "succeeded").length);
const failed = computed(() => history.value.filter((item) => item.status === "failed").length);

async function load(silent = false) {
  if (!silent) loading.value = true;
  error.value = "";
  try {
    const result = await api<{ items: AccountLifecycleEvent[] }>("/admin/operations/lifecycle?limit=100");
    history.value = result.items;
    if (canReadBilling.value) reconciliation.value = await api<BillingReconciliation>("/admin/billing/reconciliation");
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "运营数据读取失败";
  } finally {
    loading.value = false;
  }
}

function parameters() {
  if (form.action === "extend") return { days: form.days };
  if (form.action === "grant_coins" || form.action === "grant_registration_days") return { amount: form.amount, reason: form.reason };
  if (form.action === "notify") return { title: form.title, body: form.body, severity: form.severity };
  return {};
}

async function submitBatch() {
  error.value = "";
  notice.value = "";
  if (!targetCount.value) {
    error.value = "请至少填写一个有效的统一账号 ID 或 Telegram ID";
    return;
  }
  submitting.value = true;
  try {
    const task = await api<OperationTask>("/admin/operations/batches", {
      method: "POST",
      idempotencyKey: idempotencyKey(`batch-${form.action}`),
      body: JSON.stringify({ action: form.action, tg_ids: telegramTargets.value, account_ids: accountTargets.value, parameters: parameters(), confirm: form.confirm }),
    });
    notice.value = `已创建批量任务 ${task.id}，共 ${targetCount.value} 个目标。执行结果会实时写入下方生命周期记录。`;
    form.confirm = false;
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "批量任务创建失败";
  } finally {
    submitting.value = false;
  }
}

function actionLabel(value: string) {
  return actionOptions.find((item) => item.value === value)?.label || value;
}
function detailText(item: AccountLifecycleEvent) {
  if (!item.detail) return "—";
  if (typeof item.detail.error === "string") return item.detail.error;
  if (item.detail.days) return `${item.detail.days} 天`;
  if (item.detail.balance !== undefined) return `余额 ${item.detail.balance}`;
  if (item.detail.notified) return "通知已进入送达队列";
  return Object.entries(item.detail).slice(0, 2).map(([key, value]) => `${key}: ${String(value)}`).join(" · ");
}

onMounted(load);
useRealtimeEvents(["user.lifecycle.updated", "billing.order.updated", "task.updated"], () => load(true), true);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="ACCOUNT OPERATIONS" title="批量运营" description="以可确认、可追踪的后台任务批量管理账号生命周期、权益和通知，每个用户都有独立结果与审计记录。" :icon="UserRoundCog">
      <template #actions><button class="secondary-button" @click="load()"><RefreshCw :size="16" />刷新记录</button></template>
    </AdminPageHeader>

    <div v-if="error" class="error-banner"><TriangleAlert :size="17" />{{ error }}</div>
    <div v-if="notice" class="operation-notice"><CheckCircle2 :size="17" />{{ notice }}</div>

    <section class="stats-grid admin-stats">
      <MetricCard label="当前目标" :value="formatNumber(targetCount)" caption="统一账号或 Telegram ID，单次最多 500 个" :icon="UsersRound" tone="pink" featured />
      <MetricCard label="成功记录" :value="formatNumber(succeeded)" caption="最近 100 条生命周期记录" :icon="CheckCircle2" tone="green" />
      <MetricCard label="失败记录" :value="formatNumber(failed)" caption="失败用户可核对后重新提交" :icon="TriangleAlert" :tone="failed ? 'red' : 'green'" />
      <MetricCard label="交易待核对" :value="formatNumber((reconciliation?.stale_pending || 0) + (reconciliation?.credited_without_ledger || 0) + (reconciliation?.duplicate_credit_entries || 0))" caption="超时订单与账单结构检查" :icon="CircleDollarSign" :tone="reconciliation?.status === 'attention' ? 'gold' : 'cyan'" />
    </section>

    <section class="operation-layout">
      <form class="panel batch-form" @submit.prevent="submitBatch">
        <div class="panel-heading"><div><span class="section-kicker">BATCH COMMAND</span><h2>创建批量任务</h2></div><StatusBadge :label="selectedAction.danger ? '高风险操作' : '需要确认'" :tone="selectedAction.danger ? 'danger' : 'warning'" /></div>
        <label><span>选择操作</span><select v-model="form.action" @change="form.confirm = false"><option v-for="item in actionOptions" :key="item.value" :value="item.value">{{ item.label }} — {{ item.description }}</option></select></label>
        <label><span>统一账号 ID / Telegram ID 列表</span><textarea v-model="targetText" rows="7" placeholder="支持逗号、空格或换行分隔。Web 独立账号请粘贴 UUID，Telegram 用户也可填写数字 ID。" /><small>已识别 {{ accountTargets.length }} 个统一账号、{{ telegramTargets.length }} 个 Telegram ID；不存在的用户会被安全忽略。</small></label>

        <div v-if="form.action === 'extend'" class="parameter-box"><label><span>延期天数</span><input v-model.number="form.days" type="number" min="1" max="3650" required /></label></div>
        <div v-else-if="form.action === 'grant_coins' || form.action === 'grant_registration_days'" class="parameter-box two"><label><span>调整数量</span><input v-model.number="form.amount" type="number" min="-10000000" max="10000000" required /></label><label><span>调整原因</span><input v-model.trim="form.reason" maxlength="255" required /></label><p>正数为增加，负数为扣减；不会允许余额被扣到负数。</p></div>
        <div v-else-if="form.action === 'notify'" class="parameter-box"><label><span>通知标题</span><input v-model.trim="form.title" maxlength="200" required /></label><label><span>通知正文</span><textarea v-model.trim="form.body" rows="5" maxlength="2000" required /></label><label><span>级别</span><select v-model="form.severity"><option value="info">普通</option><option value="success">成功</option><option value="warning">提醒</option><option value="danger">重要</option></select></label></div>
        <div v-else class="impact-box" :class="{ danger: selectedAction.danger }"><component :is="selectedAction.icon" :size="20" /><div><strong>{{ selectedAction.label }}</strong><p>{{ selectedAction.description }}。每个用户执行失败不会中断整批任务。</p></div></div>

        <label class="confirm-row"><input v-model="form.confirm" type="checkbox" /><span>我已核对目标用户和操作参数，并确认创建后台任务</span></label>
        <button class="primary-button wide" :disabled="!canUpdate || submitting || !form.confirm"><Send :size="16" />{{ submitting ? "正在创建…" : canUpdate ? `确认执行 · ${targetCount} 个用户` : "当前角色无操作权限" }}</button>
      </form>

      <aside class="side-stack">
        <article class="panel reconciliation-card">
          <div class="panel-heading"><div><span class="section-kicker">RECONCILIATION</span><h2>交易对账</h2></div><StatusBadge v-if="reconciliation" :label="reconciliation.status === 'healthy' ? '账单一致' : '需要核对'" :tone="reconciliation.status === 'healthy' ? 'success' : 'warning'" /></div>
          <div v-if="reconciliation" class="reconciliation-grid"><div><span>超过 24 小时待处理</span><strong>{{ reconciliation.stale_pending }}</strong></div><div><span>入账但缺少流水</span><strong>{{ reconciliation.credited_without_ledger }}</strong></div><div><span>重复入账流水</span><strong>{{ reconciliation.duplicate_credit_entries }}</strong></div><div><span>已退款订单</span><strong>{{ reconciliation.status_counts.refunded || 0 }}</strong></div></div>
          <p v-else>{{ canReadBilling ? "正在读取对账结果…" : "当前角色没有账单读取权限。" }}</p>
        </article>
        <article class="panel safety-card"><span><History :size="20" /></span><div><strong>任务与审计双记录</strong><p>任务队列负责重试和执行状态；生命周期记录负责逐个用户回溯，操作日志记录发起人与具体变更。</p></div></article>
      </aside>
    </section>

    <section class="panel table-panel">
      <div class="panel-heading section-pad"><div><span class="section-kicker">LIFECYCLE HISTORY</span><h2>账号生命周期记录</h2></div><span class="page-count">最近 {{ history.length }} 条</span></div>
      <AdminDataTable :loading="loading" :empty="!history.length" empty-title="暂无批量运营记录" min-width="820px">
        <template #head><tr><th>用户</th><th>操作</th><th>结果</th><th>执行详情</th><th>执行节点</th><th>时间</th></tr></template>
        <template #body><tr v-for="item in history" :key="item.id"><td><strong>{{ item.account_id ? `账号 · ${item.account_id.slice(0, 8)}` : `TG · ${item.tg}` }}</strong></td><td>{{ actionLabel(item.action) }}</td><td><StatusBadge :label="item.status === 'succeeded' ? '成功' : '失败'" :tone="item.status === 'succeeded' ? 'success' : 'danger'" /></td><td class="detail-cell">{{ detailText(item) }}</td><td>{{ item.actor_id }}</td><td>{{ formatDate(item.created_at) }}</td></tr></template>
      </AdminDataTable>
    </section>
  </div>
</template>

<style scoped>
.operation-notice{display:flex;align-items:center;gap:9px;padding:12px 15px;color:var(--green);border:1px solid rgba(113,211,155,.2);border-radius:12px;background:rgba(113,211,155,.06)}
.operation-layout{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(320px,.65fr);gap:17px}.batch-form{display:grid;gap:17px;padding:23px}.batch-form>label,.parameter-box label{display:grid;gap:7px}.batch-form label>span,.parameter-box label>span{color:var(--muted);font-size:10px}.batch-form input,.batch-form textarea,.batch-form select{width:100%;padding:11px 12px;color:var(--text);border:1px solid var(--border);border-radius:10px;background:rgba(255,255,255,.035)}.batch-form textarea{resize:vertical}.batch-form label small{color:var(--muted-2);font-size:9px}
.parameter-box{display:grid;gap:12px;padding:15px;border:1px solid var(--border);border-radius:13px;background:rgba(255,255,255,.018)}.parameter-box.two{grid-template-columns:repeat(2,minmax(0,1fr))}.parameter-box.two p{grid-column:1/-1;color:var(--muted-2);font-size:9px}.impact-box{display:flex;align-items:flex-start;gap:12px;padding:15px;color:var(--cyan);border:1px solid rgba(89,213,209,.14);border-radius:13px;background:rgba(89,213,209,.05)}.impact-box.danger{color:var(--red);border-color:rgba(255,116,125,.18);background:rgba(255,116,125,.05)}.impact-box p{margin-top:5px;color:var(--muted);font-size:10px;line-height:1.6}.confirm-row{display:flex!important;grid-template-columns:auto 1fr!important;align-items:center;gap:9px!important;padding:13px;border:1px solid var(--border);border-radius:11px;background:rgba(255,255,255,.02)}.confirm-row input{width:auto}.confirm-row span{color:var(--text)!important}
.side-stack{display:grid;align-content:start;gap:15px}.reconciliation-card>.panel-heading{padding:19px}.reconciliation-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1px;background:var(--border);border-top:1px solid var(--border)}.reconciliation-grid div{padding:18px;background:var(--surface)}.reconciliation-grid span{color:var(--muted);font-size:9px}.reconciliation-grid strong{display:block;margin-top:7px;font-size:23px}.reconciliation-card>p{padding:25px;color:var(--muted);border-top:1px solid var(--border)}.safety-card{display:flex;gap:13px;padding:19px}.safety-card>span{color:var(--violet)}.safety-card p{margin-top:6px;color:var(--muted);font-size:10px;line-height:1.65}.section-pad{padding:19px 21px}.detail-cell{max-width:340px;overflow:hidden;color:var(--muted);text-overflow:ellipsis;white-space:nowrap}
@media(max-width:1000px){.operation-layout{grid-template-columns:1fr}}@media(max-width:650px){.parameter-box.two,.reconciliation-grid{grid-template-columns:1fr}.admin-stats{grid-template-columns:1fr}}
</style>
