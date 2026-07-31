<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { ArrowRight, CalendarDays, CheckCircle2, CircleDollarSign, Clapperboard, Coins, Crown, Film, MessageCircle, Sparkles } from "lucide-vue-next";
import LoadingBlock from "@/components/LoadingBlock.vue";
import { api } from "@/lib/api";
import { daysUntil, formatDate, formatNumber, initials, levelLabel } from "@/lib/format";
import { useRealtimeEvents } from "@/composables/useRealtime";
import type { PointTransaction, UserProfile } from "@/types";

const profile = ref<UserProfile | null>(null);
const transactions = ref<PointTransaction[]>([]);
const loading = ref(true);
const error = ref("");

const remainingDays = computed(() => daysUntil(profile.value?.expires_at));
const expiryTone = computed(() => {
  if (remainingDays.value === null) return "neutral";
  if (remainingDays.value <= 7) return "danger";
  if (remainingDays.value <= 30) return "warning";
  return "good";
});

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    const [user, history] = await Promise.all([
      api<UserProfile>("/me"),
      api<{ items: PointTransaction[] }>("/me/point-transactions?limit=5"),
    ]);
    profile.value = user;
    transactions.value = history.items;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "加载账户信息失败";
  } finally {
    loading.value = false;
  }
}

onMounted(load);
useRealtimeEvents(
  ["user.updated", "points.changed", "partition.changed"],
  () => load(true),
);
</script>

<template>
  <div class="page-stack">
    <header class="page-heading">
      <div>
        <span class="eyebrow">PERSONAL OVERVIEW</span>
        <h1>你好，{{ profile?.name || "Sakura 用户" }}</h1>
        <p>这是你的账户近况，Bot 与网页端的每一次变更都会同步到这里。</p>
      </div>
      <span class="date-chip"><CalendarDays :size="16" /> {{ new Date().toLocaleDateString("zh-CN") }}</span>
    </header>

    <LoadingBlock v-if="loading" />
    <div v-else-if="error" class="error-banner">{{ error }}</div>
    <template v-else-if="profile">
      <section class="hero-profile">
        <div class="profile-main">
          <div class="profile-avatar">{{ initials(profile.name, profile.tg) }}</div>
          <div>
            <div class="profile-name">
              <h2>{{ profile.name || "尚未创建 Emby 账户" }}</h2>
              <span class="level-badge" :data-level="profile.level">
                <Crown :size="14" /> {{ levelLabel(profile.level) }}
              </span>
            </div>
            <p>Telegram ID · {{ profile.tg }}</p>
            <div class="profile-flags">
              <span><CheckCircle2 :size="15" /> 身份已验证</span>
              <span v-if="profile.has_account"><Film :size="15" /> Emby 已绑定</span>
            </div>
          </div>
        </div>
        <div class="profile-decoration">
          <span>桜</span>
          <small>MEMBER</small>
        </div>
      </section>

      <section class="stats-grid portal-stats">
        <article class="stat-card accent">
          <span class="stat-icon"><Coins :size="21" /></span>
          <div>
            <small>可用积分</small>
            <strong>{{ formatNumber(profile.coins) }}</strong>
            <p>可用于兑换与续期</p>
          </div>
        </article>
        <article class="stat-card">
          <span class="stat-icon cyan"><CalendarDays :size="21" /></span>
          <div>
            <small>注册额度</small>
            <strong>{{ formatNumber(profile.registration_days) }} <em>天</em></strong>
            <p>当前可使用的注册天数</p>
          </div>
        </article>
        <article class="stat-card">
          <span class="stat-icon gold"><Sparkles :size="21" /></span>
          <div>
            <small>账户有效期</small>
            <strong :class="`tone-${expiryTone}`">
              {{ remainingDays === null ? "长期" : `${Math.max(remainingDays, 0)} 天` }}
            </strong>
            <p>{{ formatDate(profile.expires_at) }}</p>
          </div>
        </article>
      </section>

      <section class="content-grid">
        <article class="panel recent-panel">
          <div class="panel-heading">
            <div>
              <span class="section-kicker">RECENT ACTIVITY</span>
              <h2>最近积分动态</h2>
            </div>
            <RouterLink to="/points">查看全部 <ArrowRight :size="15" /></RouterLink>
          </div>
          <div v-if="transactions.length" class="activity-list">
            <div v-for="item in transactions" :key="item.id" class="activity-item">
              <span class="activity-mark" :class="{ positive: item.amount > 0 }">
                {{ item.amount > 0 ? "+" : "−" }}
              </span>
              <div>
                <strong>{{ item.reason }}</strong>
                <small>{{ formatDate(item.created_at) }} · {{ item.balance_type === "coins" ? "积分" : "注册天数" }}</small>
              </div>
              <span class="amount" :class="{ positive: item.amount > 0 }">
                {{ item.amount > 0 ? "+" : "" }}{{ item.amount }}
              </span>
            </div>
          </div>
          <div v-else class="inline-empty">还没有积分变动记录</div>
        </article>

        <article class="panel account-snapshot">
          <div class="panel-heading">
            <div>
              <span class="section-kicker">ACCOUNT</span>
              <h2>账户信息</h2>
            </div>
          </div>
          <dl class="detail-list">
            <div><dt>Emby 用户名</dt><dd>{{ profile.name || "未创建" }}</dd></div>
            <div><dt>账户 ID</dt><dd>{{ profile.embyid || "—" }}</dd></div>
            <div><dt>注册时间</dt><dd>{{ formatDate(profile.created_at) }}</dd></div>
            <div><dt>最近签到</dt><dd>{{ formatDate(profile.checked_in_at) }}</dd></div>
          </dl>
          <RouterLink class="secondary-button wide" to="/account">管理账户安全 <ArrowRight :size="15" /></RouterLink>
        </article>
      </section>

      <section class="quick-actions">
        <RouterLink to="/billing">
          <CircleDollarSign :size="20" />
          <span><strong>充值中心</strong><small>提交充值订单并查看入账状态</small></span>
          <ArrowRight :size="15" />
        </RouterLink>
        <RouterLink to="/tickets">
          <MessageCircle :size="20" />
          <span><strong>服务工单</strong><small>联系管理员并跟踪处理进度</small></span>
          <ArrowRight :size="15" />
        </RouterLink>
        <RouterLink to="/requests">
          <Clapperboard :size="20" />
          <span><strong>求片订阅</strong><small>提交作品并查看下载入库进度</small></span>
          <ArrowRight :size="15" />
        </RouterLink>
      </section>
    </template>
  </div>
</template>
