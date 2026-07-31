<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { Activity, ArrowRight, Coins, HardDrive, MonitorPlay, Network, ShieldCheck, Sparkles, TimerReset, UserCheck, Users, Workflow } from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import { api } from "@/lib/api";
import { actionLabel, formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { AdminOverview, AuditLog, CoreDashboard } from "@/types";

const sessionStore = useSessionStore();
const overview = ref<AdminOverview | null>(null);
const core = ref<CoreDashboard | null>(null);
const audit = ref<AuditLog[]>([]);
const loading = ref(true);
const levelTotal = computed(() =>
  Object.values(overview.value?.levels || {}).reduce((sum, value) => sum + value, 0),
);
const maxLevel = computed(() => Math.max(...Object.values(overview.value?.levels || {}), 1));
function hasPermission(required: string) {
  return Boolean(
    sessionStore.session?.permissions.some(
      (permission) =>
        permission === "*" ||
        permission === required ||
        permission === `${required.split(":")[0]}:*`,
    ),
  );
}

onMounted(async () => {
  api<CoreDashboard>("/admin/dashboard/core")
    .then((result) => (core.value = result))
    .catch(() => undefined);
  try {
    const [summary, logs] = await Promise.all([
      api<AdminOverview>("/admin/overview"),
      api<{ items: AuditLog[] }>("/admin/audit?limit=6").catch(() => ({ items: [] })),
    ]);
    overview.value = summary;
    audit.value = logs.items;
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="OPERATIONS OVERVIEW"
      title="运营仪表盘"
      description="用户、账户与敏感操作的实时摘要，帮助你快速发现需要处理的事情。"
      :icon="Activity"
    >
      <template #meta><span class="date-chip"><span class="status-dot" /> 数据库已连接</span></template>
    </AdminPageHeader>

    <LoadingBlock v-if="loading" />
    <template v-else-if="overview">
      <section class="stats-grid admin-stats">
        <MetricCard label="成员总数" :value="formatNumber(overview.users_total)" caption="全部 Telegram 成员" :icon="Users" featured />
        <MetricCard label="已开通账户" :value="formatNumber(overview.accounts_active)" caption="当前存在 Emby 账户" :icon="UserCheck" tone="cyan" />
        <MetricCard label="即将到期" :value="formatNumber(overview.expiring_soon)" caption="未来 7 天需关注" :icon="TimerReset" tone="gold" />
        <MetricCard label="今日操作" :value="formatNumber(overview.audit_events_today)" caption="已记录的审计事件" :icon="Activity" tone="violet" />
      </section>

      <section v-if="core" class="operations-pulse panel">
        <div><span class="pulse-icon live"><MonitorPlay :size="18" /></span><p><small>在线播放</small><strong>{{ formatNumber(core.live_sessions) }}</strong></p></div>
        <div><span class="pulse-icon"><Activity :size="18" /></span><p><small>今日播放</small><strong>{{ formatNumber(core.plays_today) }}</strong></p></div>
        <div><span class="pulse-icon device"><HardDrive :size="18" /></span><p><small>已知设备</small><strong>{{ formatNumber(core.known_devices) }}</strong><em v-if="core.risk_devices">{{ core.risk_devices }} 台需关注</em></p></div>
        <div><span class="pulse-icon line"><Network :size="18" /></span><p><small>健康线路</small><strong>{{ core.lines_healthy }} / {{ core.lines_total }}</strong></p></div>
      </section>

      <section class="content-grid admin-overview-grid">
        <article class="panel level-panel">
          <div class="panel-heading">
            <div><span class="section-kicker">MEMBER LEVELS</span><h2>用户等级分布</h2></div>
            <span class="page-count">{{ formatNumber(levelTotal) }} 人</span>
          </div>
          <div class="level-chart">
            <div v-for="level in ['a', 'b', 'c', 'd']" :key="level" class="level-row">
              <span class="level-badge" :data-level="level">{{ level.toUpperCase() }}</span>
              <div class="level-track">
                <i :style="{ width: `${((overview.levels[level] || 0) / maxLevel) * 100}%` }" :data-level="level" />
              </div>
              <strong>{{ overview.levels[level] || 0 }}</strong>
            </div>
          </div>
          <div class="mini-metrics">
            <div><Coins :size="17" /><span>积分总量</span><strong>{{ formatNumber(overview.coins_total) }}</strong></div>
            <div><Sparkles :size="17" /><span>今日积分变动</span><strong>{{ formatNumber(overview.point_changes_today) }}</strong></div>
          </div>
        </article>

        <article class="panel recent-panel">
          <div class="panel-heading">
            <div><span class="section-kicker">AUDIT STREAM</span><h2>最近操作</h2></div>
            <RouterLink to="/audit">全部日志 <ArrowRight :size="15" /></RouterLink>
          </div>
          <div class="audit-stream">
            <div v-for="item in audit" :key="item.id" class="audit-stream-item">
              <span><ShieldCheck :size="16" /></span>
              <div><strong>{{ actionLabel(item.action) }}</strong><small>{{ item.actor_kind }} · {{ item.actor_id }} · {{ formatDate(item.created_at) }}</small></div>
              <i :class="item.outcome === 'success' ? 'success-dot' : 'error-dot'" />
            </div>
          </div>
        </article>
      </section>

      <section class="quick-actions">
        <RouterLink v-if="hasPermission('users:read')" to="/users"><Users :size="20" /><span><strong>站点账号</strong><small>检索、查看与调整积分</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('playback:read')" to="/playback/live"><MonitorPlay :size="20" /><span><strong>在线播放</strong><small>查看并处理当前会话</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('devices:read')" to="/devices"><HardDrive :size="20" /><span><strong>设备管理</strong><small>识别共享与异常设备</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('lines:read')" to="/lines"><Network :size="20" /><span><strong>线路管理</strong><small>探测健康与维护状态</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('tasks:read')" to="/tasks"><Workflow :size="20" /><span><strong>系统任务</strong><small>运行维护任务并查看状态</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('roles:read')" to="/roles"><ShieldCheck :size="20" /><span><strong>角色权限</strong><small>配置后台访问范围</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink v-if="hasPermission('audit:read')" to="/audit"><Activity :size="20" /><span><strong>操作记录</strong><small>追踪敏感管理操作</small></span><ArrowRight :size="17" /></RouterLink>
      </section>
    </template>
  </div>
</template>
