<script setup lang="ts">
import { onMounted, ref } from "vue";
import { BookOpenText, Coins, Search } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import type { BillingEntry } from "@/types";

const items = ref<BillingEntry[]>([]);
const total = ref(0);
const tg = ref("");
const entryType = ref("");
const loading = ref(true);
function money(cents: number | null) { return cents === null ? "—" : `¥${(cents / 100).toFixed(2)}`; }
function label(type: string) {
  return ({ order_created: "创建订单", order_credited: "确认入账", order_refunded: "退款冲正", order_rejected: "拒绝订单", order_canceled: "用户取消" } as Record<string, string>)[type] || type;
}
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" });
  if (tg.value) params.set("tg", tg.value);
  if (entryType.value) params.set("entry_type", entryType.value);
  try {
    const result = await api<{ items: BillingEntry[]; total: number }>(`/admin/billing/ledger?${params}`);
    items.value = result.items; total.value = result.total;
  } finally { loading.value = false; }
}
onMounted(load);
useRealtimeEvents(["billing.order.created", "billing.order.updated"], () => load(), true);
</script>
<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="FINANCIAL LEDGER" title="账单记录" description="不可变的订单事件流水，用于核对创建、审核、入账和异常处理。" :icon="BookOpenText">
      <template #meta><span class="date-chip">{{ formatNumber(total) }} 条流水</span></template>
    </AdminPageHeader>
    <section class="panel table-panel">
      <FilterBar><label class="search-box"><Search :size="18" /><input v-model.trim="tg" inputmode="numeric" placeholder="筛选 Telegram ID" @keyup.enter="load" /></label><label class="select-box"><select v-model="entryType" @change="load"><option value="">全部事件</option><option value="order_created">创建订单</option><option value="order_credited">确认入账</option><option value="order_refunded">退款冲正</option><option value="order_rejected">拒绝订单</option><option value="order_canceled">用户取消</option></select></label></FilterBar>
      <AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无账单流水" min-width="900px">
        <template #head><tr><th>流水</th><th>用户</th><th>事件</th><th>金额</th><th>积分</th><th>说明</th><th>操作人</th><th>时间</th></tr></template>
        <template #body><tr v-for="item in items" :key="item.id"><td>#{{ item.id }}</td><td>TG · {{ item.tg }}</td><td><StatusBadge :label="label(item.entry_type)" :tone="item.entry_type === 'order_credited' ? 'success' : item.entry_type === 'order_rejected' || item.entry_type === 'order_refunded' ? 'danger' : 'info'" /></td><td class="strong-cell">{{ money(item.amount_cents) }}</td><td><span class="ledger-coins"><Coins :size="14" /> {{ formatNumber(item.coins) }}</span></td><td>{{ item.description }}</td><td>{{ item.actor_kind }} · {{ item.actor_id }}</td><td>{{ formatDate(item.created_at) }}</td></tr></template>
      </AdminDataTable>
    </section>
  </div>
</template>
