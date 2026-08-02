<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { BadgeCheck, Plus, Search, Tags, Trash2, UsersRound } from "lucide-vue-next";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";

interface Tag { id: number; name: string; color: string; description: string | null }
interface Plan { id: number; code: string; name: string; duration_days: number; legacy_level: string; enabled: boolean; is_default: boolean; revision: number; entitlements: Record<string, unknown> }
interface Account { account_id: string; tg: number | null; display_name: string | null; status: string; identities: Array<{ provider: string; username: string | null }>; membership: { expires_at: string | null; plan: Plan } | null; tags: Tag[]; wallets: { coins: number; registration_days: number }; emby: { username: string | null } | null; created_at: string }

const accounts = ref<Account[]>([]);
const plans = ref<Plan[]>([]);
const tags = ref<Tag[]>([]);
const total = ref(0);
const search = ref("");
const loading = ref(true);
const selectedIds = ref<string[]>([]);
const selectedPlan = reactive({ account_id: "", plan_id: 0, duration_days: 30 });
const planForm = reactive({ code: "", name: "", duration_days: 30, legacy_level: "b", enabled: true, is_default: false, entitlements: {} });
const tagForm = reactive({ name: "", color: "#8b7cf6", description: "" });
const batchTag = reactive({ tag_ids: [] as number[], mode: "add" });
const notice = ref("");
const error = ref("");
let timer: number | undefined;

const allSelected = computed(() => accounts.value.length > 0 && selectedIds.value.length === accounts.value.length);

async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" });
  if (search.value) params.set("search", search.value);
  try {
    const [accountResult, planResult, tagResult] = await Promise.all([
      api<{ items: Account[]; total: number }>(`/admin/accounts?${params}`),
      api<{ items: Plan[] }>("/admin/membership-plans"),
      api<{ items: Tag[] }>("/admin/account-tags"),
    ]);
    accounts.value = accountResult.items;
    total.value = accountResult.total;
    plans.value = planResult.items;
    tags.value = tagResult.items;
    selectedIds.value = selectedIds.value.filter((id) => accounts.value.some((item) => item.account_id === id));
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "账号数据加载失败";
  } finally {
    loading.value = false;
  }
}

function toggleAll() { selectedIds.value = allSelected.value ? [] : accounts.value.map((item) => item.account_id); }
function toggle(id: string) { selectedIds.value = selectedIds.value.includes(id) ? selectedIds.value.filter((item) => item !== id) : [...selectedIds.value, id]; }

async function createPlan() {
  error.value = "";
  try {
    await api("/admin/membership-plans", { method: "POST", body: JSON.stringify(planForm) });
    Object.assign(planForm, { code: "", name: "", duration_days: 30, legacy_level: "b", enabled: true, is_default: false, entitlements: {} });
    notice.value = "会员方案已创建";
    await load();
  } catch (reason) { error.value = reason instanceof Error ? reason.message : "创建失败"; }
}

async function createTag() {
  error.value = "";
  try {
    await api("/admin/account-tags", { method: "POST", body: JSON.stringify(tagForm) });
    Object.assign(tagForm, { name: "", color: "#8b7cf6", description: "" });
    notice.value = "用户标签已创建";
    await load();
  } catch (reason) { error.value = reason instanceof Error ? reason.message : "创建失败"; }
}

async function updatePlan(plan: Plan, data: Record<string, unknown>) {
  error.value = "";
  try {
    await api(`/admin/membership-plans/${plan.id}`, {
      method: "PATCH",
      body: JSON.stringify({ revision: plan.revision, ...data }),
    });
    notice.value = "会员方案已更新";
    await load();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "方案更新失败";
  }
}

async function deleteTag(tag: Tag) {
  if (!window.confirm(`确认删除标签“${tag.name}”？账号上的该标签也会一并移除。`)) return;
  try {
    await api(`/admin/account-tags/${tag.id}`, { method: "DELETE" });
    notice.value = "标签已删除";
    await load();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "标签删除失败";
  }
}

async function assignMembership() {
  if (!selectedPlan.account_id || !selectedPlan.plan_id) return;
  await api(`/admin/accounts/${selectedPlan.account_id}/membership`, { method: "POST", body: JSON.stringify({ plan_id: selectedPlan.plan_id, duration_days: selectedPlan.duration_days }) });
  notice.value = "会员方案已分配并同步到旧 Bot 等级";
  selectedPlan.account_id = "";
  await load();
}

async function applyTags() {
  if (!selectedIds.value.length) return;
  await api("/admin/accounts/tags/batch", { method: "POST", body: JSON.stringify({ account_ids: selectedIds.value, tag_ids: batchTag.tag_ids, mode: batchTag.mode }) });
  notice.value = `已更新 ${selectedIds.value.length} 个账号的标签`;
  await load();
}

watch(search, () => { window.clearTimeout(timer); timer = window.setTimeout(load, 350); });
load();
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="ACCOUNT & ENTITLEMENTS" title="会员与标签" description="统一管理 Web、Telegram 与 Emby 身份，会员方案、权益和运营标签只保留一份。" :icon="UsersRound">
      <template #meta><span class="date-chip"><BadgeCheck :size="16" /> {{ formatNumber(total) }} 个统一账号</span></template>
    </AdminPageHeader>
    <div v-if="notice" class="success-banner">{{ notice }}</div><div v-if="error" class="error-banner">{{ error }}</div>

    <section class="panel table-panel">
      <FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索显示名、Web 登录名或 Telegram ID" /></label></FilterBar>
      <AdminDataTable :loading="loading" :empty="!accounts.length" empty-title="暂无统一账号" min-width="1050px">
        <template #head><tr><th><input type="checkbox" :checked="allSelected" @change="toggleAll" /></th><th>账号</th><th>登录身份</th><th>会员方案</th><th>标签</th><th>余额</th><th>创建时间</th><th /></tr></template>
        <template #body><tr v-for="item in accounts" :key="item.account_id">
          <td><input type="checkbox" :checked="selectedIds.includes(item.account_id)" @change="toggle(item.account_id)" /></td>
          <td><div class="table-primary"><strong>{{ item.display_name || item.emby?.username || "未命名账号" }}</strong><small>{{ item.account_id.slice(0, 8) }} · {{ item.tg ? `TG ${item.tg}` : "Web 独立账号" }}</small></div></td>
          <td>{{ item.identities.map((identity) => identity.provider === "local" ? `Web:${identity.username}` : identity.provider).join(" · ") }}</td>
          <td><StatusBadge :label="item.membership?.plan.name || '未分配'" :tone="item.membership ? 'success' : 'muted'" /><small class="cell-sub">{{ formatDate(item.membership?.expires_at, "长期") }}</small></td>
          <td><span v-for="tag in item.tags" :key="tag.id" class="account-tag" :style="{ borderColor: tag.color, color: tag.color }">{{ tag.name }}</span><span v-if="!item.tags.length">—</span></td>
          <td>{{ formatNumber(item.wallets.coins) }} 积分</td><td>{{ formatDate(item.created_at) }}</td>
          <td><button class="text-button" @click="Object.assign(selectedPlan, { account_id: item.account_id, plan_id: item.membership?.plan.id || plans.find((plan) => plan.is_default)?.id || 0, duration_days: item.membership?.plan.duration_days || 30 })">分配方案</button></td>
        </tr></template>
      </AdminDataTable>
    </section>

    <section class="membership-grid">
      <form class="panel settings-card" @submit.prevent="createPlan"><div class="panel-heading"><Plus :size="19" /><div><span class="section-kicker">MEMBERSHIP PLAN</span><h2>创建会员方案</h2></div></div><div class="form-grid"><label><span>方案代码</span><input v-model.trim="planForm.code" required pattern="[a-z][a-z0-9_-]+" /></label><label><span>方案名称</span><input v-model.trim="planForm.name" required /></label><label><span>有效天数</span><input v-model.number="planForm.duration_days" type="number" min="0" max="3650" /></label><label><span>兼容 Bot 等级</span><select v-model="planForm.legacy_level"><option value="a">A 白名单</option><option value="b">B 普通</option><option value="c">C 冻结</option><option value="d">D 未注册</option></select></label></div><label class="check-row"><input v-model="planForm.is_default" type="checkbox" /><span>设为默认注册方案</span></label><button class="primary-button"><Plus :size="16" /> 创建方案</button></form>
      <form class="panel settings-card" @submit.prevent="createTag"><div class="panel-heading"><Tags :size="19" /><div><span class="section-kicker">USER TAG</span><h2>创建运营标签</h2></div></div><label><span>标签名称</span><input v-model.trim="tagForm.name" required maxlength="64" /></label><label><span>标签颜色</span><input v-model="tagForm.color" type="color" /></label><label><span>说明</span><textarea v-model.trim="tagForm.description" maxlength="500" /></label><button class="primary-button"><Plus :size="16" /> 创建标签</button></form>
    </section>

    <section class="panel catalog-card">
      <div class="panel-heading"><BadgeCheck :size="19" /><div><span class="section-kicker">PLAN CATALOG</span><h2>现有会员方案</h2></div></div>
      <div class="catalog-list">
        <article v-for="plan in plans" :key="plan.id">
          <div><strong>{{ plan.name }}</strong><small>{{ plan.code }} · {{ plan.duration_days ? `${plan.duration_days} 天` : "长期" }} · Bot 等级 {{ plan.legacy_level.toUpperCase() }}</small></div>
          <StatusBadge :label="plan.is_default ? '默认方案' : plan.enabled ? '已启用' : '已停用'" :tone="plan.is_default ? 'success' : plan.enabled ? 'info' : 'muted'" />
          <button v-if="!plan.is_default" class="text-button" @click="updatePlan(plan, { is_default: true, enabled: true })">设为默认</button>
          <button v-if="!plan.is_default" class="text-button" @click="updatePlan(plan, { enabled: !plan.enabled })">{{ plan.enabled ? "停用" : "启用" }}</button>
        </article>
      </div>
      <div class="tag-catalog"><span v-for="tag in tags" :key="tag.id" class="managed-tag" :style="{ borderColor: tag.color, color: tag.color }"><i :style="{ background: tag.color }" />{{ tag.name }}<button title="删除标签" @click="deleteTag(tag)"><Trash2 :size="12" /></button></span><small v-if="!tags.length">暂无用户标签</small></div>
    </section>

    <section v-if="selectedIds.length" class="panel batch-tag-bar"><strong>已选择 {{ selectedIds.length }} 个账号</strong><select v-model="batchTag.mode"><option value="add">追加标签</option><option value="remove">移除标签</option><option value="replace">替换全部标签</option></select><select v-model="batchTag.tag_ids" multiple><option v-for="tag in tags" :key="tag.id" :value="tag.id">{{ tag.name }}</option></select><button class="primary-button" @click="applyTags">批量应用</button></section>

    <div v-if="selectedPlan.account_id" class="modal-layer"><form class="modal-card" @submit.prevent="assignMembership"><header><div><span class="section-kicker">ASSIGN PLAN</span><h2>分配会员方案</h2></div><button type="button" class="icon-button" @click="selectedPlan.account_id = ''">×</button></header><label><span>会员方案</span><select v-model.number="selectedPlan.plan_id" required><option v-for="plan in plans.filter((item) => item.enabled)" :key="plan.id" :value="plan.id">{{ plan.name }}</option></select></label><label><span>有效天数</span><input v-model.number="selectedPlan.duration_days" type="number" min="0" max="3650" /></label><footer><button type="button" class="secondary-button" @click="selectedPlan.account_id = ''">取消</button><button class="primary-button">确认分配</button></footer></form></div>
  </div>
</template>

<style scoped>
.membership-grid{display:grid;grid-template-columns:1fr 1fr;gap:17px}.settings-card{display:grid;gap:15px;padding:22px}.settings-card label{display:grid;gap:7px}.settings-card input,.settings-card select,.settings-card textarea,.batch-tag-bar select{padding:10px 12px;color:var(--text);border:1px solid var(--border);border-radius:10px;background:rgba(255,255,255,.035)}.account-tag{display:inline-block;margin:2px 4px 2px 0;padding:3px 7px;border:1px solid;border-radius:999px;font-size:9px}.batch-tag-bar{display:flex;align-items:center;gap:12px;padding:16px 20px}.batch-tag-bar select[multiple]{min-width:180px;max-height:90px}.catalog-card{padding:22px}.catalog-list{display:grid;gap:8px;margin-top:15px}.catalog-list article{display:grid;grid-template-columns:minmax(220px,1fr) auto auto auto;align-items:center;gap:12px;padding:12px 14px;border:1px solid var(--border);border-radius:11px}.catalog-list article small{display:block;margin-top:4px;color:var(--muted-2)}.tag-catalog{display:flex;flex-wrap:wrap;gap:8px;margin-top:17px;padding-top:17px;border-top:1px solid var(--border)}.managed-tag{display:inline-flex;align-items:center;gap:6px;padding:5px 8px;border:1px solid;border-radius:999px;font-size:10px}.managed-tag i{width:6px;height:6px;border-radius:50%}.managed-tag button{display:grid;padding:1px;color:inherit;border:0;background:none;cursor:pointer}@media(max-width:850px){.membership-grid{grid-template-columns:1fr}.batch-tag-bar{align-items:stretch;flex-direction:column}.catalog-list article{grid-template-columns:1fr auto}.catalog-list article .text-button{justify-self:start}}
</style>
