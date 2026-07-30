<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ExternalLink, KeyRound, LoaderCircle, MessageCircle, ShieldCheck } from "lucide-vue-next";
import QRCode from "qrcode";
import BrandMark from "@/components/BrandMark.vue";
import { api, ApiError } from "@/lib/api";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";
import type { TelegramLogin } from "@/types";

const route = useRoute();
const router = useRouter();
const sessionStore = useSessionStore();
const method = ref<"telegram" | "emby">("telegram");
const telegram = ref<TelegramLogin | null>(null);
const qrData = ref("");
const status = ref<"idle" | "creating" | "pending" | "approved" | "expired">("idle");
const error = ref(route.query.forbidden ? "此账户没有管理后台访问权限" : "");
const username = ref("");
const password = ref("");
const busy = ref(false);
let timer: number | undefined;

const isAdmin = runtime.area === "admin";
const loginTitle = computed(() => (isAdmin ? "登录管理控制台" : "欢迎回到 Sakura"));

async function startTelegram() {
  error.value = "";
  status.value = "creating";
  try {
    telegram.value = await api<TelegramLogin>("/auth/telegram/start", {
      method: "POST",
      body: "{}",
    });
    qrData.value = await QRCode.toDataURL(telegram.value.deep_link, {
      width: 248,
      margin: 1,
      color: { dark: "#17131d", light: "#fffafd" },
    });
    status.value = "pending";
    schedulePoll();
  } catch (e) {
    status.value = "idle";
    error.value = e instanceof Error ? e.message : "无法创建登录请求";
  }
}

function schedulePoll() {
  window.clearTimeout(timer);
  timer = window.setTimeout(pollTelegram, (telegram.value?.poll_after_seconds || 2) * 1000);
}

async function pollTelegram() {
  if (!telegram.value || status.value !== "pending") return;
  try {
    const result = await api<{ status: string; expires_at: string }>("/auth/telegram/status", {
      method: "POST",
      body: JSON.stringify({ token: telegram.value.request_token }),
    });
    if (result.status === "approved") {
      status.value = "approved";
      await api("/auth/telegram/exchange", {
        method: "POST",
        body: JSON.stringify({ token: telegram.value.request_token }),
      });
      await finishLogin();
      return;
    }
    if (["expired", "rejected", "consumed"].includes(result.status)) {
      status.value = "expired";
      return;
    }
    schedulePoll();
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) status.value = "expired";
    else schedulePoll();
  }
}

async function embyLogin() {
  busy.value = true;
  error.value = "";
  try {
    await api("/auth/emby", {
      method: "POST",
      body: JSON.stringify({ username: username.value, password: password.value }),
    });
    await finishLogin();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "登录失败";
  } finally {
    busy.value = false;
  }
}

async function finishLogin() {
  await sessionStore.load();
  const target = typeof route.query.next === "string" ? route.query.next : "/";
  await router.replace(target);
}

onBeforeUnmount(() => window.clearTimeout(timer));
</script>

<template>
  <main class="login-page">
    <div class="login-aurora one" />
    <div class="login-aurora two" />
    <section class="login-story">
      <BrandMark />
      <div class="story-copy">
        <span class="eyebrow">{{ isAdmin ? "SAKURA OPERATIONS" : "YOUR MEDIA GARDEN" }}</span>
        <h1>{{ isAdmin ? "让每一次管理，都清晰而从容。" : "你的影音世界，安静盛放。" }}</h1>
        <p>
          {{
            isAdmin
              ? "统一管理成员、积分、角色与审计记录，所有敏感操作均经过权限和安全校验。"
              : "在这里查看账户状态、积分流水与安全信息。Bot 与 Web 数据始终保持一致。"
          }}
        </p>
      </div>
      <div class="trust-row">
        <ShieldCheck :size="18" />
        <span>安全会话</span>
        <i />
        <span>实时互通</span>
        <i />
        <span>隐私保护</span>
      </div>
    </section>

    <section class="login-panel">
      <div class="login-card">
        <div class="login-heading">
          <span class="petal-chip">桜</span>
          <div>
            <h2>{{ loginTitle }}</h2>
            <p>{{ isAdmin ? "仅允许已授权的 Telegram 管理员登录" : "选择一种方式验证你的身份" }}</p>
          </div>
        </div>

        <div v-if="!isAdmin" class="method-switch">
          <button :class="{ active: method === 'telegram' }" @click="method = 'telegram'">
            <MessageCircle :size="17" /> Telegram
          </button>
          <button :class="{ active: method === 'emby' }" @click="method = 'emby'">
            <KeyRound :size="17" /> Emby 账户
          </button>
        </div>

        <div v-if="method === 'telegram' || isAdmin" class="telegram-login">
          <button v-if="status === 'idle'" class="primary-button wide" @click="startTelegram">
            <MessageCircle :size="18" /> 通过 Telegram 登录
          </button>
          <div v-else-if="status === 'creating'" class="login-loading">
            <LoaderCircle class="spin" :size="26" />
            <span>正在创建安全登录请求…</span>
          </div>
          <div v-else-if="telegram && status === 'pending'" class="qr-flow">
            <div class="qr-frame"><img :src="qrData" alt="Telegram 登录二维码" /></div>
            <div class="qr-copy">
              <strong>打开 Telegram 完成确认</strong>
              <p>扫码或点击下方按钮，在 Bot 对话中批准本次登录。</p>
              <a :href="telegram.deep_link" target="_blank" rel="noreferrer">
                前往 Telegram <ExternalLink :size="15" />
              </a>
              <span><i class="pulse-dot" /> 等待确认中</span>
            </div>
          </div>
          <div v-else-if="status === 'approved'" class="login-loading">
            <LoaderCircle class="spin" :size="26" />
            <span>验证成功，正在进入…</span>
          </div>
          <div v-else class="expired-box">
            <p>登录请求已失效，请重新创建。</p>
            <button class="secondary-button" @click="startTelegram">重新尝试</button>
          </div>
        </div>

        <form v-else class="emby-form" @submit.prevent="embyLogin">
          <label>
            <span>Emby 用户名</span>
            <input v-model.trim="username" autocomplete="username" required placeholder="请输入用户名" />
          </label>
          <label>
            <span>密码</span>
            <input
              v-model="password"
              type="password"
              autocomplete="current-password"
              required
              placeholder="请输入密码"
            />
          </label>
          <button class="primary-button wide" :disabled="busy">
            <LoaderCircle v-if="busy" class="spin" :size="18" />
            <KeyRound v-else :size="18" />
            {{ busy ? "正在验证…" : "登录用户中心" }}
          </button>
        </form>

        <p v-if="error" class="form-error">{{ error }}</p>
        <p class="login-footnote">登录即代表你授权当前浏览器创建安全会话。我们不会在网页中保存你的密码。</p>
      </div>
    </section>
  </main>
</template>
