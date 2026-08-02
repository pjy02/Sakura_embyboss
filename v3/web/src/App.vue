<script setup lang="ts">
import { onMounted } from 'vue'
import { useSessionStore } from './stores/session'
import AuthView from './views/AuthView.vue'
import AppShell from './components/AppShell.vue'

const session = useSessionStore()
onMounted(() => session.restore())
</script>

<template>
  <main v-if="!session.ready" class="splash"><div class="brand-orb">S</div><p>正在连接 Sakura…</p></main>
  <AuthView v-else-if="!session.account" />
  <AppShell v-else />
</template>
