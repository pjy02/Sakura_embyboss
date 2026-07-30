<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { KeyRound, LogOut, MonitorSmartphone, ShieldCheck, TriangleAlert } from "lucide-vue-next";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { UserProfile } from "@/types";

const sessionStore = useSessionStore();
const router = useRouter();
const profile = ref<UserProfile | null>(null);
const busy = ref<"current" | "all" | null>(null);

onMounted(async () => {
  profile.value = await api<UserProfile>("/me");
});

async function logout(all: boolean) {
  busy.value = all ? "all" : "current";
  try {
    await sessionStore.logout(all);
    await router.replace("/login");
  } finally {
    busy.value = null;
  }
}
</script>

<template>
  <div class="page-stack">
    <header class="page-heading">
      <div>
        <span class="eyebrow">ACCOUNT SECURITY</span>
        <h1>账户与安全</h1>
        <p>查看当前身份和会话状态，发现异常时可立即让所有设备退出。</p>
      </div>
    </header>

    <section class="security-grid">
      <article class="panel security-card">
        <span class="security-icon good"><ShieldCheck :size="23" /></span>
        <div>
          <span class="section-kicker">IDENTITY</span>
          <h2>身份验证正常</h2>
          <p>当前会话已通过服务端签名校验，并启用了跨站请求保护。</p>
        </div>
        <dl class="detail-list">
          <div><dt>Telegram ID</dt><dd>{{ profile?.tg || sessionStore.session?.tg }}</dd></div>
          <div><dt>Emby 账户</dt><dd>{{ profile?.name || "未绑定" }}</dd></div>
          <div><dt>登录方式</dt><dd>{{ sessionStore.session?.auth_method === "telegram" ? "Telegram" : "Emby 密码" }}</dd></div>
          <div><dt>账户创建</dt><dd>{{ formatDate(profile?.created_at) }}</dd></div>
        </dl>
      </article>

      <article class="panel security-card">
        <span class="security-icon"><MonitorSmartphone :size="23" /></span>
        <div>
          <span class="section-kicker">CURRENT SESSION</span>
          <h2>当前浏览器</h2>
          <p>会话由 HttpOnly Cookie 保存，网页脚本无法读取登录凭据。</p>
        </div>
        <button class="secondary-button wide" :disabled="Boolean(busy)" @click="logout(false)">
          <LogOut :size="17" /> {{ busy === "current" ? "正在退出…" : "退出当前浏览器" }}
        </button>
      </article>
    </section>

    <section class="danger-zone">
      <div class="danger-copy">
        <span><TriangleAlert :size="20" /></span>
        <div>
          <h2>发现陌生登录？</h2>
          <p>退出全部会话会立即撤销当前账号的所有网页登录，需要重新验证身份。</p>
        </div>
      </div>
      <button class="danger-button" :disabled="Boolean(busy)" @click="logout(true)">
        <KeyRound :size="17" /> {{ busy === "all" ? "正在撤销…" : "退出所有设备" }}
      </button>
    </section>
  </div>
</template>
