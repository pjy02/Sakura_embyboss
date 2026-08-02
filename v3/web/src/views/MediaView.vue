<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Film, Search, Send, Sparkles } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import DataTable from '../components/DataTable.vue'
import { api, idempotencyKey, type ItemList } from '../lib/api'

const query = ref('')
const busy = ref(false)
const results = ref<Record<string, unknown>[]>([])
const requests = ref<Record<string, unknown>[]>([])
const message = ref('')
async function loadRequests() { requests.value = (await api.call<ItemList>('listMyMediaRequests', { query: { limit: 30 } })).items }
async function search() { if (!query.value.trim()) return; busy.value = true; message.value = ''; try { const data = await api.call<ItemList>('searchTMDBMedia', { query: { q: query.value, page: 1 } }); results.value = data.items || (data as unknown as Record<string, unknown>[]); } catch (e) { message.value = e instanceof Error ? e.message : '搜索失败' } finally { busy.value = false } }
async function requestMedia(item: Record<string, unknown>) { busy.value = true; try { await api.call('createMediaRequest', { body: { media_id: item.id, note: '', idempotency_key: idempotencyKey('media-request') } }); message.value = '求片已提交，进度会实时同步。'; await loadRequests() } catch (e) { message.value = e instanceof Error ? e.message : '提交失败' } finally { busy.value = false } }
onMounted(loadRequests)
</script>
<template><div><PageHeader eyebrow="TMDB DISCOVERY" title="影片与求片" description="输入片名即可自动匹配 TMDB；重复影片和重复下载由服务端识别。" :busy="busy" @refresh="loadRequests" /><section class="search-hero"><div><Sparkles /><h2>今天想看什么？</h2><p>支持电影、剧集和多语言标题搜索</p></div><form @submit.prevent="search"><Search /><input v-model="query" placeholder="搜索片名，例如：星际穿越" /><button class="primary-button">搜索</button></form></section><p v-if="message" class="toast-line">{{ message }}</p><section v-if="results.length" class="media-grid"><article v-for="item in results" :key="String(item.id)" class="media-card"><div class="poster"><img v-if="item.poster_url" :src="String(item.poster_url)" alt="" /><Film v-else /></div><div><small>{{ item.media_type || 'MEDIA' }} · {{ item.release_date || item.release_year || '待定' }}</small><h3>{{ item.title || item.name }}</h3><p>{{ item.overview || '暂无简介' }}</p><button class="secondary-button" @click="requestMedia(item)"><Send :size="15" />发起求片</button></div></article></section><section class="section-block"><div class="section-title"><div><p class="eyebrow">REQUEST TRACKING</p><h2>我的求片</h2></div></div><DataTable :items="requests" :loading="busy" /></section></div></template>
