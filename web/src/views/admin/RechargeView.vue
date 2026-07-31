<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Check, CircleDollarSign, CirclePlus, Coins, Package, Pencil, Search, X, XCircle } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import FilterBar from "@/components/admin/FilterBar.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { RechargeOrder, RechargeProduct } from "@/types";

const sessionStore = useSessionStore();
const products = ref<RechargeProduct[]>([]);
const orders = ref<RechargeOrder[]>([]);
const loading = ref(true);
const search = ref("");
const status = ref("");
const modalOpen = ref(false);
const editing = ref<RechargeProduct | null>(null);
const selectedOrder = ref<RechargeOrder | null>(null);
const decision = ref<"approve" | "reject" | null>(null);
const busy = ref(false);
const error = ref("");
const decisionForm = reactive({ payment_reference: "", admin_note: "" });
const form = reactive({ name: "", description: "", amount_yuan: 0, coins: 0, bonus_coins: 0, enabled: true, sort_order: 0 });
const canUpdate = computed(() => sessionStore.session?.permissions.some((item) => item === "*" || item === "billing:*" || item === "billing:update"));

function orderTone(value: RechargeOrder["status"]): "warning" | "success" | "muted" {
  return value === "pending" ? "warning" : value === "credited" ? "success" : "muted";
}
function orderLabel(value: RechargeOrder["status"]) {
  return value === "pending" ? "待确认" : value === "credited" ? "已入账" : "已取消";
}
function money(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}
async function load() {
  loading.value = true;
  const params = new URLSearchParams({ limit: "100", offset: "0" });
  if (search.value) params.set("search", search.value);
  if (status.value) params.set("status", status.value);
  try {
    const [productResult, orderResult] = await Promise.all([
      api<{ items: RechargeProduct[] }>("/admin/recharge/products"),
      api<{ items: RechargeOrder[] }>(`/admin/recharge/orders?${params}`),
    ]);
    products.value = productResult.items;
    orders.value = orderResult.items;
  } finally {
    loading.value = false;
  }
}
function editProduct(product?: RechargeProduct) {
  editing.value = product || null;
  Object.assign(form, {
    name: product?.name || "",
    description: product?.description || "",
    amount_yuan: (product?.amount_cents || 0) / 100,
    coins: product?.coins || 0,
    bonus_coins: product?.bonus_coins || 0,
    enabled: product?.enabled ?? true,
    sort_order: product?.sort_order || products.value.length * 10,
  });
  error.value = "";
  modalOpen.value = true;
}
async function saveProduct() {
  error.value = "";
  const payload = {
    name: form.name,
    description: form.description || null,
    amount_cents: Math.round(form.amount_yuan * 100),
    coins: form.coins,
    bonus_coins: form.bonus_coins,
    enabled: form.enabled,
    sort_order: form.sort_order,
    revision: editing.value?.revision,
  };
  try {
    if (editing.value) await api(`/admin/recharge/products/${editing.value.id}`, { method: "PATCH", body: JSON.stringify(payload) });
    else {
      const { revision: _revision, ...createPayload } = payload;
      await api("/admin/recharge/products", { method: "POST", body: JSON.stringify(createPayload) });
    }
    modalOpen.value = false;
    await load();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "商品保存失败";
  }
}
function openDecision(order: RechargeOrder, action: "approve" | "reject") {
  selectedOrder.value = order;
  decision.value = action;
  decisionForm.payment_reference = "";
  decisionForm.admin_note = "";
}
async function decide() {
  if (!selectedOrder.value || !decision.value) return;
  busy.value = true;
  try {
    await api(`/admin/recharge/orders/${selectedOrder.value.id}/decision`, {
      method: "POST",
      body: JSON.stringify({
        approve: decision.value === "approve",
        payment_reference: decisionForm.payment_reference || null,
        admin_note: decisionForm.admin_note || null,
      }),
    });
    decision.value = null;
    selectedOrder.value = null;
    await load();
  } finally {
    busy.value = false;
  }
}
onMounted(load);
useRealtimeEvents(
  ["billing.order.created", "billing.order.updated", "billing.product.created", "billing.product.updated"],
  () => load(),
  true,
);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="RECHARGE OPERATIONS" title="充值中心" description="维护积分商品，审核用户提交的人工充值订单，确认后自动入账并生成完整流水。" :icon="CircleDollarSign">
      <template #actions><button v-if="canUpdate" class="primary-button" @click="editProduct()"><CirclePlus :size="17" /> 新增商品</button></template>
    </AdminPageHeader>

    <section class="commerce-product-grid">
      <article v-for="product in products" :key="product.id" class="panel commerce-product-card" :class="{ disabled: !product.enabled }">
        <header><span><Package :size="20" /></span><StatusBadge :label="product.enabled ? '销售中' : '已停用'" :tone="product.enabled ? 'success' : 'muted'" /></header>
        <h2>{{ product.name }}</h2><p>{{ product.description || "暂无商品说明" }}</p>
        <strong>{{ money(product.amount_cents) }}</strong>
        <div><span><Coins :size="15" /> {{ formatNumber(product.coins) }} 积分</span><em v-if="product.bonus_coins">赠 {{ formatNumber(product.bonus_coins) }}</em></div>
        <button v-if="canUpdate" class="text-button" @click="editProduct(product)"><Pencil :size="14" /> 编辑商品</button>
      </article>
    </section>

    <section class="panel table-panel">
      <div class="panel-heading commerce-heading"><div><span class="section-kicker">MANUAL ORDERS</span><h2>充值订单</h2></div></div>
      <FilterBar>
        <label class="search-box"><Search :size="18" /><input v-model.trim="search" placeholder="订单号、Telegram ID 或支付备注" @keyup.enter="load" /></label>
        <label class="select-box"><select v-model="status" @change="load"><option value="">全部状态</option><option value="pending">待确认</option><option value="credited">已入账</option><option value="canceled">已取消</option></select></label>
      </FilterBar>
      <AdminDataTable :loading="loading" :empty="!orders.length" empty-title="暂无充值订单" min-width="980px">
        <template #head><tr><th>订单</th><th>用户</th><th>商品</th><th>金额</th><th>到账积分</th><th>创建时间</th><th>状态</th><th /></tr></template>
        <template #body>
          <tr v-for="order in orders" :key="order.id">
            <td><div class="table-primary"><strong>{{ order.order_no }}</strong><small>{{ order.payment_reference || "人工确认" }}</small></div></td>
            <td>TG · {{ order.tg }}</td><td>{{ order.product_name }}</td><td class="strong-cell">{{ money(order.amount_cents) }}</td>
            <td>{{ formatNumber(order.coins + order.bonus_coins) }}</td><td>{{ formatDate(order.created_at) }}</td>
            <td><StatusBadge :label="orderLabel(order.status)" :tone="orderTone(order.status)" /></td>
            <td><div v-if="canUpdate && order.status === 'pending'" class="row-actions"><button class="success-action" title="确认入账" @click="openDecision(order, 'approve')"><Check :size="16" /></button><button class="danger-action" title="拒绝" @click="openDecision(order, 'reject')"><XCircle :size="16" /></button></div></td>
          </tr>
        </template>
      </AdminDataTable>
    </section>

    <div v-if="modalOpen" class="modal-layer"><form class="modal-card" @submit.prevent="saveProduct">
      <header><div><span class="section-kicker">PRODUCT</span><h2>{{ editing ? "编辑充值商品" : "新增充值商品" }}</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header>
      <label><span>商品名称</span><input v-model.trim="form.name" required minlength="2" /></label>
      <label><span>商品说明</span><textarea v-model.trim="form.description" maxlength="500" /></label>
      <div class="form-grid compact-form-grid"><label><span>售价（元）</span><input v-model.number="form.amount_yuan" type="number" min="0.01" step="0.01" required /></label><label><span>基础积分</span><input v-model.number="form.coins" type="number" min="1" required /></label><label><span>赠送积分</span><input v-model.number="form.bonus_coins" type="number" min="0" /></label><label><span>显示顺序</span><input v-model.number="form.sort_order" type="number" min="0" /></label></div>
      <label class="check-row"><input v-model="form.enabled" type="checkbox" /><span>启用该商品</span></label>
      <p v-if="error" class="form-error">{{ error }}</p><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button">保存</button></footer>
    </form></div>

    <ConfirmDialog :open="Boolean(decision)" :title="decision === 'approve' ? '确认订单并入账？' : '拒绝这笔订单？'" :description="decision === 'approve' ? '确认后积分会立即进入用户余额，不能重复入账。' : '订单会被标记为已取消，不会产生积分。'" :confirm-label="decision === 'approve' ? '确认入账' : '拒绝订单'" :tone="decision === 'approve' ? 'normal' : 'danger'" :busy="busy" @close="decision = null" @confirm="decide">
      <div class="decision-fields"><strong>{{ selectedOrder?.order_no }} · {{ selectedOrder && money(selectedOrder.amount_cents) }}</strong><input v-if="decision === 'approve'" v-model.trim="decisionForm.payment_reference" placeholder="支付参考号（可选）" /><textarea v-model.trim="decisionForm.admin_note" placeholder="管理员备注（可选）" /></div>
    </ConfirmDialog>
  </div>
</template>
