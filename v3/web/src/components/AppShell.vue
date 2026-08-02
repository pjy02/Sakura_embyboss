<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Bell, ChevronDown, Command, LogOut, Menu, Search, Sparkles, X } from 'lucide-vue-next'
import { navigation } from '../navigation'
import { useSessionStore } from '../stores/session'
import RealtimeTray from './RealtimeTray.vue'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const mobileOpen = ref(false)
const trayOpen = ref(false)
const searchOpen = ref(false)
const searchQuery = ref('')
const userItems = computed(() => navigation.filter((item) => item.area === 'user'))
const adminItems = computed(() => navigation.filter((item) => item.area === 'admin' && session.has(item.permission)))
const searchResults = computed(() => [...userItems.value, ...adminItems.value].filter((item) => `${item.label}${item.description}`.toLowerCase().includes(searchQuery.value.toLowerCase())))
async function logout() { await session.logout(); await router.push('/') }
function shortcut(event: KeyboardEvent) { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); searchOpen.value = !searchOpen.value } }
onMounted(() => window.addEventListener('keydown', shortcut))
onBeforeUnmount(() => window.removeEventListener('keydown', shortcut))
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ open: mobileOpen }">
      <div class="side-brand"><span class="brand-orb"><Sparkles :size="19" /></span><span><strong>Sakura</strong><small>MEDIA OS</small></span><button class="mobile-close" @click="mobileOpen = false"><X /></button></div>
      <nav>
        <p class="nav-heading">用户中心</p>
        <RouterLink v-for="item in userItems" :key="item.path" :to="item.path" @click="mobileOpen = false"><span>{{ item.label }}</span></RouterLink>
        <template v-if="adminItems.length">
          <p class="nav-heading">管理后台</p>
          <RouterLink v-for="item in adminItems" :key="item.path" :to="item.path" @click="mobileOpen = false"><span>{{ item.label }}</span></RouterLink>
        </template>
      </nav>
      <div class="side-account"><div class="avatar">{{ session.account?.display_name?.slice(0, 1) || 'S' }}</div><span><strong>{{ session.account?.display_name }}</strong><small>@{{ session.username }}</small></span><button title="退出" @click="logout"><LogOut :size="17" /></button></div>
    </aside>
    <div v-if="mobileOpen" class="scrim" @click="mobileOpen = false" />
    <section class="app-main">
      <header class="topbar">
        <button class="mobile-menu" @click="mobileOpen = true"><Menu /></button>
        <button class="command-search" @click="searchOpen = true"><Search :size="17" /><span>搜索功能或管理模块</span><kbd><Command :size="12" /> K</kbd></button>
        <div class="top-actions"><button class="icon-button" title="实时任务" @click="trayOpen = !trayOpen"><Bell :size="19" /><i /></button><button class="profile-button"><span class="avatar small">{{ session.account?.display_name?.slice(0, 1) }}</span><ChevronDown :size="15" /></button></div>
      </header>
      <main class="page-wrap" :key="route.fullPath"><RouterView /></main>
    </section>
    <RealtimeTray :open="trayOpen" @close="trayOpen = false" />
    <Transition name="modal"><div v-if="searchOpen" class="modal-layer command-layer" @mousedown.self="searchOpen = false"><section class="command-panel"><label><Search /><input v-model="searchQuery" autofocus placeholder="输入影片、账号、风险、设置…" /><button @click="searchOpen = false"><X /></button></label><div><RouterLink v-for="item in searchResults" :key="item.path" :to="item.path" @click="searchOpen = false"><span><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span><kbd>↵</kbd></RouterLink></div></section></div></Transition>
  </div>
</template>
