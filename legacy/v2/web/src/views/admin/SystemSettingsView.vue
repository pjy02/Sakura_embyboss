<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import {
  Clock3,
  History,
  RefreshCw,
  RotateCcw,
  Save,
  Settings2,
  SlidersHorizontal,
} from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import DetailDrawer from "@/components/admin/DetailDrawer.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { ConfigRevision, DynamicSetting, DynamicSettingValue } from "@/types";

const sessionStore = useSessionStore();
const settings = ref<DynamicSetting[]>([]);
const drafts = reactive<Record<string, DynamicSettingValue>>({});
const loading = ref(true);
const refreshing = ref(false);
const busyKey = ref("");
const error = ref("");
const notice = ref("");
const historyOpen = ref(false);
const historyLoading = ref(false);
const historySetting = ref<DynamicSetting | null>(null);
const revisions = ref<ConfigRevision[]>([]);

const canManage = computed(() =>
  sessionStore.session?.permissions.some(
    (item) => item === "*" || item === "settings:*" || item === "settings:manage",
  ),
);

const groups = computed(() => {
  const result = new Map<string, DynamicSetting[]>();
  for (const item of settings.value) {
    const entries = result.get(item.group) || [];
    entries.push(item);
    result.set(item.group, entries);
  }
  return [...result.entries()].map(([name, items]) => ({ name, items }));
});

function applyResult(items: DynamicSetting[]) {
  settings.value = items;
  for (const item of items) drafts[item.key] = item.value;
}

function unchanged(item: DynamicSetting) {
  return JSON.stringify(drafts[item.key]) === JSON.stringify(item.value);
}

function inputValue(key: string) {
  return String(drafts[key] ?? "");
}

function updateBoolean(key: string, event: Event) {
  drafts[key] = (event.target as HTMLInputElement).checked;
}

function updateInput(item: DynamicSetting, event: Event) {
  const value = (event.target as HTMLInputElement | HTMLSelectElement).value;
  drafts[item.key] = item.value_type === "integer" ? Number.parseInt(value || "0", 10) : value;
}

async function load(silent = false) {
  if (silent) refreshing.value = true;
  else loading.value = true;
  error.value = "";
  try {
    const result = await api<{ items: DynamicSetting[] }>("/admin/settings");
    applyResult(result.items);
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "系统设置加载失败";
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
}

async function save(item: DynamicSetting) {
  if (!canManage.value || unchanged(item)) return;
  busyKey.value = item.key;
  error.value = "";
  notice.value = "";
  try {
    const updated = await api<DynamicSetting>(`/admin/settings/${encodeURIComponent(item.key)}`, {
      method: "PATCH",
      body: JSON.stringify({
        value: drafts[item.key],
        expected_revision: item.revision,
      }),
    });
    const index = settings.value.findIndex((entry) => entry.key === item.key);
    if (index >= 0) settings.value[index] = updated;
    drafts[item.key] = updated.value;
    notice.value = `${updated.label}已保存${updated.restart_required ? "，重启相关服务后完全生效" : "并同步到运行时"}。`;
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "设置保存失败";
    await load(true);
  } finally {
    busyKey.value = "";
  }
}

async function openHistory(item: DynamicSetting) {
  historySetting.value = item;
  historyOpen.value = true;
  historyLoading.value = true;
  revisions.value = [];
  try {
    const result = await api<{ items: ConfigRevision[] }>(
      `/admin/settings/${encodeURIComponent(item.key)}/history?limit=30`,
    );
    revisions.value = result.items;
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "变更历史加载失败";
  } finally {
    historyLoading.value = false;
  }
}

async function rollback(revision: ConfigRevision) {
  const item = historySetting.value;
  if (!item || !canManage.value) return;
  busyKey.value = item.key;
  error.value = "";
  try {
    const updated = await api<DynamicSetting>(
      `/admin/settings/${encodeURIComponent(item.key)}/rollback`,
      {
        method: "POST",
        body: JSON.stringify({
          target_revision: revision.revision,
          expected_revision: item.revision,
        }),
      },
    );
    const index = settings.value.findIndex((entry) => entry.key === item.key);
    if (index >= 0) settings.value[index] = updated;
    drafts[item.key] = updated.value;
    historySetting.value = updated;
    notice.value = `${updated.label}已回滚到版本 ${revision.revision}。`;
    await openHistory(updated);
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "设置回滚失败";
    await load(true);
  } finally {
    busyKey.value = "";
  }
}

useRealtimeEvents(["setting.updated"], () => load(true), true);
load();
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="RUNTIME CONFIGURATION"
      title="系统设置"
      description="集中管理允许在线修改的业务参数，所有变更均记录版本、操作人和审计日志。"
      :icon="Settings2"
    >
      <template #actions>
        <button class="secondary-button" :disabled="refreshing" @click="load(true)">
          <RefreshCw :size="16" :class="{ spin: refreshing }" /> 刷新配置
        </button>
      </template>
    </AdminPageHeader>

    <section class="settings-notice">
      <Settings2 :size="20" />
      <div>
        <strong>安全边界</strong>
        <p>密钥、数据库密码和 Telegram Token 仍由环境变量或 config.json 管理，本页不读取也不展示这些敏感值。</p>
      </div>
    </section>

    <div v-if="error" class="error-banner">{{ error }}</div>
    <div v-if="notice" class="success-banner">{{ notice }}</div>
    <LoadingBlock v-if="loading" />
    <EmptyState
      v-else-if="!settings.length"
      title="暂无动态设置"
      description="请确认当前镜像已包含最新迁移，并检查后端服务状态。"
    />

    <section v-else class="settings-groups">
      <article v-for="group in groups" :key="group.name" class="panel settings-group">
        <div class="panel-heading settings-heading">
          <span class="settings-group-icon"><SlidersHorizontal :size="19" /></span>
          <div><span class="section-kicker">CONFIG GROUP</span><h2>{{ group.name }}</h2></div>
        </div>
        <div class="settings-list">
          <div v-for="item in group.items" :key="item.key" class="setting-row">
            <div class="setting-copy">
              <div class="setting-title">
                <strong>{{ item.label }}</strong>
                <StatusBadge
                  :label="item.source === 'database' ? `动态版本 ${item.revision}` : '默认配置'"
                  :tone="item.source === 'database' ? 'info' : 'muted'"
                />
                <StatusBadge v-if="item.restart_required" label="需重启" tone="warning" />
              </div>
              <p>{{ item.description }}</p>
              <code>{{ item.key }}</code>
            </div>

            <div class="setting-control">
              <label v-if="item.value_type === 'boolean'" class="setting-switch">
                <input
                  type="checkbox"
                  :checked="Boolean(drafts[item.key])"
                  :disabled="!canManage || busyKey === item.key"
                  @change="updateBoolean(item.key, $event)"
                />
                <span><i /></span>
                <em>{{ drafts[item.key] ? "已开启" : "已关闭" }}</em>
              </label>
              <select
                v-else-if="item.options.length"
                class="compact-select setting-input"
                :value="inputValue(item.key)"
                :disabled="!canManage || busyKey === item.key"
                @change="updateInput(item, $event)"
              >
                <option v-for="option in item.options" :key="option" :value="option">{{ option }}</option>
              </select>
              <input
                v-else
                class="form-input setting-input"
                :type="item.value_type === 'integer' ? 'number' : 'text'"
                :min="item.minimum ?? undefined"
                :max="item.maximum ?? undefined"
                :value="inputValue(item.key)"
                :disabled="!canManage || busyKey === item.key"
                @input="updateInput(item, $event)"
              />
              <div class="setting-actions">
                <button class="secondary-button" @click="openHistory(item)">
                  <History :size="15" /> 历史
                </button>
                <button
                  v-if="canManage"
                  class="primary-button"
                  :disabled="unchanged(item) || busyKey === item.key"
                  @click="save(item)"
                >
                  <Save :size="15" />{{ busyKey === item.key ? "保存中" : "保存" }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </article>
    </section>

    <DetailDrawer
      :open="historyOpen"
      :title="historySetting?.label || '设置历史'"
      eyebrow="CONFIG REVISIONS"
      description="每次保存或回滚都会形成不可变的版本记录。"
      @close="historyOpen = false"
    >
      <LoadingBlock v-if="historyLoading" />
      <EmptyState
        v-else-if="!revisions.length"
        title="暂无变更历史"
        description="首次保存该设置后，版本记录会显示在这里。"
      />
      <div v-else class="revision-list">
        <article v-for="revision in revisions" :key="revision.id">
          <header>
            <strong>版本 {{ revision.revision }}</strong>
            <span><Clock3 :size="13" /> {{ formatDate(revision.created_at) }}</span>
          </header>
          <p>{{ revision.actor_kind }}:{{ revision.actor_id }}</p>
          <div><small>旧值</small><code>{{ JSON.stringify(revision.old_value) }}</code></div>
          <div><small>新值</small><code>{{ JSON.stringify(revision.new_value) }}</code></div>
          <button
            v-if="canManage"
            class="secondary-button"
            :disabled="busyKey === historySetting?.key"
            @click="rollback(revision)"
          >
            <RotateCcw :size="14" /> 回滚到此版本
          </button>
        </article>
      </div>
    </DetailDrawer>
  </div>
</template>

<style scoped>
.settings-notice {
  display: flex;
  align-items: flex-start;
  gap: 13px;
  padding: 16px 18px;
  color: var(--cyan);
  border: 1px solid rgba(89, 213, 209, .15);
  border-radius: 14px;
  background: rgba(89, 213, 209, .055);
}
.settings-notice svg { flex: 0 0 auto; }
.settings-notice p { margin: 5px 0 0; color: var(--muted); line-height: 1.6; }
.success-banner { padding: 12px 15px; color: var(--green); border: 1px solid rgba(113,211,155,.16); border-radius: 11px; background: rgba(113,211,155,.06); }
.settings-groups { display: grid; gap: 17px; }
.settings-heading { display: flex; align-items: center; gap: 12px; padding: 19px 20px; }
.settings-group-icon { display: grid; place-items: center; width: 38px; height: 38px; color: var(--pink); border-radius: 12px; background: rgba(241,113,165,.09); }
.settings-list { display: grid; }
.setting-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(270px, 390px); gap: 24px; padding: 20px; border-top: 1px solid var(--border); }
.setting-title, .setting-actions, .revision-list header, .revision-list header span { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.setting-copy p { margin: 7px 0; color: var(--muted); line-height: 1.55; }
.setting-copy code { color: var(--muted-2); font-size: 11px; }
.setting-control { display: grid; align-content: center; gap: 11px; }
.setting-input { width: 100%; }
input.setting-input { height: 39px; padding: 0 11px; color: var(--text); border: 1px solid var(--border); border-radius: 9px; outline: 0; background: rgba(255,255,255,.025); }
.setting-actions { justify-content: flex-end; }
.setting-switch { display: flex; align-items: center; gap: 10px; cursor: pointer; }
.setting-switch input { position: absolute; opacity: 0; pointer-events: none; }
.setting-switch > span { display: flex; align-items: center; width: 43px; height: 24px; padding: 3px; border-radius: 999px; background: rgba(255,255,255,.12); transition: .18s ease; }
.setting-switch i { width: 18px; height: 18px; border-radius: 50%; background: white; transition: .18s ease; }
.setting-switch input:checked + span { background: var(--pink); }
.setting-switch input:checked + span i { transform: translateX(19px); }
.setting-switch em { color: var(--muted); font-style: normal; }
.revision-list { display: grid; gap: 12px; }
.revision-list article { display: grid; gap: 10px; padding: 14px; border: 1px solid var(--border); border-radius: 12px; background: rgba(255,255,255,.025); }
.revision-list header { justify-content: space-between; }
.revision-list header span, .revision-list p, .revision-list small { color: var(--muted); font-size: 11px; }
.revision-list p { margin: 0; }
.revision-list article > div { display: grid; grid-template-columns: 42px minmax(0,1fr); gap: 8px; }
.revision-list code { overflow-wrap: anywhere; }
@media (max-width: 760px) {
  .setting-row { grid-template-columns: 1fr; gap: 15px; }
  .setting-actions .primary-button, .setting-actions .secondary-button { flex: 1; }
}
</style>
