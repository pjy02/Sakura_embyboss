<script setup lang="ts">
import { ref } from 'vue'
import { ArrowRight, CheckCircle2, Clapperboard, ShieldCheck, Sparkles } from 'lucide-vue-next'
import { useSessionStore } from '../stores/session'

const session = useSessionStore()
const mode = ref<'login' | 'register'>('login')
const username = ref('')
const displayName = ref('')
const password = ref('')
async function submit() {
  if (mode.value === 'login') await session.login(username.value, password.value)
  else await session.register(username.value, displayName.value, password.value)
}
</script>

<template>
  <main class="auth-page">
    <section class="auth-story">
      <div class="wordmark"><span class="brand-orb"><Sparkles :size="22" /></span><strong>Sakura</strong><span>MEDIA OS</span></div>
      <div class="auth-copy">
        <p class="eyebrow">YOUR MEDIA, BEAUTIFULLY MANAGED</p>
        <h1>一个账号，连接<br><em>所有观影体验。</em></h1>
        <p>Web 与 Telegram Bot 共享同一套账号、权限和业务结果，任何入口都可以独立使用。</p>
        <div class="promise-grid">
          <span><Clapperboard />多 Emby 实例</span><span><ShieldCheck />设备与风险保护</span><span><CheckCircle2 />实时任务进度</span>
        </div>
      </div>
      <p class="auth-foot">Sakura v3 · API first architecture</p>
    </section>
    <section class="auth-panel">
      <form class="auth-card" @submit.prevent="submit">
        <p class="eyebrow">{{ mode === 'login' ? 'WELCOME BACK' : 'CREATE ACCOUNT' }}</p>
        <h2>{{ mode === 'login' ? '欢迎回来' : '创建你的 Sakura 账号' }}</h2>
        <p class="muted">{{ mode === 'login' ? '登录用户中心或管理后台。' : '无需 Telegram，即可独立完成注册。' }}</p>
        <label>用户名<input v-model.trim="username" autocomplete="username" required placeholder="yourname" /></label>
        <label v-if="mode === 'register'">显示名称<input v-model.trim="displayName" required placeholder="如何称呼你" /></label>
        <label>密码<input v-model="password" type="password" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" required minlength="8" placeholder="至少 8 位" /></label>
        <p v-if="session.error" class="form-error">{{ session.error }}</p>
        <button class="primary-button" :disabled="session.busy">{{ session.busy ? '请稍候…' : mode === 'login' ? '进入 Sakura' : '注册并进入' }}<ArrowRight :size="18" /></button>
        <button type="button" class="text-button" @click="mode = mode === 'login' ? 'register' : 'login'">{{ mode === 'login' ? '没有账号？立即注册' : '已有账号？返回登录' }}</button>
      </form>
    </section>
  </main>
</template>
