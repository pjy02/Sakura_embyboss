<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, ArrowUpRight, CircleDollarSign, Film, Server, ShieldAlert, Sparkles, TicketCheck, Users } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import DataTable from '../components/DataTable.vue'
import { api, readable, type ItemList } from '../lib/api'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const session = useSessionStore()
const admin = computed(() => route.path === '/admin')
const busy = ref(false)
const dashboard = ref<Record<string, unknown>>({})
const liveItems = ref<Record<string, unknown>[]>([])
const error = ref('')
async function load() {
  busy.value = true; error.value = ''
  try {
    if (admin.value) {
      const [accounts, tasks, risks, orders] = await Promise.all([
        api.call<ItemList>('listAccounts', { query: { limit: 8 } }), api.call<ItemList>('listEmbyTasks', { query: { limit: 8 } }),
        api.call<ItemList>('listRiskEvents', { query: { limit: 8 } }), api.call<ItemList>('listRechargeOrders', { query: { limit: 8 } }),
      ])
      dashboard.value = { accounts: accounts.items.length, tasks: tasks.items.length, risks: risks.items.length, orders: orders.items.length }
      liveItems.value = [...tasks.items, ...risks.items].slice(0, 8)
    } else {
      const results = await Promise.allSettled([api.call('getMyMembership'), api.call('getMyWallet'), api.call<ItemList>('listMyEmbyBindings'), api.call<ItemList>('listMyMediaRequests', { query: { limit: 6 } })])
      dashboard.value = { membership: results[0].status === 'fulfilled' ? results[0].value : null, wallet: results[1].status === 'fulfilled' ? results[1].value : null, bindings: results[2].status === 'fulfilled' ? results[2].value.items : [], requests: results[3].status === 'fulfilled' ? results[3].value.items : [] }
      liveItems.value = (dashboard.value.requests as Record<string, unknown>[]) || []
    }
  } catch (e) { error.value = e instanceof Error ? e.message : '加载失败' }
  finally { busy.value = false }
}
onMounted(load)
const cards = computed(() => admin.value ? [
  { label: '活跃账号', value: dashboard.value.accounts || 0, icon: Users, note: '统一身份' },
  { label: '同步任务', value: dashboard.value.tasks || 0, icon: Server, note: '实时队列' },
  { label: '风险事件', value: dashboard.value.risks || 0, icon: ShieldAlert, note: '待关注' },
  { label: '近期订单', value: dashboard.value.orders || 0, icon: CircleDollarSign, note: '交易中心' },
] : [
  { label: '会员状态', value: readable((dashboard.value.membership as Record<string, unknown>)?.status || '未开通'), icon: Sparkles, note: readable((dashboard.value.membership as Record<string, unknown>)?.expires_at) },
  { label: '积分余额', value: readable((dashboard.value.wallet as Record<string, unknown>)?.balance || 0), icon: CircleDollarSign, note: 'POINTS' },
  { label: '已绑定站点', value: ((dashboard.value.bindings as unknown[]) || []).length, icon: Server, note: '多 Emby' },
  { label: '进行中求片', value: ((dashboard.value.requests as unknown[]) || []).length, icon: Film, note: '自动去重' },
])
</script>

<template>
  <div>
    <PageHeader :eyebrow="admin ? 'OPERATIONS OVERVIEW' : 'PERSONAL OVERVIEW'" :title="admin ? '运营仪表盘' : `晚上好，${session.account?.display_name}`" :description="admin ? '关键业务、异步任务和风险事件集中呈现。' : '你的会员、站点、求片和服务动态都在这里。'" :busy="busy" @refresh="load">
      <RouterLink :to="admin ? '/admin/accounts' : '/media'" class="primary-button">{{ admin ? '管理账号' : '发现影片' }}<ArrowUpRight :size="17" /></RouterLink>
    </PageHeader>
    <p v-if="error" class="inline-error">{{ error }}</p>
    <section class="metric-grid"><article v-for="card in cards" :key="card.label" class="metric-card"><div class="metric-icon"><component :is="card.icon" /></div><div><span>{{ card.label }}</span><strong>{{ card.value }}</strong><small>{{ card.note }}</small></div><Activity :size="17" class="metric-trend" /></article></section>
    <section class="content-grid dashboard-grid"><article class="feature-card"><div class="card-heading"><div><p class="eyebrow">{{ admin ? 'LIVE OPERATIONS' : 'CURRENT ACTIVITY' }}</p><h2>{{ admin ? '任务与风险动态' : '求片与服务进度' }}</h2></div><TicketCheck /></div><DataTable :items="liveItems" :loading="busy" /></article><aside class="spotlight-card"><p class="eyebrow">SAKURA V3</p><h2>每一个入口，<br>同一份业务结果。</h2><p>Web、Bot 和开放 API 都只调用共享业务服务。你可以独立关闭任意入口，而不影响其他入口运行。</p><div class="architecture-line"><span>WEB</span><i /><span>API</span><i /><span>BOT</span></div></aside></section>
  </div>
</template>
