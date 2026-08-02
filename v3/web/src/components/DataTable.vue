<script setup lang="ts">
import { computed } from 'vue'
import { Database, MoreHorizontal } from 'lucide-vue-next'
import { readable } from '../lib/api'

const props = defineProps<{ items: Record<string, unknown>[]; loading?: boolean }>()
const hidden = new Set(['metadata', 'payload', 'before_state', 'after_state', 'content', 'description'])
const preferred = ['display_name', 'username', 'name', 'title', 'subject', 'code', 'type', 'status', 'state', 'amount', 'balance', 'created_at', 'updated_at']
const keys = computed(() => {
  const all = [...new Set(props.items.flatMap(Object.keys))].filter((key) => !hidden.has(key))
  return [...preferred.filter((key) => all.includes(key)), ...all.filter((key) => !preferred.includes(key))].slice(0, 6)
})
const labels: Record<string, string> = { display_name: '名称', username: '用户名', name: '名称', title: '标题', subject: '主题', code: '编号', type: '类型', status: '状态', state: '状态', amount: '数量', balance: '余额', created_at: '创建时间', updated_at: '更新时间', id: 'ID', email: '邮箱', client_name: '客户端', device_name: '设备', severity: '级别' }
</script>
<template>
  <div class="table-card">
    <div v-if="loading" class="table-loading"><span v-for="i in 5" :key="i" /></div>
    <div v-else-if="!items.length" class="empty-state"><Database /><strong>暂无数据</strong><span>这里会展示服务端返回的最新记录</span></div>
    <div v-else class="table-scroll"><table><thead><tr><th v-for="key in keys" :key="key">{{ labels[key] || key }}</th><th /></tr></thead><tbody><tr v-for="(item, index) in items" :key="String(item.id || index)"><td v-for="key in keys" :key="key"><span v-if="key === 'status' || key === 'state'" class="status-pill" :data-status="String(item[key])">{{ readable(item[key]) }}</span><span v-else :title="readable(item[key])">{{ readable(item[key]) }}</span></td><td><button class="row-menu"><MoreHorizontal :size="17" /></button></td></tr></tbody></table></div>
  </div>
</template>
