<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { Activity, ArrowRight, Coins, ShieldCheck, Sparkles, TimerReset, UserCheck, Users, Workflow } from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import { api } from "@/lib/api";
import { actionLabel, formatDate, formatNumber } from "@/lib/format";
import type { AdminOverview, AuditLog } from "@/types";

const overview = ref<AdminOverview | null>(null);
const audit = ref<AuditLog[]>([]);
const loading = ref(true);
const levelTotal = computed(() =>
  Object.values(overview.value?.levels || {}).reduce((sum, value) => sum + value, 0),
);
const maxLevel = computed(() => Math.max(...Object.values(overview.value?.levels || {}), 1));

onMounted(async () => {
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
    <header class="page-heading">
      <div>
        <span class="eyebrow">OPERATIONS OVERVIEW</span>
        <h1>运营概览</h1>
        <p>用户、账户与敏感操作的实时摘要，帮助你快速发现需要处理的事情。</p>
      </div>
      <span class="date-chip"><span class="status-dot" /> 数据库已连接</span>
    </header>

    <LoadingBlock v-if="loading" />
    <template v-else-if="overview">
      <section class="stats-grid admin-stats">
        <article class="stat-card accent">
          <span class="stat-icon"><Users :size="21" /></span>
          <div><small>成员总数</small><strong>{{ formatNumber(overview.users_total) }}</strong><p>全部 Telegram 成员</p></div>
        </article>
        <article class="stat-card">
          <span class="stat-icon cyan"><UserCheck :size="21" /></span>
          <div><small>已开通账户</small><strong>{{ formatNumber(overview.accounts_active) }}</strong><p>当前存在 Emby 账户</p></div>
        </article>
        <article class="stat-card">
          <span class="stat-icon gold"><TimerReset :size="21" /></span>
          <div><small>即将到期</small><strong class="tone-warning">{{ formatNumber(overview.expiring_soon) }}</strong><p>未来 7 天需关注</p></div>
        </article>
        <article class="stat-card">
          <span class="stat-icon violet"><Activity :size="21" /></span>
          <div><small>今日操作</small><strong>{{ formatNumber(overview.audit_events_today) }}</strong><p>已记录的审计事件</p></div>
        </article>
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
        <RouterLink to="/users"><Users :size="20" /><span><strong>管理用户</strong><small>检索、查看与调整积分</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink to="/tasks"><Workflow :size="20" /><span><strong>任务中心</strong><small>运行维护任务并查看状态</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink to="/roles"><ShieldCheck :size="20" /><span><strong>角色权限</strong><small>配置后台访问范围</small></span><ArrowRight :size="17" /></RouterLink>
        <RouterLink to="/audit"><Activity :size="20" /><span><strong>审计日志</strong><small>追踪敏感管理操作</small></span><ArrowRight :size="17" /></RouterLink>
      </section>
    </template>
  </div>
</template>
