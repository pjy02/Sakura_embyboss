<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { KeyRound, LogOut, MonitorSmartphone, ShieldCheck, TriangleAlert } from "lucide-vue-next";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { UserProfile } from "@/types";

interface CanonicalAccount {
  account_id: string;
  display_name: string | null;
  identities: Array<{ provider: string; username: string | null }>;
  membership: { expires_at: string | null; plan: { name: string } } | null;
  tags: Array<{ id: number; name: string; color: string }>;
}

const sessionStore = useSessionStore();
const router = useRouter();
const profile = ref<UserProfile | null>(null);
const account = ref<CanonicalAccount | null>(null);
const localUsername = ref("");
const localPassword = ref("");
const confirmLocalPassword = ref("");
const identityNotice = ref("");
const identityError = ref("");
const busy = ref<"current" | "all" | null>(null);

onMounted(async () => {
  [profile.value, account.value] = await Promise.all([
    api<UserProfile>("/me"),
    api<CanonicalAccount>("/me/account"),
  ]);
  localUsername.value = account.value.identities.find((item) => item.provider === "local")?.username || "";
});

async function saveLocalIdentity() {
  identityError.value = "";
  identityNotice.value = "";
  if (localPassword.value !== confirmLocalPassword.value) {
    identityError.value = "两次输入的密码不一致";
    return;
  }
  try {
    await api("/auth/local/identity", { method: "PUT", body: JSON.stringify({ username: localUsername.value, password: localPassword.value }) });
    identityNotice.value = "Web 登录身份已保存，之后可以不通过 Telegram 登录";
    localPassword.value = "";
    confirmLocalPassword.value = "";
    account.value = await api<CanonicalAccount>("/me/account");
  } catch (reason) { identityError.value = reason instanceof Error ? reason.message : "保存失败"; }
}

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
          <div><dt>统一账号 ID</dt><dd>{{ account?.account_id.slice(0, 8) }}</dd></div>
          <div><dt>Telegram ID</dt><dd>{{ profile?.tg && profile.tg > 0 ? profile.tg : "未绑定" }}</dd></div>
          <div><dt>Emby 账户</dt><dd>{{ profile?.name || "未绑定" }}</dd></div>
          <div><dt>登录方式</dt><dd>{{ sessionStore.session?.auth_method === "telegram" ? "Telegram" : sessionStore.session?.auth_method === "local" ? "Web 账号" : "Emby 密码" }}</dd></div>
          <div><dt>会员方案</dt><dd>{{ account?.membership?.plan.name || "未分配" }}</dd></div>
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

    <form class="panel local-identity-card" @submit.prevent="saveLocalIdentity">
      <div><span class="section-kicker">LOCAL IDENTITY</span><h2>Web 登录身份</h2><p>设置后即使 Telegram Bot 离线，也可以使用登录名和密码进入用户中心。</p></div>
      <label><span>Web 登录名</span><input v-model.trim="localUsername" minlength="3" maxlength="32" required autocomplete="username" /></label>
      <label><span>新密码</span><input v-model="localPassword" type="password" minlength="10" maxlength="128" required autocomplete="new-password" /></label>
      <label><span>确认新密码</span><input v-model="confirmLocalPassword" type="password" minlength="10" maxlength="128" required autocomplete="new-password" /></label>
      <p v-if="identityNotice" class="success-banner">{{ identityNotice }}</p><p v-if="identityError" class="form-error">{{ identityError }}</p>
      <button class="primary-button"><KeyRound :size="17" /> 保存 Web 登录身份</button>
    </form>

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

<style scoped>
.local-identity-card{display:grid;grid-template-columns:1.2fr 1fr 1fr 1fr auto;align-items:end;gap:14px;padding:22px}.local-identity-card>div p{margin-top:6px;color:var(--muted);font-size:10px}.local-identity-card label{display:grid;gap:7px}.local-identity-card input{padding:10px 12px;color:var(--text);border:1px solid var(--border);border-radius:10px;background:rgba(255,255,255,.035)}@media(max-width:1050px){.local-identity-card{grid-template-columns:1fr 1fr}.local-identity-card>div{grid-column:1/-1}}@media(max-width:650px){.local-identity-card{grid-template-columns:1fr}}
</style>
