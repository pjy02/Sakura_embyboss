<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  ChevronLeft,
  ChevronRight,
  Coins,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  UserRound,
  X,
} from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api, idempotencyKey } from "@/lib/api";
import { formatDate, formatNumber, initials, levelLabel } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { Role, UserProfile } from "@/types";

const sessionStore = useSessionStore();
const route = useRoute();
const items = ref<UserProfile[]>([]);
const total = ref(0);
const loading = ref(true);
const search = ref("");
const level = ref("");
const offset = ref(0);
const limit = 20;
const selected = ref<UserProfile | null>(null);
const drawerLoading = ref(false);
const pointOpen = ref(false);
const roleOpen = ref(false);
const success = ref("");
const error = ref("");
const roles = ref<Role[]>([]);
const pointForm = reactive({ amount: 0, balance_type: "coins", reason: "", allow_negative: false });
const roleForm = reactive({ role: "operator", enabled: true });
let searchTimer: number | undefined;

const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)));
const currentPage = computed(() => Math.floor(offset.value / limit) + 1);
const canUpdate = computed(() =>
  sessionStore.session?.permissions.some((item) => item === "*" || item === "users:*" || item === "users:update"),
);
const isOwner = computed(() => sessionStore.session?.roles.includes("owner"));

async function load() {
  loading.value = true;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset.value),
    sort_by: "tg",
    sort_order: "desc",
  });
  if (search.value) params.set("search", search.value);
  if (level.value) params.set("level", level.value);
  try {
    const result = await api<{ items: UserProfile[]; total: number }>(`/admin/users?${params}`);
    items.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function showUser(user: UserProfile) {
  selected.value = user;
  drawerLoading.value = true;
  try {
    selected.value = await api<UserProfile>(`/admin/users/${user.tg}`);
  } finally {
    drawerLoading.value = false;
  }
}

async function openRequestedUser(value: unknown) {
  const requestedUser = typeof value === "string" ? Number(value) : 0;
  if (!Number.isSafeInteger(requestedUser) || requestedUser <= 0) return;
  drawerLoading.value = true;
  try {
    selected.value = await api<UserProfile>(`/admin/users/${requestedUser}`);
  } catch {
    selected.value = null;
  } finally {
    drawerLoading.value = false;
  }
}

function closeDialogs() {
  pointOpen.value = false;
  roleOpen.value = false;
  error.value = "";
}

async function adjustPoints() {
  if (!selected.value) return;
  error.value = "";
  try {
    await api(`/admin/users/${selected.value.tg}/points`, {
      method: "POST",
      idempotencyKey: idempotencyKey("points"),
      body: JSON.stringify(pointForm),
    });
    success.value = "积分调整已完成，并写入审计日志";
    closeDialogs();
    await Promise.all([load(), showUser(selected.value)]);
    window.setTimeout(() => (success.value = ""), 3500);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "积分调整失败";
  }
}

async function saveRole() {
  if (!selected.value) return;
  error.value = "";
  try {
    await api(`/admin/users/${selected.value.tg}/role`, {
      method: "PUT",
      body: JSON.stringify(roleForm),
    });
    success.value = roleForm.enabled ? "角色已分配" : "角色已移除";
    closeDialogs();
    await showUser(selected.value);
    window.setTimeout(() => (success.value = ""), 3500);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "角色变更失败";
  }
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit);
  load();
}

watch(search, () => {
  window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(() => {
    offset.value = 0;
    load();
  }, 350);
});
watch(level, () => {
  offset.value = 0;
  load();
});
watch(() => route.query.user, openRequestedUser);

onMounted(async () => {
  await Promise.all([
    load(),
    api<{ items: Role[] }>("/admin/roles").then((result) => (roles.value = result.items)).catch(() => undefined),
  ]);
  await openRequestedUser(route.query.user);
});
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="MEMBER DIRECTORY"
      title="站点账号"
      description="检索成员、查看账户状态，并执行有审计记录的积分和权限调整。"
      :icon="UserRound"
    >
      <template #meta><span class="date-chip"><UserRound :size="16" /> 共 {{ formatNumber(total) }} 位成员</span></template>
    </AdminPageHeader>

    <div v-if="success" class="success-banner"><ShieldCheck :size="17" /> {{ success }}</div>
    <section class="panel table-panel">
      <FilterBar>
        <label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="搜索用户名、Telegram ID 或 Emby ID" /></label>
        <label class="select-box"><SlidersHorizontal :size="17" /><select v-model="level"><option value="">全部等级</option><option value="a">A · 白名单</option><option value="b">B · 高级会员</option><option value="c">C · 正式会员</option><option value="d">D · 普通会员</option></select></label>
      </FilterBar>
      <AdminDataTable
        class="user-table"
        :loading="loading"
        :empty="!items.length"
        empty-title="没有匹配的用户"
        empty-description="请调整搜索关键词或等级筛选。"
      >
        <template #head><tr><th>用户</th><th>等级</th><th>积分</th><th>注册天数</th><th>到期时间</th><th>状态</th><th /></tr></template>
        <template #body>
          <tr v-for="user in items" :key="user.tg" @click="showUser(user)">
              <td><div class="table-user"><span>{{ initials(user.name, user.tg) }}</span><div><strong>{{ user.name || "未创建账户" }}</strong><small>TG · {{ user.tg }}</small></div></div></td>
              <td><span class="level-badge" :data-level="user.level">{{ user.level.toUpperCase() }} · {{ levelLabel(user.level) }}</span></td>
              <td class="strong-cell">{{ formatNumber(user.coins) }}</td>
              <td>{{ formatNumber(user.registration_days) }}</td>
              <td>{{ formatDate(user.expires_at, "长期") }}</td>
              <td><StatusBadge :label="user.has_account ? '已开通' : '未开通'" :tone="user.has_account ? 'success' : 'muted'" /></td>
              <td><button class="text-button">查看</button></td>
          </tr>
        </template>
        <template #footer><div class="pagination"><span>第 {{ currentPage }} / {{ pages }} 页</span><div><button :disabled="offset === 0" @click="page(-1)"><ChevronLeft :size="16" /> 上一页</button><button :disabled="currentPage >= pages" @click="page(1)">下一页 <ChevronRight :size="16" /></button></div></div></template>
      </AdminDataTable>
    </section>

    <DetailDrawer
      :open="Boolean(selected)"
      title="用户详情"
      eyebrow="MEMBER DETAIL"
      :description="selected ? `Telegram ID · ${selected.tg}` : ''"
      @close="selected = null"
    >
      <template v-if="selected">
        <LoadingBlock v-if="drawerLoading" />
        <template v-else>
          <div class="drawer-profile"><span>{{ initials(selected.name, selected.tg) }}</span><div><h3>{{ selected.name || "未创建 Emby 账户" }}</h3><p>Telegram ID · {{ selected.tg }}</p></div></div>
          <div class="drawer-badges"><span class="level-badge" :data-level="selected.level">{{ levelLabel(selected.level) }}</span><StatusBadge :label="selected.has_account ? '账户正常' : '未开通'" :tone="selected.has_account ? 'success' : 'muted'" /></div>
          <dl class="detail-list boxed">
            <div><dt>当前积分</dt><dd>{{ formatNumber(selected.coins) }}</dd></div>
            <div><dt>注册天数</dt><dd>{{ formatNumber(selected.registration_days) }}</dd></div>
            <div><dt>Emby ID</dt><dd>{{ selected.embyid || "—" }}</dd></div>
            <div><dt>注册时间</dt><dd>{{ formatDate(selected.created_at) }}</dd></div>
            <div><dt>账户到期</dt><dd>{{ formatDate(selected.expires_at, "长期") }}</dd></div>
            <div><dt>后台角色</dt><dd>{{ selected.roles?.join("、") || "普通用户" }}</dd></div>
          </dl>
        </template>
      </template>
      <template #actions>
        <template v-if="selected">
          <button v-if="canUpdate" class="primary-button" @click="pointOpen = true"><Coins :size="17" /> 调整积分/天数</button>
          <button v-if="isOwner" class="secondary-button" @click="roleOpen = true"><ShieldCheck :size="17" /> 管理角色</button>
        </template>
      </template>
    </DetailDrawer>

    <div v-if="pointOpen" class="modal-layer">
      <form class="modal-card" @submit.prevent="adjustPoints">
        <header><div><span class="section-kicker">BALANCE ADJUSTMENT</span><h2>调整账户余额</h2></div><button type="button" class="icon-button" @click="closeDialogs"><X :size="19" /></button></header>
        <p class="modal-context">目标用户：{{ selected?.name || selected?.tg }}</p>
        <label><span>余额类型</span><select v-model="pointForm.balance_type"><option value="coins">积分</option><option value="registration_days">注册天数</option></select></label>
        <label><span>变动数量</span><input v-model.number="pointForm.amount" type="number" required placeholder="正数增加，负数扣减" /></label>
        <label><span>操作原因</span><textarea v-model.trim="pointForm.reason" required minlength="3" maxlength="255" placeholder="用于审计，请填写清晰原因" /></label>
        <label class="check-row"><input v-model="pointForm.allow_negative" type="checkbox" /><span>允许扣减后余额为负数</span></label>
        <p v-if="error" class="form-error">{{ error }}</p>
        <footer><button type="button" class="secondary-button" @click="closeDialogs">取消</button><button class="primary-button">确认调整</button></footer>
      </form>
    </div>

    <div v-if="roleOpen" class="modal-layer">
      <form class="modal-card" @submit.prevent="saveRole">
        <header><div><span class="section-kicker">ROLE ASSIGNMENT</span><h2>管理后台角色</h2></div><button type="button" class="icon-button" @click="closeDialogs"><X :size="19" /></button></header>
        <p class="modal-context">只有所有者可以改变成员的后台角色。</p>
        <label><span>角色</span><select v-model="roleForm.role"><option v-for="role in roles.filter((item) => !['owner', 'user'].includes(item.name))" :key="role.id" :value="role.name">{{ role.name }}</option></select></label>
        <label><span>操作</span><select v-model="roleForm.enabled"><option :value="true">分配角色</option><option :value="false">移除角色</option></select></label>
        <p v-if="error" class="form-error">{{ error }}</p>
        <footer><button type="button" class="secondary-button" @click="closeDialogs">取消</button><button class="primary-button">确认变更</button></footer>
      </form>
    </div>
  </div>
</template>
