<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Ban, CheckCircle2, HardDrive, Plus, Search, ShieldCheck, ShieldX, TestTube2 } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { DeviceClientRule, KnownDevice } from "@/types";

const session = useSessionStore();
const tab = ref<"devices" | "rules">("devices");
const devices = ref<KnownDevice[]>([]);
const rules = ref<DeviceClientRule[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const editorOpen = ref(false);
const editingId = ref<number | null>(null);
const testClient = ref("");
const testResult = ref<{ action: string; matched: DeviceClientRule | null } | null>(null);
const form = reactive({ name: "", pattern: "", match_type: "contains" as DeviceClientRule["match_type"], action: "allow" as DeviceClientRule["action"], enabled: true, priority: 100, notes: "", revision: undefined as number | undefined });

const canUpdate = computed(() => session.session?.permissions.some((item) => item === "*" || item === "devices:*" || item === "devices:update"));
const allowCount = computed(() => rules.value.filter((item) => item.enabled && item.action === "allow").length);
const blockCount = computed(() => rules.value.filter((item) => item.enabled && item.action === "block").length);

async function load() {
  loading.value = true;
  try {
    const params = new URLSearchParams({ limit: "100", offset: "0" });
    if (search.value) params.set("search", search.value);
    const [deviceResult, ruleResult] = await Promise.all([
      api<{ items: KnownDevice[]; total: number }>(`/admin/devices?${params}`),
      api<{ items: DeviceClientRule[] }>("/admin/device-rules"),
    ]);
    devices.value = deviceResult.items;
    total.value = deviceResult.total;
    rules.value = ruleResult.items;
  } finally {
    loading.value = false;
  }
}

async function updateDevice(item: KnownDevice, action: "trust" | "ban" | "unban") {
  if (!canUpdate.value) return;
  const body = action === "trust" ? { trusted: true, banned: false } : action === "ban" ? { banned: true, trusted: false } : { banned: false };
  await api(`/admin/devices/${encodeURIComponent(item.device_key)}`, { method: "PATCH", body: JSON.stringify(body) });
  await load();
}

function openRule(item?: DeviceClientRule) {
  editingId.value = item?.id ?? null;
  Object.assign(form, item ? { name: item.name, pattern: item.pattern, match_type: item.match_type, action: item.action, enabled: item.enabled, priority: item.priority, notes: item.notes || "", revision: item.revision } : { name: "", pattern: "", match_type: "contains", action: "allow", enabled: true, priority: 100, notes: "", revision: undefined });
  editorOpen.value = true;
}

async function saveRule() {
  const path = editingId.value ? `/admin/device-rules/${editingId.value}` : "/admin/device-rules";
  await api(path, { method: editingId.value ? "PATCH" : "POST", body: JSON.stringify(form) });
  editorOpen.value = false;
  await load();
}

async function removeRule(item: DeviceClientRule) {
  if (item.built_in || !window.confirm(`删除规则“${item.name}”？`)) return;
  await api(`/admin/device-rules/${item.id}`, { method: "DELETE" });
  await load();
}

async function evaluate() {
  if (!testClient.value) return;
  testResult.value = await api("/admin/device-rules/evaluate", { method: "POST", body: JSON.stringify({ client_name: testClient.value }) });
}

function deviceTone(item: KnownDevice): "danger" | "warning" | "success" | "muted" {
  if (item.banned || item.risk_level === "high") return "danger";
  if (item.risk_level === "warning") return "warning";
  if (item.trusted) return "success";
  return "muted";
}

onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="DEVICE CONTROL" title="设备与客户端规则" description="统一管理设备画像、客户端黑白名单和登录拦截策略；规则由 Bot 与 Webhook 实时共用。" :icon="HardDrive">
      <template #actions><button v-if="tab === 'rules' && canUpdate" class="primary-button" @click="openRule()"><Plus :size="17" /> 新建规则</button></template>
    </AdminPageHeader>

    <div class="metric-grid compact-grid">
      <MetricCard label="已知设备" :value="formatNumber(total)" caption="播放同步建立的设备画像" :icon="HardDrive" />
      <MetricCard label="客户端白名单" :value="formatNumber(allowCount)" caption="命中后明确放行" :icon="ShieldCheck" tone="green" />
      <MetricCard label="客户端黑名单" :value="formatNumber(blockCount)" caption="命中后拒绝并进入风控" :icon="ShieldX" tone="red" />
    </div>

    <div class="segmented-tabs"><button :class="{ active: tab === 'devices' }" @click="tab = 'devices'">设备画像</button><button :class="{ active: tab === 'rules' }" @click="tab = 'rules'">客户端规则</button></div>

    <section v-if="tab === 'devices'" class="panel table-panel">
      <div class="toolbar"><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索设备、用户、客户端或 IP" @keyup.enter="load" /></label><button class="secondary-button" @click="load">查询</button></div>
      <AdminDataTable :loading="loading" :empty="!devices.length" empty-title="暂无设备数据" empty-description="同步播放会话后会自动建立设备画像。" min-width="960px">
        <template #head><tr><th>设备 / 客户端</th><th>账号</th><th>最近 IP</th><th>播放</th><th>最近活跃</th><th>状态</th><th>操作</th></tr></template>
        <template #body><tr v-for="item in devices" :key="item.device_key"><td><div class="table-primary"><strong>{{ item.device_name || "未知设备" }}</strong><small>{{ item.client_name || "未知客户端" }} {{ item.app_version || "" }}</small></div></td><td>{{ item.emby_user_name || "—" }}<small class="cell-sub">{{ item.tg ? `TG · ${item.tg}` : item.emby_user_id || "" }}</small></td><td>{{ item.last_ip || "—" }}</td><td>{{ formatNumber(item.playback_count) }}</td><td>{{ formatDate(item.last_seen_at) }}</td><td><StatusBadge :label="item.banned ? '已封禁' : item.trusted ? '已信任' : item.risk_level === 'normal' ? '待验证' : '需关注'" :tone="deviceTone(item)" /></td><td><div class="row-actions"><button v-if="canUpdate && !item.trusted" class="text-button" @click="updateDevice(item, 'trust')"><CheckCircle2 :size="15" />信任</button><button v-if="canUpdate && !item.banned" class="text-button danger-text" @click="updateDevice(item, 'ban')"><Ban :size="15" />封禁</button><button v-if="canUpdate && item.banned" class="text-button" @click="updateDevice(item, 'unban')">解封</button></div></td></tr></template>
      </AdminDataTable>
    </section>

    <template v-else>
      <section class="panel rule-tester"><div><span class="section-kicker">RULE TESTER</span><h3>测试客户端名称</h3><p>输入 Emby 会话中的 Client 字段，预览最终动作，不产生拦截。</p></div><div class="test-row"><input v-model.trim="testClient" placeholder="例如 Infuse-Direct" @keyup.enter="evaluate" /><button class="secondary-button" @click="evaluate"><TestTube2 :size="17" />测试</button></div><StatusBadge v-if="testResult" :label="testResult.matched ? `${testResult.action} · ${testResult.matched.name}` : '未命中 · 观察'" :tone="testResult.action === 'block' ? 'danger' : testResult.action === 'allow' ? 'success' : 'muted'" /></section>
      <section class="panel table-panel"><AdminDataTable :loading="loading" :empty="!rules.length" empty-title="暂无客户端规则" empty-description="新建白名单、黑名单或仅观察规则。" min-width="920px"><template #head><tr><th>规则</th><th>匹配</th><th>动作</th><th>优先级</th><th>命中</th><th>状态</th><th>操作</th></tr></template><template #body><tr v-for="item in rules" :key="item.id"><td><div class="table-primary"><strong>{{ item.name }}</strong><small>{{ item.built_in ? "内置规则" : item.notes || "自定义规则" }}</small></div></td><td><code>{{ item.pattern }}</code><small class="cell-sub">{{ item.match_type }}</small></td><td><StatusBadge :label="item.action === 'allow' ? '白名单' : item.action === 'block' ? '黑名单' : '仅观察'" :tone="item.action === 'allow' ? 'success' : item.action === 'block' ? 'danger' : 'muted'" /></td><td>{{ item.priority }}</td><td>{{ formatNumber(item.hit_count) }}</td><td>{{ item.enabled ? "启用" : "停用" }}</td><td><div class="row-actions"><button v-if="canUpdate" class="text-button" @click="openRule(item)">编辑</button><button v-if="canUpdate && !item.built_in" class="text-button danger-text" @click="removeRule(item)">删除</button></div></td></tr></template></AdminDataTable></section>
    </template>

    <div v-if="editorOpen" class="modal-layer"><form class="modal-card" @submit.prevent="saveRule"><header><div><span class="section-kicker">CLIENT RULE</span><h2>{{ editingId ? "编辑客户端规则" : "新建客户端规则" }}</h2></div><button type="button" class="icon-button" @click="editorOpen = false">×</button></header><label><span>规则名称</span><input v-model.trim="form.name" required maxlength="120" /></label><label><span>匹配内容</span><input v-model.trim="form.pattern" required maxlength="255" /></label><div class="form-grid"><label><span>匹配方式</span><select v-model="form.match_type"><option value="contains">包含</option><option value="exact">完全相等</option><option value="glob">通配符</option><option value="regex">正则</option></select></label><label><span>命中动作</span><select v-model="form.action"><option value="allow">白名单放行</option><option value="block">黑名单拦截</option><option value="observe">仅观察</option></select></label></div><div class="form-grid"><label><span>优先级（越小越先）</span><input v-model.number="form.priority" type="number" min="0" max="100000" /></label><label class="check-row"><input v-model="form.enabled" type="checkbox" /><span>立即启用</span></label></div><label><span>备注</span><textarea v-model.trim="form.notes" maxlength="500" /></label><footer><button type="button" class="secondary-button" @click="editorOpen = false">取消</button><button class="primary-button">保存规则</button></footer></form></div>
  </div>
</template>

<style scoped>
.segmented-tabs{display:flex;gap:6px;padding:5px;background:var(--surface-subtle);border:1px solid var(--border);border-radius:14px;width:max-content}.segmented-tabs button{border:0;background:transparent;padding:9px 16px;border-radius:10px;color:var(--text-muted);font-weight:700}.segmented-tabs button.active{background:var(--surface);color:var(--text);box-shadow:var(--shadow-sm)}.toolbar,.test-row,.row-actions{display:flex;align-items:center;gap:10px}.toolbar{padding:18px}.toolbar .search-box{flex:1}.rule-tester{display:grid;grid-template-columns:1fr minmax(280px,520px) auto;align-items:center;gap:20px;padding:22px}.rule-tester h3{margin:4px 0}.rule-tester p{margin:0;color:var(--text-muted)}.test-row input{width:100%}.danger-text{color:var(--danger)}code{font-size:12px;color:var(--primary)}@media(max-width:800px){.rule-tester{grid-template-columns:1fr}.test-row{width:100%}.metric-grid{grid-template-columns:1fr}}
</style>
