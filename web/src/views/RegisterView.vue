<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import {
  ArrowRight,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  LoaderCircle,
  LockKeyhole,
  MessageCircle,
  ShieldCheck,
  Sparkles,
  UserRoundPlus,
  UsersRound,
  X,
} from "lucide-vue-next";
import QRCode from "qrcode";
import BrandMark from "@/components/BrandMark.vue";
import { api, idempotencyKey } from "@/lib/api";
import { useSessionStore } from "@/stores/session";
import type {
  RegistrationStatus,
  RegistrationTask,
  TelegramLogin,
} from "@/types";

type Step = "identity" | "form" | "queue" | "success" | "account";

const sessionStore = useSessionStore();
const identityMode = ref<"local" | "telegram">("local");
const loginName = ref("");
const loginPassword = ref("");
const confirmLoginPassword = ref("");
const displayName = ref("");
const creatingLocal = ref(false);
const publicStatus = ref<RegistrationStatus | null>(null);
const memberStatus = ref<RegistrationStatus | null>(null);
const telegram = ref<TelegramLogin | null>(null);
const telegramState = ref<"idle" | "creating" | "pending" | "approved" | "expired">("idle");
const qrData = ref("");
const task = ref<RegistrationTask | null>(null);
const username = ref("");
const safetyCode = ref("");
const confirmSafetyCode = ref("");
const registrationCode = ref("");
const submitting = ref(false);
const canceling = ref(false);
const loading = ref(true);
const error = ref("");
const copied = ref("");
let telegramTimer: number | undefined;
let taskTimer: number | undefined;

const status = computed(() => memberStatus.value || publicStatus.value);
const registrationAuthorized = computed(
  () => sessionStore.session?.purpose === "registration",
);
const needsCode = computed(
  () =>
    Boolean(memberStatus.value) &&
    Boolean(memberStatus.value?.requires_invite) &&
    Number(memberStatus.value?.qualification_days || 0) <= 0,
);
const step = computed<Step>(() => {
  if (task.value?.status === "succeeded" && task.value.result?.ok) return "success";
  if (memberStatus.value?.has_account) return "account";
  if (
    task.value &&
    ["pending", "retrying", "running"].includes(task.value.status)
  ) {
    return "queue";
  }
  return registrationAuthorized.value ? "form" : "identity";
});
const capacityPercent = computed(() => {
  const value = status.value;
  if (!value?.user_limit) return 0;
  return Math.min(100, Math.round((value.registered / value.user_limit) * 100));
});
const queueText = computed(() => {
  if (!task.value) return "";
  if (task.value.status === "running") return "正在创建 Emby 账号";
  if (task.value.status === "retrying") return "正在等待重新处理";
  return `排队等待中${task.value.position ? ` · 前方约 ${Math.max(0, task.value.position - 1)} 项` : ""}`;
});

async function loadPublicStatus() {
  publicStatus.value = await api<RegistrationStatus>("/registration/status");
}

async function loadMemberStatus() {
  memberStatus.value = await api<RegistrationStatus>("/registration/me");
  if (memberStatus.value.active_task) {
    task.value = memberStatus.value.active_task;
    scheduleTaskPoll();
  }
}

async function initialize() {
  loading.value = true;
  error.value = "";
  try {
    await loadPublicStatus();
    if (sessionStore.authenticated) await loadMemberStatus();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "注册中心暂时无法加载";
  } finally {
    loading.value = false;
  }
}

async function startTelegram() {
  telegramState.value = "creating";
  error.value = "";
  window.clearTimeout(telegramTimer);
  try {
    telegram.value = await api<TelegramLogin>("/registration/telegram/start", {
      method: "POST",
      body: "{}",
    });
    qrData.value = await QRCode.toDataURL(telegram.value.deep_link, {
      width: 260,
      margin: 1,
      color: { dark: "#17131d", light: "#fffafd" },
    });
    telegramState.value = "pending";
    scheduleTelegramPoll();
  } catch (reason) {
    telegramState.value = "idle";
    error.value = reason instanceof Error ? reason.message : "无法创建 Telegram 验证请求";
  }
}

async function startLocalAccount() {
  error.value = "";
  if (loginPassword.value !== confirmLoginPassword.value) {
    error.value = "两次输入的 Web 登录密码不一致";
    return;
  }
  creatingLocal.value = true;
  try {
    await api("/registration/local/start", {
      method: "POST",
      body: JSON.stringify({
        login_name: loginName.value,
        password: loginPassword.value,
        display_name: displayName.value || null,
      }),
    });
    await sessionStore.load();
    await loadMemberStatus();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "无法创建 Web 登录账号";
  } finally {
    creatingLocal.value = false;
  }
}

function scheduleTelegramPoll() {
  window.clearTimeout(telegramTimer);
  telegramTimer = window.setTimeout(
    pollTelegram,
    (telegram.value?.poll_after_seconds || 2) * 1000,
  );
}

async function pollTelegram() {
  if (!telegram.value || telegramState.value !== "pending") return;
  try {
    const result = await api<{ status: string }>(
      "/registration/telegram/status",
      {
        method: "POST",
        body: JSON.stringify({ token: telegram.value.request_token }),
      },
    );
    if (result.status === "approved") {
      telegramState.value = "approved";
      await api("/registration/telegram/exchange", {
        method: "POST",
        body: JSON.stringify({ token: telegram.value.request_token }),
      });
      await sessionStore.load();
      await loadMemberStatus();
      return;
    }
    if (["expired", "rejected", "consumed"].includes(result.status)) {
      telegramState.value = "expired";
      return;
    }
    scheduleTelegramPoll();
  } catch {
    scheduleTelegramPoll();
  }
}

async function submitRegistration() {
  error.value = "";
  if (!/^[^\s/\\<>]{2,32}$/.test(username.value)) {
    error.value = "用户名需为 2–32 个字符，不能包含空格、斜杠或尖括号";
    return;
  }
  if (!/^\d{4,6}$/.test(safetyCode.value)) {
    error.value = "安全码必须是 4–6 位数字";
    return;
  }
  if (safetyCode.value !== confirmSafetyCode.value) {
    error.value = "两次输入的安全码不一致";
    return;
  }
  if (needsCode.value && !registrationCode.value.trim()) {
    error.value = "当前未开放注册，请填写注册码";
    return;
  }

  submitting.value = true;
  try {
    task.value = await api<RegistrationTask>("/registration/submit", {
      method: "POST",
      idempotencyKey: idempotencyKey("registration"),
      body: JSON.stringify({
        username: username.value,
        safety_code: safetyCode.value,
        registration_code: registrationCode.value || null,
      }),
    });
    scheduleTaskPoll();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "注册任务提交失败";
  } finally {
    submitting.value = false;
  }
}

function scheduleTaskPoll() {
  window.clearTimeout(taskTimer);
  if (!task.value || !["pending", "retrying", "running"].includes(task.value.status)) {
    return;
  }
  taskTimer = window.setTimeout(pollTask, 1800);
}

async function pollTask() {
  if (!task.value) return;
  try {
    task.value = await api<RegistrationTask>(`/registration/tasks/${task.value.id}`);
    if (task.value.status === "succeeded" && !task.value.result?.ok) {
      error.value = task.value.result?.message || "账号创建失败，请检查信息后重试";
    } else if (task.value.status === "failed") {
      error.value = task.value.error_message || "任务处理失败，请稍后重试";
    }
    if (["succeeded", "failed", "canceled"].includes(task.value.status)) {
      await loadMemberStatus();
    } else {
      scheduleTaskPoll();
    }
  } catch {
    scheduleTaskPoll();
  }
}

async function cancelTask() {
  if (!task.value) return;
  canceling.value = true;
  error.value = "";
  try {
    task.value = await api<RegistrationTask>(
      `/registration/tasks/${task.value.id}/cancel`,
      { method: "POST", body: "{}" },
    );
    await loadMemberStatus();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "当前任务无法取消";
  } finally {
    canceling.value = false;
  }
}

function retryForm() {
  task.value = null;
  error.value = "";
  window.clearTimeout(taskTimer);
}

async function copyValue(label: string, value?: string) {
  if (!value) return;
  await navigator.clipboard.writeText(value);
  copied.value = label;
  window.setTimeout(() => {
    if (copied.value === label) copied.value = "";
  }, 1600);
}

onMounted(initialize);
onBeforeUnmount(() => {
  window.clearTimeout(telegramTimer);
  window.clearTimeout(taskTimer);
});
</script>

<template>
  <main class="register-page">
    <div class="register-orb orb-one" />
    <div class="register-orb orb-two" />

    <header class="register-header">
      <BrandMark />
      <RouterLink to="/login">已有账号，去登录 <ArrowRight :size="15" /></RouterLink>
    </header>

    <section class="register-layout">
      <aside class="register-story">
        <span class="story-kicker">SAKURA ACCESS</span>
        <h1>从一个身份，走进你的影音花园。</h1>
        <p>直接创建 Web 登录身份并提交 Emby 账号申请；也可以选择绑定 Telegram。两种入口共享账号、资格与注册进度。</p>

        <div class="feature-list">
          <div>
            <span><ShieldCheck :size="20" /></span>
            <div><strong>独立 Web 身份</strong><small>不安装 Telegram 也能注册、登录和管理账号</small></div>
          </div>
          <div>
            <span><Clock3 :size="20" /></span>
            <div><strong>可靠注册队列</strong><small>重启不丢任务，状态持续同步</small></div>
          </div>
          <div>
            <span><Sparkles :size="20" /></span>
            <div><strong>创建后立即可用</strong><small>一次展示账号凭据，请妥善保存</small></div>
          </div>
        </div>

        <div v-if="status" class="capacity-card">
          <div>
            <span>当前名额</span>
            <strong>{{ status.remaining }}<small> / {{ status.user_limit }}</small></strong>
          </div>
          <div class="capacity-track"><i :style="{ width: `${capacityPercent}%` }" /></div>
          <p>
            已注册 {{ status.registered }} 人
            <span>·</span>
            队列 {{ status.queue_waiting }} 项
          </p>
        </div>
      </aside>

      <section class="register-card">
        <div class="stepper" aria-label="注册步骤">
          <div :class="{ active: step === 'identity', done: registrationAuthorized }">
            <span>{{ registrationAuthorized ? "✓" : "1" }}</span><small>确认身份</small>
          </div>
          <i />
          <div :class="{ active: step === 'form', done: ['queue', 'success', 'account'].includes(step) }">
            <span>2</span><small>填写资料</small>
          </div>
          <i />
          <div :class="{ active: step === 'queue', done: ['success', 'account'].includes(step) }">
            <span>3</span><small>创建账号</small>
          </div>
        </div>

        <div v-if="loading" class="center-state">
          <LoaderCircle class="spin" :size="30" />
          <strong>正在载入注册中心</strong>
        </div>

        <template v-else-if="step === 'identity'">
          <div class="card-heading">
            <span class="heading-icon"><UserRoundPlus :size="22" /></span>
            <div><small>STEP 01</small><h2>创建或确认 Sakura 身份</h2></div>
          </div>
          <p class="card-description">可以直接创建 Web 登录账号，不使用 Telegram；也可以继续通过 Telegram 完成身份确认。</p>

          <div class="method-switch">
            <button :class="{ active: identityMode === 'local' }" @click="identityMode = 'local'">
              <LockKeyhole :size="17" /> Web 账号
            </button>
            <button :class="{ active: identityMode === 'telegram' }" @click="identityMode = 'telegram'">
              <MessageCircle :size="17" /> Telegram
            </button>
          </div>

          <form v-if="identityMode === 'local'" class="register-form" @submit.prevent="startLocalAccount">
            <label>
              <span>Web 登录名</span>
              <input v-model.trim="loginName" minlength="3" maxlength="32" autocomplete="username" required placeholder="3–32 个字符" />
            </label>
            <label>
              <span>显示名称（可选）</span>
              <input v-model.trim="displayName" maxlength="255" autocomplete="nickname" placeholder="用户中心显示的名称" />
            </label>
            <div class="form-grid">
              <label>
                <span>登录密码</span>
                <input v-model="loginPassword" minlength="10" maxlength="128" type="password" autocomplete="new-password" required placeholder="至少 10 个字符" />
              </label>
              <label>
                <span>确认登录密码</span>
                <input v-model="confirmLoginPassword" minlength="10" maxlength="128" type="password" autocomplete="new-password" required placeholder="再次输入" />
              </label>
            </div>
            <button class="register-primary" :disabled="creatingLocal">
              <LoaderCircle v-if="creatingLocal" class="spin" :size="18" />
              <UserRoundPlus v-else :size="18" />
              {{ creatingLocal ? "正在创建…" : "创建 Web 账号并继续" }}
            </button>
          </form>

          <template v-else>
          <button
            v-if="telegramState === 'idle'"
            class="register-primary"
            @click="startTelegram"
          >
            <MessageCircle :size="18" /> 使用 Telegram 继续
          </button>
          <div v-else-if="telegramState === 'creating'" class="center-state compact">
            <LoaderCircle class="spin" :size="26" /><strong>正在创建安全验证…</strong>
          </div>
          <div v-else-if="telegram && telegramState === 'pending'" class="telegram-flow">
            <div class="qr-shell"><img :src="qrData" alt="Telegram 注册验证二维码" /></div>
            <div>
              <strong>在 Telegram 中确认注册</strong>
              <p>扫描二维码，或在当前设备直接打开 Bot。</p>
              <a :href="telegram.deep_link" target="_blank" rel="noreferrer">
                打开 Telegram <ExternalLink :size="15" />
              </a>
              <span><i /> 等待确认中</span>
            </div>
          </div>
          <div v-else-if="telegramState === 'approved'" class="center-state compact">
            <LoaderCircle class="spin" :size="26" /><strong>身份已确认，正在准备资料页…</strong>
          </div>
          <div v-else class="retry-box">
            <X :size="20" /><div><strong>验证请求已失效</strong><p>请重新发起一次 Telegram 验证。</p></div>
            <button @click="startTelegram">重新验证</button>
          </div>
          </template>
        </template>

        <template v-else-if="step === 'form'">
          <div class="card-heading">
            <span class="heading-icon"><LockKeyhole :size="22" /></span>
            <div><small>STEP 02</small><h2>设置你的站点账号</h2></div>
          </div>
          <p class="card-description">
            {{ sessionStore.session?.auth_method === "local" ? "Web 登录账号已创建。" : `Telegram ID ${sessionStore.session?.tg} 已验证。` }}
            <template v-if="memberStatus?.enabled && !memberStatus?.requires_invite">当前为开放注册，账号有效期 {{ memberStatus.open_registration_days }} 天。</template>
            <template v-else-if="memberStatus?.qualification_days">已拥有 {{ memberStatus.qualification_days }} 天注册资格。</template>
            <template v-else>当前为邀请注册，请填写注册码。</template>
          </p>

          <form class="register-form" @submit.prevent="submitRegistration">
            <label>
              <span>Emby 用户名</span>
              <input v-model.trim="username" maxlength="32" autocomplete="username" placeholder="2–32 个字符" required />
              <small>支持中文、英文、数字和 emoji，不含空格与斜杠。</small>
            </label>
            <div class="form-grid">
              <label>
                <span>安全码</span>
                <input v-model="safetyCode" maxlength="6" inputmode="numeric" type="password" autocomplete="new-password" placeholder="4–6 位数字" required />
              </label>
              <label>
                <span>确认安全码</span>
                <input v-model="confirmSafetyCode" maxlength="6" inputmode="numeric" type="password" autocomplete="new-password" placeholder="再次输入" required />
              </label>
            </div>
            <label v-if="needsCode">
              <span>注册码</span>
              <input v-model.trim="registrationCode" autocomplete="off" placeholder="请输入 Bot 或管理员发放的注册码" required />
            </label>
            <div class="safety-note">
              <ShieldCheck :size="17" />
              <p>安全码用于重置密码、删除账号等敏感操作。它不会出现在任务列表或管理日志中。</p>
            </div>
            <button class="register-primary" :disabled="submitting || !status?.remaining">
              <LoaderCircle v-if="submitting" class="spin" :size="18" />
              <UserRoundPlus v-else :size="18" />
              {{ submitting ? "正在提交…" : status?.remaining ? "提交注册" : "当前名额已满" }}
            </button>
          </form>
        </template>

        <template v-else-if="step === 'queue'">
          <div class="queue-visual">
            <div class="queue-ring"><LoaderCircle class="spin" :size="34" /></div>
            <span>REGISTRATION QUEUE</span>
            <h2>{{ queueText }}</h2>
            <p>任务已持久化保存，可以关闭页面。再次进入注册中心仍可继续查看进度。</p>
          </div>
          <div class="task-meta">
            <div><small>账号名</small><strong>{{ task?.username || username }}</strong></div>
            <div><small>任务状态</small><strong>{{ task?.status }}</strong></div>
            <div><small>任务编号</small><strong>{{ task?.id.slice(0, 8) }}</strong></div>
          </div>
          <button
            v-if="task?.status === 'pending' || task?.status === 'retrying'"
            class="cancel-button"
            :disabled="canceling"
            @click="cancelTask"
          >
            <X :size="16" /> {{ canceling ? "正在取消…" : "取消本次注册" }}
          </button>
        </template>

        <template v-else-if="step === 'success'">
          <div class="success-heading">
            <span><CheckCircle2 :size="32" /></span>
            <small>WELCOME TO SAKURA</small>
            <h2>你的账号已经准备好了</h2>
            <p>请立即保存下面的登录信息。离开此页面后，建议通过用户中心安全地管理密码。</p>
          </div>
          <div class="credentials">
            <div>
              <span>Emby 用户名</span>
              <strong>{{ task?.result?.username }}</strong>
              <button @click="copyValue('username', task?.result?.username)">
                <Check v-if="copied === 'username'" :size="15" /><Copy v-else :size="15" />
              </button>
            </div>
            <div>
              <span>初始密码</span>
              <strong>{{ task?.result?.emby_password }}</strong>
              <button @click="copyValue('password', task?.result?.emby_password)">
                <Check v-if="copied === 'password'" :size="15" /><Copy v-else :size="15" />
              </button>
            </div>
            <div>
              <span>账号到期时间</span>
              <strong>{{ task?.result?.expires_at }}</strong>
            </div>
          </div>
          <RouterLink class="register-primary" to="/">进入用户中心 <ArrowRight :size="17" /></RouterLink>
        </template>

        <template v-else>
          <div class="success-heading account-ready">
            <span><UsersRound :size="32" /></span>
            <small>ACCOUNT CONNECTED</small>
            <h2>当前 Telegram 已经拥有站点账号</h2>
            <p>无需重复注册，你可以直接进入用户中心查看账号状态、到期时间和安全设置。</p>
          </div>
          <RouterLink class="register-primary" to="/">进入用户中心 <ArrowRight :size="17" /></RouterLink>
        </template>

        <div v-if="error" class="register-error">
          <X :size="17" /><span>{{ error }}</span>
          <button v-if="task && ['failed', 'canceled', 'succeeded'].includes(task.status)" @click="retryForm">
            重新填写
          </button>
        </div>
      </section>
    </section>

    <footer class="register-footer">
      <ShieldCheck :size="14" /> 身份验证、注册资格与任务操作均会记录安全审计
    </footer>
  </main>
</template>

<style scoped>
.register-page {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  padding: 28px clamp(22px, 5vw, 76px) 24px;
  background:
    radial-gradient(circle at 15% 5%, rgba(241, 113, 165, .12), transparent 28%),
    radial-gradient(circle at 90% 85%, rgba(101, 81, 180, .1), transparent 30%),
    #0a090e;
}
.register-orb { position: absolute; border-radius: 50%; filter: blur(90px); pointer-events: none; opacity: .16; }
.orb-one { width: 340px; height: 340px; top: 18%; left: -220px; background: #f171a5; }
.orb-two { width: 300px; height: 300px; right: -160px; bottom: -110px; background: #826ee3; }
.register-header { position: relative; z-index: 2; display: flex; align-items: center; justify-content: space-between; width: min(1180px, 100%); margin: 0 auto; }
.register-header > a { display: inline-flex; align-items: center; gap: 7px; color: var(--muted); font-size: 11px; transition: .18s ease; }
.register-header > a:hover { color: var(--pink-strong); }
.register-layout { position: relative; z-index: 1; display: grid; grid-template-columns: minmax(280px, .9fr) minmax(430px, 1.1fr); align-items: center; gap: clamp(45px, 7vw, 110px); width: min(1120px, 100%); min-height: calc(100vh - 150px); margin: 20px auto 0; }
.register-story { padding: 28px 0; }
.story-kicker { color: var(--pink); font-size: 10px; font-weight: 700; letter-spacing: .25em; }
.register-story h1 { max-width: 560px; margin: 15px 0 19px; font-size: clamp(38px, 5vw, 66px); line-height: 1.08; letter-spacing: -.055em; }
.register-story > p { max-width: 510px; color: var(--muted); font-size: 13px; line-height: 1.85; }
.feature-list { display: grid; gap: 16px; margin-top: 35px; }
.feature-list > div { display: flex; align-items: center; gap: 14px; }
.feature-list > div > span { display: grid; place-items: center; flex: none; width: 42px; height: 42px; color: var(--pink-strong); border: 1px solid rgba(241,113,165,.16); border-radius: 13px; background: rgba(241,113,165,.08); }
.feature-list div div { display: grid; gap: 4px; }
.feature-list strong { font-size: 12px; font-weight: 600; }
.feature-list small { color: var(--muted-2); font-size: 10px; }
.capacity-card { margin-top: 35px; padding: 18px 20px; border: 1px solid var(--border); border-radius: 16px; background: rgba(255,255,255,.025); }
.capacity-card > div:first-child { display: flex; align-items: center; justify-content: space-between; }
.capacity-card span, .capacity-card p { color: var(--muted); font-size: 9px; }
.capacity-card strong { font-size: 20px; }
.capacity-card strong small { color: var(--muted-2); font-size: 10px; }
.capacity-track { height: 5px; margin: 12px 0; overflow: hidden; border-radius: 99px; background: rgba(255,255,255,.055); }
.capacity-track i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, var(--pink), var(--violet)); }
.capacity-card p { display: flex; gap: 7px; }
.register-card { position: relative; min-height: 580px; padding: clamp(26px, 4vw, 43px); border: 1px solid rgba(255,255,255,.1); border-radius: 26px; background: linear-gradient(145deg, rgba(29,25,35,.96), rgba(17,15,21,.96)); box-shadow: 0 34px 90px rgba(0,0,0,.34); backdrop-filter: blur(20px); }
.stepper { display: grid; grid-template-columns: auto 1fr auto 1fr auto; align-items: center; gap: 10px; margin-bottom: 42px; }
.stepper > div { display: flex; align-items: center; gap: 7px; color: var(--muted-2); }
.stepper > div > span { display: grid; place-items: center; width: 25px; height: 25px; font-size: 9px; border: 1px solid var(--border); border-radius: 50%; background: rgba(255,255,255,.025); }
.stepper small { font-size: 8px; white-space: nowrap; }
.stepper > i { height: 1px; background: var(--border); }
.stepper > div.active { color: var(--text); }
.stepper > div.active > span { color: #2a1520; border-color: var(--pink); background: var(--pink); box-shadow: 0 0 18px rgba(241,113,165,.25); }
.stepper > div.done > span { color: var(--green); border-color: rgba(113,211,155,.3); background: rgba(113,211,155,.1); }
.card-heading { display: flex; align-items: center; gap: 14px; }
.heading-icon { display: grid; place-items: center; width: 45px; height: 45px; color: var(--pink-strong); border-radius: 14px; background: var(--pink-soft); }
.card-heading div { display: grid; gap: 5px; }
.card-heading small, .success-heading > small { color: var(--pink); font-size: 8px; font-weight: 700; letter-spacing: .2em; }
.card-heading h2 { font-size: 21px; letter-spacing: -.025em; }
.card-description { margin: 20px 0 27px; color: var(--muted); font-size: 11px; line-height: 1.8; }
.register-primary { display: flex; align-items: center; justify-content: center; gap: 9px; width: 100%; min-height: 47px; color: #29151f; font-size: 11px; font-weight: 700; border: 1px solid #ff9dc3; border-radius: 12px; background: linear-gradient(135deg, #ffb0cf, #ed73a5); box-shadow: 0 12px 28px rgba(241,113,165,.16); cursor: pointer; transition: .18s ease; }
.register-primary:hover { transform: translateY(-1px); filter: brightness(1.05); }
.register-primary:disabled { opacity: .45; cursor: not-allowed; transform: none; }
.center-state { display: grid; place-items: center; align-content: center; gap: 13px; min-height: 360px; color: var(--muted); }
.center-state.compact { min-height: 220px; }
.center-state strong { font-size: 11px; font-weight: 500; }
.spin { animation: spin .9s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.telegram-flow { display: grid; grid-template-columns: 178px 1fr; align-items: center; gap: 25px; }
.qr-shell { padding: 8px; border-radius: 17px; background: #fffafd; box-shadow: 0 15px 40px rgba(0,0,0,.2); }
.qr-shell img { display: block; width: 100%; border-radius: 10px; }
.telegram-flow > div:last-child { display: grid; justify-items: start; gap: 11px; }
.telegram-flow strong { font-size: 14px; }
.telegram-flow p { color: var(--muted); font-size: 10px; line-height: 1.7; }
.telegram-flow a { display: inline-flex; align-items: center; gap: 6px; color: var(--pink-strong); font-size: 10px; font-weight: 600; }
.telegram-flow div > span { display: flex; align-items: center; gap: 7px; color: var(--muted-2); font-size: 9px; }
.telegram-flow div > span i { width: 6px; height: 6px; border-radius: 50%; background: var(--green); box-shadow: 0 0 11px var(--green); animation: pulse 1.3s ease infinite; }
@keyframes pulse { 50% { opacity: .35; } }
.retry-box { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 13px; margin-top: 25px; padding: 15px; color: var(--red); border: 1px solid rgba(255,116,125,.16); border-radius: 13px; background: rgba(255,116,125,.06); }
.retry-box div { display: grid; gap: 4px; }
.retry-box strong { font-size: 11px; }
.retry-box p { color: var(--muted); font-size: 9px; }
.retry-box button, .register-error button { color: var(--pink-strong); font-size: 9px; border: 0; background: transparent; cursor: pointer; }
.register-form { display: grid; gap: 17px; }
.register-form label { display: grid; gap: 7px; }
.register-form label > span { color: #d9d0db; font-size: 10px; font-weight: 500; }
.register-form input { width: 100%; height: 44px; padding: 0 13px; color: var(--text); font-size: 11px; border: 1px solid var(--border); border-radius: 11px; outline: 0; background: rgba(255,255,255,.028); }
.register-form input:focus { border-color: rgba(241,113,165,.52); box-shadow: 0 0 0 3px rgba(241,113,165,.07); }
.register-form label small { color: var(--muted-2); font-size: 8px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.safety-note { display: flex; align-items: flex-start; gap: 10px; padding: 12px 13px; color: var(--green); border: 1px solid rgba(113,211,155,.13); border-radius: 11px; background: rgba(113,211,155,.045); }
.safety-note svg { flex: none; }
.safety-note p { color: #9ab5a6; font-size: 9px; line-height: 1.65; }
.queue-visual { display: grid; justify-items: center; padding: 24px 10px 30px; text-align: center; }
.queue-ring { display: grid; place-items: center; width: 82px; height: 82px; margin-bottom: 20px; color: var(--pink); border: 1px solid rgba(241,113,165,.22); border-radius: 50%; background: radial-gradient(circle, rgba(241,113,165,.13), transparent 67%); box-shadow: 0 0 45px rgba(241,113,165,.11); }
.queue-visual > span { color: var(--pink); font-size: 8px; font-weight: 700; letter-spacing: .2em; }
.queue-visual h2 { margin: 10px 0 8px; font-size: 20px; }
.queue-visual p { max-width: 390px; color: var(--muted); font-size: 10px; line-height: 1.7; }
.task-meta { display: grid; grid-template-columns: repeat(3, 1fr); overflow: hidden; margin-top: 4px; border: 1px solid var(--border); border-radius: 13px; }
.task-meta > div { display: grid; gap: 5px; padding: 14px; border-right: 1px solid var(--border); }
.task-meta > div:last-child { border: 0; }
.task-meta small { color: var(--muted-2); font-size: 8px; }
.task-meta strong { overflow: hidden; font-size: 10px; text-overflow: ellipsis; }
.cancel-button { display: flex; align-items: center; gap: 6px; margin: 18px auto 0; padding: 8px 12px; color: var(--muted); font-size: 9px; border: 1px solid var(--border); border-radius: 9px; background: transparent; cursor: pointer; }
.success-heading { display: grid; justify-items: center; padding: 17px 10px 24px; text-align: center; }
.success-heading > span { display: grid; place-items: center; width: 67px; height: 67px; margin-bottom: 16px; color: var(--green); border: 1px solid rgba(113,211,155,.2); border-radius: 21px; background: rgba(113,211,155,.08); }
.success-heading h2 { margin: 9px 0; font-size: 23px; }
.success-heading p { max-width: 390px; color: var(--muted); font-size: 10px; line-height: 1.7; }
.success-heading.account-ready > span { color: var(--pink); border-color: rgba(241,113,165,.2); background: var(--pink-soft); }
.credentials { display: grid; margin-bottom: 20px; padding: 4px 16px; border: 1px solid var(--border); border-radius: 14px; background: rgba(255,255,255,.02); }
.credentials > div { display: grid; grid-template-columns: 120px 1fr auto; align-items: center; gap: 12px; min-height: 51px; border-bottom: 1px solid var(--border); }
.credentials > div:last-child { border: 0; }
.credentials span { color: var(--muted); font-size: 9px; }
.credentials strong { overflow-wrap: anywhere; font-size: 11px; }
.credentials button { display: grid; place-items: center; width: 29px; height: 29px; color: var(--pink); border: 1px solid var(--border); border-radius: 8px; background: rgba(255,255,255,.025); cursor: pointer; }
.register-error { display: flex; align-items: center; gap: 8px; margin-top: 17px; padding: 11px 13px; color: #ff9da4; font-size: 9px; border: 1px solid rgba(255,116,125,.16); border-radius: 10px; background: rgba(255,116,125,.06); }
.register-error span { flex: 1; }
.register-footer { position: relative; z-index: 1; display: flex; align-items: center; justify-content: center; gap: 7px; color: var(--muted-2); font-size: 8px; }
@media (max-width: 900px) {
  .register-layout { grid-template-columns: 1fr; gap: 12px; padding-top: 35px; }
  .register-story { text-align: center; }
  .register-story h1, .register-story > p { margin-left: auto; margin-right: auto; }
  .feature-list { display: none; }
  .capacity-card { max-width: 500px; margin-left: auto; margin-right: auto; text-align: left; }
  .register-card { width: min(590px, 100%); min-height: 520px; margin: 0 auto 35px; }
}
@media (max-width: 560px) {
  .register-page { padding: 20px 15px; }
  .register-header > a { font-size: 0; }
  .register-header > a svg { width: 19px; height: 19px; }
  .register-story h1 { font-size: 38px; }
  .register-story > p { font-size: 11px; }
  .register-card { padding: 24px 19px; border-radius: 21px; }
  .stepper small { display: none; }
  .telegram-flow { grid-template-columns: 1fr; justify-items: center; text-align: center; }
  .telegram-flow .qr-shell { width: 180px; }
  .telegram-flow > div:last-child { justify-items: center; }
  .form-grid, .task-meta { grid-template-columns: 1fr; }
  .task-meta > div { border-right: 0; border-bottom: 1px solid var(--border); }
  .credentials > div { grid-template-columns: 95px 1fr auto; }
}
</style>
