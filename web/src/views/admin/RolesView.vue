<script setup lang="ts">
import { onMounted, ref } from "vue";
import { Crown, KeyRound, Shield, ShieldCheck, UserRoundCog } from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import { api } from "@/lib/api";
import type { Role } from "@/types";

const roles = ref<Role[]>([]);
const loading = ref(true);
const icons = { owner: Crown, admin: ShieldCheck, operator: UserRoundCog, auditor: KeyRound, user: Shield };
const descriptions: Record<string, string> = {
  owner: "系统所有者，拥有全部能力与角色分配权。",
  admin: "负责日常全局运营，可管理用户和业务资源。",
  operator: "处理用户、兑换码、分区和任务等常规工作。",
  auditor: "只读访问用户、任务、安全事件和审计记录。",
  user: "普通用户，仅可访问自己的账户数据。",
};

function permissionLabel(permission: string) {
  const map: Record<string, string> = {
    "*": "全部权限",
    "users:*": "用户管理",
    "users:read": "查看用户",
    "users:update": "调整用户",
    "codes:*": "兑换码管理",
    "partitions:*": "分区管理",
    "tasks:*": "任务管理",
    "tasks:read": "查看任务",
    "audit:read": "查看审计",
    "security:read": "查看安全事件",
    "settings:read": "查看设置",
    "roles:read": "查看角色",
    "self:*": "个人中心",
  };
  return map[permission] || permission;
}

onMounted(async () => {
  try {
    roles.value = (await api<{ items: Role[] }>("/admin/roles")).items;
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="ROLE BASED ACCESS"
      title="角色权限"
      description="角色定义后台访问边界；具体成员分配可在用户详情中完成。"
      :icon="ShieldCheck"
    />
    <LoadingBlock v-if="loading" />
    <section v-else class="role-grid">
      <article v-for="role in roles" :key="role.id" class="panel role-card" :data-role="role.name">
        <header><span><component :is="icons[role.name as keyof typeof icons] || Shield" :size="22" /></span><div><h2>{{ role.name }}</h2><small>{{ role.is_system ? "系统内置角色" : "自定义角色" }}</small></div></header>
        <p>{{ descriptions[role.name] || "用于限定后台功能访问范围。" }}</p>
        <div class="permission-list"><span v-for="permission in role.permissions" :key="permission">{{ permissionLabel(permission) }}</span></div>
      </article>
    </section>
    <section class="info-strip"><ShieldCheck :size="20" /><div><strong>最小权限原则</strong><p>为协作者选择满足工作需要的最低角色；所有角色变更都会进入审计日志。</p></div></section>
  </div>
</template>
