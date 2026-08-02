<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Activity, CheckCircle2, Clock3, Radio, X } from 'lucide-vue-next'
import { useSessionStore } from '../stores/session'
import { readable } from '../lib/api'

const props = defineProps<{ open: boolean }>()
defineEmits<{ close: [] }>()
const session = useSessionStore()
const state = ref<Record<string, unknown>>({})
const connected = ref(false)
let source: EventSource | null = null
function connect() {
  source?.close()
  source = new EventSource(session.isAdmin ? '/api/v3/admin/realtime' : '/api/v3/me/realtime', { withCredentials: true })
  source.addEventListener('snapshot', (event) => { state.value = JSON.parse((event as MessageEvent).data); connected.value = true })
  source.onerror = () => { connected.value = false }
}
watch(() => props.open, (open) => { if (open) connect(); else source?.close() }, { immediate: true })
onBeforeUnmount(() => source?.close())
const groups = computed(() => Object.entries(state.value).filter(([, value]) => Array.isArray(value)) as [string, Record<string, unknown>[]][])
const labels: Record<string, string> = { provisioning: '账号开通', media_requests: '求片进度', tickets: '工单', notifications: '通知', tasks: '同步任务', batch_operations: '批量任务', automation_executions: '自动化', risk_events: '风险事件' }
</script>

<template>
  <Transition name="tray"><aside v-if="open" class="realtime-tray">
    <header><div><p class="eyebrow"><Radio :size="13" /> LIVE UPDATES</p><h3>实时任务</h3></div><button class="icon-button" @click="$emit('close')"><X /></button></header>
    <div class="connection"><i :class="{ online: connected }" />{{ connected ? '已连接实时数据流' : '正在重新连接…' }}</div>
    <div v-if="!groups.length" class="empty-state compact"><CheckCircle2 /><strong>暂时没有进行中的任务</strong><span>新的进度会自动出现在这里</span></div>
    <section v-for="[key, items] in groups" :key="key" class="live-group">
      <h4>{{ labels[key] || key }}<span>{{ items.length }}</span></h4>
      <article v-for="(item, index) in items.slice(0, 8)" :key="String(item.id || index)"><Activity :size="16" /><div><strong>{{ readable(item.title || item.name || item.type || item.kind || item.subject || '进行中') }}</strong><small>{{ readable(item.status || item.state || item.updated_at) }}</small></div><Clock3 :size="14" /></article>
    </section>
  </aside></Transition>
</template>
