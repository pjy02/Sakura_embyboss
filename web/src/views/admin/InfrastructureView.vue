<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Check, CloudCog, Copy, KeyRound, Plus, Server, ShieldKeyhole, Trash2 } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { ApiClient, EmbyInstance, ManagedCredential } from "@/types";

const session = useSessionStore();
const tab = ref<"emby" | "credentials" | "api">("emby");
const credentials = ref<ManagedCredential[]>([]);
const instances = ref<EmbyInstance[]>([]);
const clients = ref<ApiClient[]>([]);
const scopes = ref<string[]>([]);
const loading = ref(true);
const modal = ref<"credential" | "emby" | "api" | null>(null);
const editingId = ref<string | null>(null);
const generatedKey = ref("");
const copied = ref(false);
const credentialForm = reactive({ name: "", provider: "tmdb", credential_type: "api_token" as ManagedCredential["credential_type"], secret: "", active: true, revision: undefined as number | undefined });
const embyForm = reactive({ name: "", base_url: "", credential_id: "", enabled: true, is_default: false, verify_tls: true, priority: 100, revision: undefined as number | undefined });
const apiForm = reactive({ name: "", scopes: [] as string[], expires_at: "" });
const canManageIntegrations = computed(() => session.session?.permissions.some((item) => item === "*" || item === "integrations:*" || item === "integrations:manage"));
const canManageApi = computed(() => session.session?.permissions.some((item) => item === "*" || item === "api:*" || item === "api:manage"));

async function load() {
  loading.value = true;
  try {
    const [credentialResult, instanceResult, clientResult] = await Promise.all([
      api<{ items: ManagedCredential[] }>("/admin/credentials"),
      api<{ items: EmbyInstance[] }>("/admin/emby-instances"),
      api<{ items: ApiClient[]; available_scopes: string[] }>("/admin/api-clients"),
    ]);
    credentials.value = credentialResult.items;
    instances.value = instanceResult.items;
    clients.value = clientResult.items;
    scopes.value = clientResult.available_scopes;
  } finally { loading.value = false; }
}

function openCredential(item?: ManagedCredential) {
  editingId.value = item?.id ?? null;
  Object.assign(credentialForm, item ? { name: item.name, provider: item.provider, credential_type: item.credential_type, secret: "", active: item.active, revision: item.revision } : { name: "", provider: "tmdb", credential_type: "api_token", secret: "", active: true, revision: undefined });
  modal.value = "credential";
}

async function saveCredential() {
  await api(editingId.value ? `/admin/credentials/${editingId.value}` : "/admin/credentials", { method: editingId.value ? "PATCH" : "POST", body: JSON.stringify({ ...credentialForm, secret: credentialForm.secret || null, metadata: {} }) });
  modal.value = null;
  await load();
}

async function deleteCredential(item: ManagedCredential) {
  if (!window.confirm(`删除凭据“${item.name}”？`)) return;
  await api(`/admin/credentials/${item.id}`, { method: "DELETE" });
  await load();
}

function openEmby(item?: EmbyInstance) {
  editingId.value = item?.id ?? null;
  Object.assign(embyForm, item ? { name: item.name, base_url: item.base_url, credential_id: item.credential_id, enabled: item.enabled, is_default: item.is_default, verify_tls: item.verify_tls, priority: item.priority, revision: item.revision } : { name: "", base_url: "", credential_id: credentials.value.find((value) => value.provider === "emby" && value.active)?.id || "", enabled: true, is_default: !instances.value.length, verify_tls: true, priority: 100, revision: undefined });
  modal.value = "emby";
}

async function saveEmby() {
  await api(editingId.value ? `/admin/emby-instances/${editingId.value}` : "/admin/emby-instances", { method: editingId.value ? "PATCH" : "POST", body: JSON.stringify(embyForm) });
  modal.value = null;
  await load();
}

async function probe(item: EmbyInstance) {
  await api(`/admin/emby-instances/${item.id}/probe`, { method: "POST" });
  await load();
}

async function adoptLegacy(item: EmbyInstance) {
  if (!window.confirm(`把 config.json 单站点中的现有账号映射到“${item.name}”？该操作不会在 Emby 中重复创建账号。`)) return;
  const result = await api<{ created: number; skipped: number }>(`/admin/emby-instances/${item.id}/adopt-legacy`, { method: "POST" });
  window.alert(`迁移完成：新建 ${result.created} 条绑定，跳过 ${result.skipped} 条。`);
  await load();
}

async function deleteEmby(item: EmbyInstance) {
  if (!window.confirm(`删除 Emby 实例“${item.name}”？`)) return;
  await api(`/admin/emby-instances/${item.id}`, { method: "DELETE" });
  await load();
}

function openApi() { Object.assign(apiForm, { name: "", scopes: ["health:read"], expires_at: "" }); generatedKey.value = ""; modal.value = "api"; }
async function createApi() {
  const result = await api<ApiClient & { api_key: string }>("/admin/api-clients", { method: "POST", body: JSON.stringify({ name: apiForm.name, scopes: apiForm.scopes, expires_at: apiForm.expires_at || null }) });
  generatedKey.value = result.api_key;
  await load();
}
async function revokeApi(item: ApiClient) { if (!window.confirm(`吊销“${item.name}”的 API Key？`)) return; await api(`/admin/api-clients/${item.id}/revoke`, { method: "POST" }); await load(); }
async function copyKey() { await navigator.clipboard.writeText(generatedKey.value); copied.value = true; window.setTimeout(() => copied.value = false, 1500); }
onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="INFRASTRUCTURE" title="集成、凭据与开放 API" description="集中管理多 Emby、TMDB、MoviePilot 等凭据和第三方 API Key；私密内容加密保存且不会再次显示。" :icon="CloudCog"><template #actions><button v-if="tab === 'emby' && canManageIntegrations" class="primary-button" @click="openEmby()"><Plus :size="16" />添加 Emby</button><button v-else-if="tab === 'credentials' && canManageIntegrations" class="primary-button" @click="openCredential()"><Plus :size="16" />添加凭据</button><button v-else-if="tab === 'api' && canManageApi" class="primary-button" @click="openApi"><Plus :size="16" />创建 API Key</button></template></AdminPageHeader>
    <section class="platform-summary"><article><Server :size="22" /><div><strong>{{ instances.length }}</strong><span>Emby 实例</span></div></article><article><ShieldKeyhole :size="22" /><div><strong>{{ credentials.filter(item => item.active).length }}</strong><span>有效凭据</span></div></article><article><KeyRound :size="22" /><div><strong>{{ clients.filter(item => item.active).length }}</strong><span>开放 API 客户端</span></div></article></section>
    <div class="segmented-tabs"><button :class="{ active: tab === 'emby' }" @click="tab = 'emby'">多 Emby</button><button :class="{ active: tab === 'credentials' }" @click="tab = 'credentials'">凭据中心</button><button :class="{ active: tab === 'api' }" @click="tab = 'api'">开放 API</button></div>
    <section v-if="tab === 'emby'" class="panel table-panel"><AdminDataTable :loading="loading" :empty="!instances.length" empty-title="尚未配置 Emby 实例" empty-description="未配置时注册仍会回退使用 config.json 中的单一 Emby。" min-width="900px"><template #head><tr><th>实例</th><th>地址</th><th>账号绑定</th><th>状态</th><th>最近探测</th><th>操作</th></tr></template><template #body><tr v-for="item in instances" :key="item.id"><td><div class="table-primary"><strong>{{ item.name }}</strong><small>{{ item.is_default ? '默认注册实例' : `优先级 ${item.priority}` }}</small></div></td><td><code>{{ item.base_url }}</code></td><td>{{ formatNumber(item.binding_count) }}</td><td><StatusBadge :label="!item.enabled ? '已停用' : item.status === 'healthy' ? '健康' : item.status === 'unhealthy' ? '异常' : '未探测'" :tone="!item.enabled ? 'muted' : item.status === 'healthy' ? 'success' : item.status === 'unhealthy' ? 'danger' : 'info'" /></td><td>{{ formatDate(item.last_checked_at) }}<small v-if="item.last_latency_ms" class="cell-sub">{{ item.last_latency_ms }} ms</small></td><td><div class="row-actions"><button class="text-button" @click="probe(item)">探测</button><button v-if="canManageIntegrations && !item.binding_count" class="text-button" @click="adoptLegacy(item)">迁移旧账号</button><button v-if="canManageIntegrations" class="text-button" @click="openEmby(item)">编辑</button><button v-if="canManageIntegrations && !item.binding_count" class="text-button danger-text" @click="deleteEmby(item)">删除</button></div></td></tr></template></AdminDataTable></section>
    <section v-else-if="tab === 'credentials'" class="panel table-panel"><div class="security-note"><ShieldKeyhole :size="20" /><div><strong>密钥只写不读</strong><p>保存后仅显示 SHA-256 指纹。轮换时填写新值，系统不会向浏览器返回历史明文。</p></div></div><AdminDataTable :loading="loading" :empty="!credentials.length" empty-title="尚未保存凭据" empty-description="建议先添加 tmdb、moviepilot 和 emby 三类凭据。" min-width="820px"><template #head><tr><th>凭据</th><th>提供方</th><th>类型</th><th>指纹</th><th>最后使用</th><th>状态</th><th>操作</th></tr></template><template #body><tr v-for="item in credentials" :key="item.id"><td>{{ item.name }}</td><td><code>{{ item.provider }}</code></td><td>{{ item.credential_type }}</td><td><code>{{ item.fingerprint }}</code></td><td>{{ formatDate(item.last_used_at) }}</td><td><StatusBadge :label="item.active ? '有效' : '停用'" :tone="item.active ? 'success' : 'muted'" /></td><td><div class="row-actions"><button v-if="canManageIntegrations" class="text-button" @click="openCredential(item)">轮换 / 编辑</button><button v-if="canManageIntegrations" class="text-button danger-text" @click="deleteCredential(item)"><Trash2 :size="14" />删除</button></div></td></tr></template></AdminDataTable></section>
    <section v-else class="panel table-panel"><div class="security-note"><KeyRound :size="20" /><div><strong>Bearer API Key</strong><p>开放接口位于 <code>/api/open/v1</code>。还需在系统设置中开启“开放 API”。</p></div></div><AdminDataTable :loading="loading" :empty="!clients.length" empty-title="尚未创建 API 客户端" empty-description="按最小权限创建，每把 Key 只在生成时显示一次。" min-width="900px"><template #head><tr><th>客户端</th><th>Key 前缀</th><th>权限范围</th><th>最后使用</th><th>来源 IP</th><th>状态</th><th>操作</th></tr></template><template #body><tr v-for="item in clients" :key="item.id"><td>{{ item.name }}</td><td><code>{{ item.key_prefix }}…</code></td><td><div class="scope-list"><span v-for="scope in item.scopes" :key="scope">{{ scope }}</span></div></td><td>{{ formatDate(item.last_used_at) }}</td><td>{{ item.last_ip || "—" }}</td><td><StatusBadge :label="item.active ? '有效' : '已吊销'" :tone="item.active ? 'success' : 'muted'" /></td><td><button v-if="canManageApi && item.active" class="text-button danger-text" @click="revokeApi(item)">吊销</button></td></tr></template></AdminDataTable></section>

    <div v-if="modal === 'credential'" class="modal-layer"><form class="modal-card" @submit.prevent="saveCredential"><header><div><span class="section-kicker">SECRET VAULT</span><h2>{{ editingId ? "轮换 / 编辑凭据" : "添加加密凭据" }}</h2></div><button type="button" class="icon-button" @click="modal = null">×</button></header><div class="form-grid"><label><span>名称</span><input v-model.trim="credentialForm.name" required /></label><label><span>提供方标识</span><input v-model.trim="credentialForm.provider" required placeholder="tmdb / moviepilot / emby" /></label></div><label><span>{{ editingId ? '新密钥（留空则不轮换）' : '密钥内容' }}</span><textarea v-model="credentialForm.secret" :required="!editingId" autocomplete="new-password" /></label><div class="form-grid"><label><span>类型</span><select v-model="credentialForm.credential_type"><option value="api_token">API Token</option><option value="api_key">API Key</option><option value="bearer">Bearer Token</option><option value="password">密码</option></select></label><label class="check-row"><input v-model="credentialForm.active" type="checkbox" /><span>立即启用</span></label></div><footer><button type="button" class="secondary-button" @click="modal = null">取消</button><button class="primary-button">加密保存</button></footer></form></div>
    <div v-if="modal === 'emby'" class="modal-layer"><form class="modal-card" @submit.prevent="saveEmby"><header><div><span class="section-kicker">EMBY INSTANCE</span><h2>{{ editingId ? "编辑 Emby 实例" : "添加 Emby 实例" }}</h2></div><button type="button" class="icon-button" @click="modal = null">×</button></header><label><span>实例名称</span><input v-model.trim="embyForm.name" required /></label><label><span>基础地址</span><input v-model.trim="embyForm.base_url" required placeholder="https://emby.example.com" /></label><label><span>Emby API 凭据</span><select v-model="embyForm.credential_id" required><option value="" disabled>请选择 Emby 凭据</option><option v-for="item in credentials.filter(value => value.active && value.provider === 'emby')" :key="item.id" :value="item.id">{{ item.name }} · {{ item.fingerprint }}</option></select></label><div class="form-grid"><label><span>优先级</span><input v-model.number="embyForm.priority" type="number" min="0" /></label><div class="check-stack"><label class="check-row"><input v-model="embyForm.enabled" type="checkbox" /><span>启用实例</span></label><label class="check-row"><input v-model="embyForm.is_default" type="checkbox" /><span>作为默认注册实例</span></label><label class="check-row"><input v-model="embyForm.verify_tls" type="checkbox" /><span>验证 TLS 证书</span></label></div></div><footer><button type="button" class="secondary-button" @click="modal = null">取消</button><button class="primary-button">保存实例</button></footer></form></div>
    <div v-if="modal === 'api'" class="modal-layer"><form class="modal-card" @submit.prevent="generatedKey ? undefined : createApi()"><header><div><span class="section-kicker">OPEN API</span><h2>{{ generatedKey ? "API Key 已生成" : "创建 API 客户端" }}</h2></div><button type="button" class="icon-button" @click="modal = null">×</button></header><template v-if="!generatedKey"><label><span>客户端名称</span><input v-model.trim="apiForm.name" required /></label><fieldset><legend>最小权限范围</legend><label v-for="scope in scopes" :key="scope" class="check-row"><input v-model="apiForm.scopes" type="checkbox" :value="scope" /><span>{{ scope }}</span></label></fieldset><label><span>过期时间（可选）</span><input v-model="apiForm.expires_at" type="datetime-local" /></label></template><template v-else><div class="generated-key"><ShieldKeyhole :size="28" /><strong>请立即复制并安全保存</strong><p>关闭后将无法再次查看，只能吊销并重新创建。</p><code>{{ generatedKey }}</code><button type="button" class="secondary-button" @click="copyKey"><Check v-if="copied" :size="16" /><Copy v-else :size="16" />{{ copied ? '已复制' : '复制 Key' }}</button></div></template><footer><button type="button" class="secondary-button" @click="modal = null">{{ generatedKey ? '我已保存' : '取消' }}</button><button v-if="!generatedKey" class="primary-button">生成一次性 Key</button></footer></form></div>
  </div>
</template>

<style scoped>
.platform-summary{display:grid;grid-template-columns:repeat(3,1fr);gap:14px}.platform-summary article{display:flex;align-items:center;gap:14px;padding:18px;border:1px solid var(--border);border-radius:16px;background:var(--surface)}.platform-summary article>svg{color:var(--primary)}.platform-summary strong,.platform-summary span{display:block}.platform-summary strong{font-size:24px}.platform-summary span{font-size:12px;color:var(--text-muted)}.segmented-tabs{display:flex;gap:6px;padding:5px;background:var(--surface-subtle);border:1px solid var(--border);border-radius:14px;width:max-content}.segmented-tabs button{border:0;background:transparent;padding:9px 16px;border-radius:10px;color:var(--text-muted);font-weight:700}.segmented-tabs button.active{background:var(--surface);color:var(--text);box-shadow:var(--shadow-sm)}.security-note{display:flex;gap:12px;align-items:flex-start;padding:18px;border-bottom:1px solid var(--border)}.security-note svg{color:var(--primary)}.security-note p{margin:4px 0 0;color:var(--text-muted)}.row-actions,.scope-list{display:flex;flex-wrap:wrap;gap:8px}.scope-list span{padding:4px 7px;border-radius:7px;background:var(--surface-subtle);font:11px ui-monospace,monospace}.danger-text{color:var(--danger)}.check-stack{display:grid;gap:8px}.generated-key{text-align:center;padding:24px;border:1px dashed var(--primary);border-radius:16px;background:color-mix(in srgb,var(--primary) 7%,transparent)}.generated-key>svg{color:var(--primary)}.generated-key strong,.generated-key code{display:block;margin:10px 0}.generated-key code{padding:12px;border-radius:10px;background:var(--surface);overflow-wrap:anywhere}fieldset{display:grid;grid-template-columns:1fr 1fr;gap:8px;border:1px solid var(--border);border-radius:12px;padding:14px}legend{padding:0 6px;font-weight:700}@media(max-width:760px){.platform-summary{grid-template-columns:1fr}.segmented-tabs{width:100%;overflow:auto}fieldset{grid-template-columns:1fr}}
</style>
