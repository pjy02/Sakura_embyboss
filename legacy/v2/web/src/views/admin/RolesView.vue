<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { CirclePlus, Crown, KeyRound, Pencil, Shield, ShieldCheck, Trash2, UserRoundCog, X } from "lucide-vue-next";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import { api } from "@/lib/api";
import { useSessionStore } from "@/stores/session";
import type { PermissionCatalogGroup, Role } from "@/types";

const sessionStore = useSessionStore();
const roles = ref<Role[]>([]);
const catalog = ref<PermissionCatalogGroup[]>([]);
const loading = ref(true);
const modalOpen = ref(false);
const editing = ref<Role | null>(null);
const deleteTarget = ref<Role | null>(null);
const busy = ref(false);
const error = ref("");
const form = reactive({ name: "", permissions: [] as string[] });
const isOwner = computed(() => sessionStore.session?.roles.includes("owner"));
const icons = { owner: Crown, admin: ShieldCheck, operator: UserRoundCog, auditor: KeyRound, user: Shield };
const descriptions: Record<string, string> = {
  owner: "系统所有者，拥有全部能力与角色分配权。",
  admin: "负责全局运营、内容审核和安全配置。",
  operator: "处理用户、交易、工单和内容等常规工作。",
  auditor: "只读访问运营数据、安全事件和审计记录。",
  user: "普通用户，仅可访问自己的账户数据。",
};
const permissionMap = computed(() => Object.fromEntries(catalog.value.flatMap((group) => group.items.map((item) => [item.permission, item.label]))));
async function load() {
  loading.value = true;
  try {
    const [roleResult, catalogResult] = await Promise.all([
      api<{ items: Role[] }>("/admin/roles"),
      api<{ items: PermissionCatalogGroup[] }>("/admin/roles/catalog"),
    ]);
    roles.value = roleResult.items; catalog.value = catalogResult.items;
  } finally { loading.value = false; }
}
function openEditor(role?: Role) {
  editing.value = role || null; form.name = role?.name || ""; form.permissions = [...(role?.permissions || [])]; error.value = ""; modalOpen.value = true;
}
function toggle(permission: string) {
  form.permissions = form.permissions.includes(permission) ? form.permissions.filter((item) => item !== permission) : [...form.permissions, permission];
}
async function save() {
  busy.value = true; error.value = "";
  try {
    if (editing.value) await api(`/admin/roles/${editing.value.id}`, { method: "PATCH", body: JSON.stringify({ permissions: form.permissions }) });
    else await api("/admin/roles", { method: "POST", body: JSON.stringify(form) });
    modalOpen.value = false; await load();
  } catch (e) { error.value = e instanceof Error ? e.message : "角色保存失败"; }
  finally { busy.value = false; }
}
async function remove() {
  if (!deleteTarget.value) return; busy.value = true;
  try { await api(`/admin/roles/${deleteTarget.value.id}`, { method: "DELETE" }); deleteTarget.value = null; await load(); }
  finally { busy.value = false; }
}
onMounted(load);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="ROLE BASED ACCESS" title="角色权限" description="按最小权限原则定义后台职责边界，角色变更会立即影响下一次权限校验。" :icon="ShieldCheck"><template #actions><button v-if="isOwner" class="primary-button" @click="openEditor()"><CirclePlus :size="17" /> 新建角色</button></template></AdminPageHeader>
    <LoadingBlock v-if="loading" />
    <section v-else class="role-grid">
      <article v-for="role in roles" :key="role.id" class="panel role-card" :data-role="role.name"><header><span><component :is="icons[role.name as keyof typeof icons] || Shield" :size="22" /></span><div><h2>{{ role.name }}</h2><small>{{ role.is_system ? "系统内置角色" : "自定义角色" }} · {{ role.member_count || 0 }} 名成员</small></div></header><p>{{ descriptions[role.name] || "自定义运营职责和后台访问范围。" }}</p><div class="permission-list"><span v-for="permission in role.permissions" :key="permission">{{ permissionMap[permission] || permission }}</span></div><footer v-if="isOwner && !['owner','user'].includes(role.name)"><button class="text-button" @click="openEditor(role)"><Pencil :size="14" /> 编辑权限</button><button v-if="!role.is_system" class="text-button danger-text" @click="deleteTarget = role"><Trash2 :size="14" /> 删除</button></footer></article>
    </section>
    <section class="info-strip"><ShieldCheck :size="20" /><div><strong>权限变更即时生效</strong><p>下一次接口请求会重新从数据库解析角色；所有创建、修改和删除操作都会进入审计日志。</p></div></section>
    <div v-if="modalOpen" class="modal-layer"><form class="modal-card role-editor-card" @submit.prevent="save"><header><div><span class="section-kicker">PERMISSION MATRIX</span><h2>{{ editing ? `编辑 ${editing.name}` : "新建自定义角色" }}</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header><label v-if="!editing"><span>角色标识</span><input v-model.trim="form.name" required pattern="[a-z][a-z0-9_-]{2,31}" placeholder="例如 content_moderator" /></label><div class="permission-matrix"><section v-for="group in catalog" :key="group.group"><h3>{{ group.group }}</h3><label v-for="item in group.items" :key="item.permission"><input type="checkbox" :checked="form.permissions.includes(item.permission)" @change="toggle(item.permission)" /><span><strong>{{ item.label }}</strong><small>{{ item.permission }}</small></span></label></section></div><p v-if="error" class="form-error">{{ error }}</p><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="busy">{{ busy ? "保存中…" : "保存角色" }}</button></footer></form></div>
    <ConfirmDialog :open="Boolean(deleteTarget)" title="删除自定义角色？" description="只有没有成员使用的自定义角色才能删除。" confirm-label="确认删除" tone="danger" :busy="busy" @close="deleteTarget = null" @confirm="remove">{{ deleteTarget?.name }}</ConfirmDialog>
  </div>
</template>
