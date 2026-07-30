<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ChevronLeft, ChevronRight, Coins, RefreshCw } from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import { api } from "@/lib/api";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { formatDate, formatNumber } from "@/lib/format";
import type { PointTransaction, UserProfile } from "@/types";

const items = ref<PointTransaction[]>([]);
const profile = ref<UserProfile | null>(null);
const loading = ref(true);
const offset = ref(0);
const limit = 20;
const canNext = computed(() => items.value.length === limit);

async function load() {
  loading.value = true;
  try {
    const [history, user] = await Promise.all([
      api<{ items: PointTransaction[] }>(`/me/point-transactions?limit=${limit}&offset=${offset.value}`),
      profile.value ? Promise.resolve(profile.value) : api<UserProfile>("/me"),
    ]);
    items.value = history.items;
    profile.value = user;
  } finally {
    loading.value = false;
  }
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit);
  load();
}

onMounted(load);
useRealtimeEvents(["points.changed"], () => load());
</script>

<template>
  <div class="page-stack">
    <header class="page-heading">
      <div>
        <span class="eyebrow">POINTS LEDGER</span>
        <h1>积分明细</h1>
        <p>每一笔积分和注册天数变动都拥有完整、不可混淆的流水记录。</p>
      </div>
      <button class="secondary-button" @click="load"><RefreshCw :size="16" /> 刷新</button>
    </header>

    <section class="balance-banner">
      <div><span class="stat-icon"><Coins :size="22" /></span><small>当前积分余额</small></div>
      <strong>{{ formatNumber(profile?.coins) }}</strong>
      <p>积分用途和兑换规则以 Bot 当前配置为准。</p>
    </section>

    <section class="panel table-panel">
      <div class="panel-heading">
        <div>
          <span class="section-kicker">TRANSACTIONS</span>
          <h2>账户流水</h2>
        </div>
        <span class="page-count">第 {{ Math.floor(offset / limit) + 1 }} 页</span>
      </div>
      <LoadingBlock v-if="loading" />
      <EmptyState v-else-if="!items.length" title="暂无积分流水" />
      <div v-else class="responsive-table">
        <table>
          <thead><tr><th>时间</th><th>类型</th><th>原因</th><th>操作来源</th><th>变动</th><th>结余</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>{{ formatDate(item.created_at) }}</td>
              <td><span class="soft-badge">{{ item.balance_type === "coins" ? "积分" : "注册天数" }}</span></td>
              <td class="strong-cell">{{ item.reason }}</td>
              <td>{{ item.actor_kind === "web" ? "网页管理" : item.actor_kind }}</td>
              <td><strong :class="item.amount >= 0 ? 'positive-text' : 'negative-text'">{{ item.amount > 0 ? "+" : "" }}{{ item.amount }}</strong></td>
              <td>{{ formatNumber(item.balance_after) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="pagination">
        <button :disabled="offset === 0 || loading" @click="page(-1)"><ChevronLeft :size="16" /> 上一页</button>
        <button :disabled="!canNext || loading" @click="page(1)">下一页 <ChevronRight :size="16" /></button>
      </div>
    </section>
  </div>
</template>
