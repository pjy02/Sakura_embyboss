<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { CheckCircle2, CircleDollarSign, Clock3, Coins, Package, ReceiptText, ShieldCheck, X } from "lucide-vue-next";
import ConfirmDialog from "@/components/admin/ConfirmDialog.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api, idempotencyKey } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/format";
import type { BillingEntry, RechargeOrder, RechargeProduct } from "@/types";

const products = ref<RechargeProduct[]>([]);
const orders = ref<RechargeOrder[]>([]);
const ledger = ref<BillingEntry[]>([]);
const loading = ref(true);
const busy = ref(false);
const selectedProduct = ref<RechargeProduct | null>(null);
const cancelTarget = ref<RechargeOrder | null>(null);
const userNote = ref("");
const error = ref("");

const pendingCount = computed(() => orders.value.filter((item) => item.status === "pending").length);

function money(cents: number | null) {
  return cents === null ? "—" : `¥${(cents / 100).toFixed(2)}`;
}

function orderLabel(status: RechargeOrder["status"]) {
  return status === "pending" ? "等待确认" : status === "credited" ? "已入账" : status === "refunded" ? "已退款" : "已取消";
}

function orderTone(status: RechargeOrder["status"]): "warning" | "success" | "danger" | "muted" {
  return status === "pending" ? "warning" : status === "credited" ? "success" : status === "refunded" ? "danger" : "muted";
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [productResult, orderResult, ledgerResult] = await Promise.all([
      api<{ items: RechargeProduct[] }>("/me/recharge/products"),
      api<{ items: RechargeOrder[] }>("/me/recharge/orders?limit=100"),
      api<{ items: BillingEntry[] }>("/me/billing/ledger?limit=12"),
    ]);
    products.value = productResult.items;
    orders.value = orderResult.items;
    ledger.value = ledgerResult.items;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "充值信息加载失败";
  } finally {
    loading.value = false;
  }
}

function chooseProduct(product: RechargeProduct) {
  selectedProduct.value = product;
  userNote.value = "";
  error.value = "";
}

async function createOrder() {
  if (!selectedProduct.value) return;
  busy.value = true;
  error.value = "";
  try {
    await api("/me/recharge/orders", {
      method: "POST",
      body: JSON.stringify({
        product_id: selectedProduct.value.id,
        user_note: userNote.value || null,
      }),
      idempotencyKey: idempotencyKey("recharge"),
    });
    selectedProduct.value = null;
    await load();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "订单提交失败";
  } finally {
    busy.value = false;
  }
}

async function cancelOrder() {
  if (!cancelTarget.value) return;
  busy.value = true;
  try {
    await api(`/me/recharge/orders/${cancelTarget.value.id}/cancel`, { method: "POST" });
    cancelTarget.value = null;
    await load();
  } finally {
    busy.value = false;
  }
}

onMounted(load);
useRealtimeEvents(
  ["billing.order.created", "billing.order.updated"],
  () => load(),
);
</script>

<template>
  <div class="page-stack">
    <header class="page-heading">
      <div>
        <span class="eyebrow">RECHARGE CENTER</span>
        <h1>充值中心</h1>
        <p>选择积分商品并提交订单。当前采用人工核验，管理员确认收款后积分才会到账。</p>
      </div>
      <span class="date-chip"><Clock3 :size="16" /> {{ pendingCount }} 笔待确认</span>
    </header>

    <LoadingBlock v-if="loading" />
    <div v-else-if="error && !products.length" class="error-banner">{{ error }}</div>
    <template v-else>
      <section class="billing-notice panel">
        <span><ShieldCheck :size="22" /></span>
        <div>
          <strong>人工确认充值</strong>
          <p>网页只负责生成订单和记录状态，不会自动扣款。请按管理员公布的渠道完成支付，并保留转账凭证。</p>
        </div>
      </section>

      <section>
        <div class="section-heading">
          <div><span class="section-kicker">POINT PACKAGES</span><h2>选择积分商品</h2></div>
        </div>
        <div v-if="products.length" class="commerce-product-grid portal-product-grid">
          <article v-for="product in products" :key="product.id" class="panel commerce-product-card">
            <header><span><Package :size="20" /></span><em v-if="product.bonus_coins">额外赠送</em></header>
            <h2>{{ product.name }}</h2>
            <p>{{ product.description || "积分将在管理员确认后进入账户。" }}</p>
            <strong>{{ money(product.amount_cents) }}</strong>
            <div>
              <span><Coins :size="15" /> {{ formatNumber(product.coins + product.bonus_coins) }} 积分</span>
              <em v-if="product.bonus_coins">含赠送 {{ formatNumber(product.bonus_coins) }}</em>
            </div>
            <button class="primary-button wide" @click="chooseProduct(product)">
              <CircleDollarSign :size="16" /> 提交充值订单
            </button>
          </article>
        </div>
        <EmptyState v-else title="暂无可用商品" description="管理员还没有开放充值商品。" />
      </section>

      <section class="billing-history-grid">
        <article class="panel table-panel">
          <div class="panel-heading">
            <div><span class="section-kicker">MY ORDERS</span><h2>我的充值订单</h2></div>
          </div>
          <EmptyState v-if="!orders.length" title="还没有充值订单" />
          <div v-else class="portal-order-list">
            <article v-for="order in orders" :key="order.id">
              <span class="order-mark" :data-status="order.status">
                <CheckCircle2 v-if="order.status === 'credited'" :size="18" />
                <Clock3 v-else :size="18" />
              </span>
              <div>
                <strong>{{ order.product_name }}</strong>
                <small>{{ order.order_no }} · {{ formatDate(order.created_at) }}</small>
              </div>
              <p><strong>{{ money(order.amount_cents) }}</strong><small>{{ formatNumber(order.coins + order.bonus_coins) }} 积分</small></p>
              <StatusBadge :label="orderLabel(order.status)" :tone="orderTone(order.status)" />
              <button v-if="order.status === 'pending'" class="text-button danger-text" @click="cancelTarget = order">取消</button>
            </article>
          </div>
        </article>

        <article class="panel billing-ledger-card">
          <div class="panel-heading">
            <div><span class="section-kicker">BILLING EVENTS</span><h2>最近账单记录</h2></div>
            <ReceiptText :size="19" />
          </div>
          <div v-if="ledger.length" class="mini-ledger">
            <article v-for="entry in ledger" :key="entry.id">
              <span><ReceiptText :size="15" /></span>
              <div><strong>{{ entry.description }}</strong><small>{{ formatDate(entry.created_at) }}</small></div>
              <em>{{ entry.coins ? `${entry.coins > 0 ? "+" : ""}${formatNumber(entry.coins)}` : money(entry.amount_cents) }}</em>
            </article>
          </div>
          <EmptyState v-else title="暂无账单记录" />
        </article>
      </section>
    </template>

    <div v-if="selectedProduct" class="modal-layer">
      <form class="modal-card recharge-submit-card" @submit.prevent="createOrder">
        <header>
          <div><span class="section-kicker">MANUAL RECHARGE</span><h2>提交充值订单</h2></div>
          <button type="button" class="icon-button" @click="selectedProduct = null"><X :size="19" /></button>
        </header>
        <div class="recharge-order-summary">
          <span><Coins :size="20" /></span>
          <div><strong>{{ selectedProduct.name }}</strong><small>{{ formatNumber(selectedProduct.coins + selectedProduct.bonus_coins) }} 积分</small></div>
          <em>{{ money(selectedProduct.amount_cents) }}</em>
        </div>
        <label><span>付款备注（可选）</span><textarea v-model.trim="userNote" maxlength="500" placeholder="可填写转账昵称、时间或需要管理员留意的信息" /></label>
        <p class="manual-payment-hint">提交后订单状态为“等待确认”。请根据管理员提供的支付方式付款，核验完成后积分会自动入账。</p>
        <p v-if="error" class="form-error">{{ error }}</p>
        <footer>
          <button type="button" class="secondary-button" @click="selectedProduct = null">取消</button>
          <button class="primary-button" :disabled="busy">{{ busy ? "正在提交…" : "确认生成订单" }}</button>
        </footer>
      </form>
    </div>

    <ConfirmDialog
      :open="Boolean(cancelTarget)"
      title="取消充值订单？"
      description="仅待确认的订单可以取消；如果已经付款，请先联系管理员核对。"
      confirm-label="确认取消"
      tone="danger"
      :busy="busy"
      @close="cancelTarget = null"
      @confirm="cancelOrder"
    >
      {{ cancelTarget?.order_no }}
    </ConfirmDialog>
  </div>
</template>
