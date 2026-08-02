<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Download, Filter, Plus, Search } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import DataTable from '../components/DataTable.vue'
import ActionDialog from '../components/ActionDialog.vue'
import { findNavigation } from '../navigation'
import { actions, datasets, type PageAction } from '../resource-config'
import { api, type ItemList } from '../lib/api'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const session = useSessionStore()
const nav = computed(() => findNavigation(route.path)!)
const pageDatasets = computed(() => datasets[route.path] || [])
const pageActions = computed(() => (actions[route.path] || []).filter((action) => session.has(action.permission)))
const active = ref(0)
const records = ref<Record<string, unknown>[]>([])
const busy = ref(false)
const error = ref('')
const search = ref('')
const dialog = ref<PageAction | null>(null)
const filtered = computed(() => !search.value ? records.value : records.value.filter((item) => JSON.stringify(item).toLowerCase().includes(search.value.toLowerCase())))
async function load() {
  const dataset = pageDatasets.value[active.value]
  if (!dataset) return
  busy.value = true; error.value = ''
  try {
    const result = await api.call<ItemList | Record<string, unknown>[]>(dataset.operation, { query: { limit: 100, ...dataset.query } })
    records.value = Array.isArray(result) ? result : result.items || [result]
  } catch (e) { records.value = []; error.value = e instanceof Error ? e.message : '加载失败' }
  finally { busy.value = false }
}
function exportData() { const blob = new Blob([JSON.stringify(records.value, null, 2)], { type: 'application/json' }); const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = `${route.path.split('/').pop() || 'sakura'}.json`; link.click(); URL.revokeObjectURL(url) }
watch(() => route.path, () => { active.value = 0; load() })
watch(active, load)
onMounted(load)
</script>

<template><div><PageHeader :eyebrow="nav.eyebrow" :title="nav.label" :description="nav.description" :busy="busy" @refresh="load"><button v-for="action in pageActions" :key="action.operation" class="primary-button" @click="dialog = action"><Plus :size="16" />{{ action.label }}</button></PageHeader><div class="resource-toolbar"><div class="tabs"><button v-for="(dataset, index) in pageDatasets" :key="dataset.operation" :class="{ active: active === index }" @click="active = index">{{ dataset.label }}</button></div><div class="table-tools"><label><Search :size="15" /><input v-model="search" placeholder="筛选当前列表" /></label><button class="icon-button" title="筛选"><Filter :size="17" /></button><button class="icon-button" title="导出 JSON" @click="exportData"><Download :size="17" /></button></div></div><p v-if="error" class="inline-error">{{ error }}</p><DataTable :items="filtered" :loading="busy" /><p class="data-foot">显示 {{ filtered.length }} 条记录 · 数据来自 Sakura v3 API</p><ActionDialog :action="dialog" @close="dialog = null" @completed="load" /></div></template>
