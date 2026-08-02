<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Download, Filter, Play, Plus, Search, X } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import DataTable from '../components/DataTable.vue'
import ActionDialog from '../components/ActionDialog.vue'
import { findNavigation } from '../navigation'
import { actions, datasets, type PageAction } from '../resource-config'
import { api, readable, type ItemList } from '../lib/api'
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
const dialogSeed = ref<Record<string, unknown>>({})
const selected = ref<Record<string, unknown> | null>(null)
const filtered = computed(() => !search.value ? records.value : records.value.filter((item) => JSON.stringify(item).toLowerCase().includes(search.value.toLowerCase())))
const selectedId = computed(() => selected.value?.id == null ? '' : String(selected.value.id))
const selectedEntries = computed(() => Object.entries(selected.value || {}).filter(([, value]) => value !== null && value !== undefined).slice(0, 14))
const contextActions = computed(() => pageActions.value.filter((action) => action.fields.some((field) => field.path)))
async function load() {
  const dataset = pageDatasets.value[active.value]
  if (!dataset) return
  busy.value = true; error.value = ''
  try {
    const result = await api.call<ItemList | Record<string, unknown>[]>(dataset.operation, { query: { limit: 100, ...dataset.query } })
    records.value = Array.isArray(result) ? result : result.items || [result]
    if (selectedId.value) selected.value = records.value.find((item) => String(item.id) === selectedId.value) || null
  } catch (e) { records.value = []; error.value = e instanceof Error ? e.message : '加载失败' }
  finally { busy.value = false }
}
function openAction(action: PageAction, item?: Record<string, unknown>) {
  dialogSeed.value = {}
  if (item) {
    for (const field of action.fields) {
      let value = item[field.key]
      if (value === undefined && field.key === 'id') value = item.id
      if (value === undefined && field.key.endsWith('_id')) value = item.id
      if (value !== undefined) dialogSeed.value[field.key] = value
    }
  }
  dialog.value = action
}
function exportData() { const blob = new Blob([JSON.stringify(records.value, null, 2)], { type: 'application/json' }); const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = `${route.path.split('/').pop() || 'sakura'}.json`; link.click(); URL.revokeObjectURL(url) }
watch(() => route.path, () => { active.value = 0; selected.value = null; load() })
watch(active, () => { selected.value = null; load() })
onMounted(load)
</script>

<template><div><PageHeader :eyebrow="nav.eyebrow" :title="nav.label" :description="nav.description" :busy="busy" @refresh="load"><button v-for="action in pageActions" :key="action.operation" class="primary-button" @click="openAction(action)"><Plus :size="16" />{{ action.label }}</button></PageHeader><div class="resource-toolbar"><div class="tabs"><button v-for="(dataset, index) in pageDatasets" :key="dataset.operation" :class="{ active: active === index }" @click="active = index">{{ dataset.label }}</button></div><div class="table-tools"><label><Search :size="15" /><input v-model="search" placeholder="筛选当前列表" /></label><button class="icon-button" title="筛选"><Filter :size="17" /></button><button class="icon-button" title="导出 JSON" @click="exportData"><Download :size="17" /></button></div></div><p v-if="error" class="inline-error">{{ error }}</p><DataTable :items="filtered" :loading="busy" :selected-id="selectedId" @select="selected = $event" /><p class="data-foot">显示 {{ filtered.length }} 条记录 · 点击记录可查看详情并自动带入操作参数</p><section v-if="selected" class="record-inspector"><header><div><p class="eyebrow">SELECTED RECORD</p><h2>记录详情</h2></div><button class="icon-button" type="button" title="关闭详情" @click="selected = null"><X :size="17" /></button></header><dl><div v-for="entry in selectedEntries" :key="entry[0]"><dt>{{ entry[0] }}</dt><dd :title="readable(entry[1])">{{ readable(entry[1]) }}</dd></div></dl><footer v-if="contextActions.length"><span>对当前记录执行</span><button v-for="action in contextActions" :key="action.operation" class="secondary-button" type="button" @click="openAction(action, selected)"><Play :size="14" />{{ action.label }}</button></footer></section><ActionDialog :action="dialog" :seed="dialogSeed" @close="dialog = null" @completed="load" /></div></template>
