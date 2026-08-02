<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import {
  Activity,
  CirclePlus,
  CloudCog,
  Gauge,
  MapPin,
  Network,
  Pencil,
  RefreshCw,
  Route,
  Wrench,
  X,
} from "lucide-vue-next";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useSessionStore } from "@/stores/session";
import type { LineEndpoint } from "@/types";

const sessionStore = useSessionStore();
const items = ref<LineEndpoint[]>([]);
const loading = ref(true);
const busyId = ref<number | null>(null);
const modalOpen = ref(false);
const editing = ref<LineEndpoint | null>(null);
const error = ref("");
const form = reactive({
  name: "",
  base_url: "",
  region: "",
  carrier: "",
  audience: "all" as "all" | "whitelist",
  weight: 100,
  sort_order: 0,
  enabled: true,
  maintenance: false,
});
const canUpdate = computed(() =>
  sessionStore.session?.permissions.some(
    (item) => item === "*" || item === "lines:*" || item === "lines:update",
  ),
);

function tone(line: LineEndpoint): "success" | "warning" | "danger" | "info" | "muted" {
  if (line.maintenance) return "warning";
  if (!line.enabled) return "muted";
  if (line.last_status === "healthy") return "success";
  if (line.last_status === "offline") return "danger";
  return "info";
}
function label(line: LineEndpoint) {
  if (line.maintenance) return "维护中";
  if (!line.enabled) return "已停用";
  if (line.last_status === "healthy") return "健康";
  if (line.last_status === "offline") return "离线";
  return "待探测";
}

async function load() {
  loading.value = true;
  try {
    const result = await api<{ items: LineEndpoint[] }>("/admin/lines");
    items.value = result.items;
  } finally {
    loading.value = false;
  }
}

function openEditor(line?: LineEndpoint) {
  editing.value = line || null;
  Object.assign(form, {
    name: line?.name || "",
    base_url: line?.base_url || "",
    region: line?.region || "",
    carrier: line?.carrier || "",
    audience: line?.audience || "all",
    weight: line?.weight ?? 100,
    sort_order: line?.sort_order ?? items.value.length,
    enabled: line?.enabled ?? true,
    maintenance: line?.maintenance ?? false,
  });
  error.value = "";
  modalOpen.value = true;
}

async function save() {
  error.value = "";
  const payload = { ...form, revision: editing.value?.revision };
  try {
    if (editing.value) {
      await api(`/admin/lines/${editing.value.id}`, { method: "PATCH", body: JSON.stringify(payload) });
    } else {
      const { revision: _revision, ...createPayload } = payload;
      await api("/admin/lines", { method: "POST", body: JSON.stringify(createPayload) });
    }
    modalOpen.value = false;
    await load();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "线路保存失败";
  }
}

async function probe(line: LineEndpoint) {
  busyId.value = line.id;
  try {
    await api(`/admin/lines/${line.id}/probe`, { method: "POST" });
    await load();
  } finally {
    busyId.value = null;
  }
}

async function toggleMaintenance(line: LineEndpoint) {
  busyId.value = line.id;
  try {
    await api(`/admin/lines/${line.id}`, {
      method: "PATCH",
      body: JSON.stringify({ revision: line.revision, maintenance: !line.maintenance }),
    });
    await load();
  } finally {
    busyId.value = null;
  }
}

onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="ROUTE OPERATIONS"
      title="线路管理"
      description="集中维护访问线路、地区、运营商和权重，保留每次健康探测结果与管理审计。"
      :icon="Network"
    >
      <template #meta><span class="date-chip"><Route :size="16" /> {{ items.length }} 条线路</span></template>
      <template #actions><button v-if="canUpdate" class="primary-button" @click="openEditor()"><CirclePlus :size="17" /> 新增线路</button></template>
    </AdminPageHeader>

    <LoadingBlock v-if="loading" />
    <EmptyState v-else-if="!items.length" title="还没有线路" description="新增第一条线路后即可进行健康探测与维护管理。" />
    <section v-else class="line-grid">
      <article v-for="line in items" :key="line.id" class="panel line-card">
        <header>
          <span class="line-icon"><CloudCog :size="21" /></span>
          <div><span class="section-kicker">ROUTE · {{ line.id }}</span><h2>{{ line.name }}</h2></div>
          <StatusBadge :label="label(line)" :tone="tone(line)" />
        </header>
        <a :href="line.base_url" target="_blank" rel="noreferrer" class="line-url">{{ line.base_url }}</a>
        <dl class="line-metrics">
          <div><dt><Gauge :size="15" /> 延迟</dt><dd>{{ line.last_latency_ms === null ? "—" : `${line.last_latency_ms} ms` }}</dd></div>
          <div><dt><MapPin :size="15" /> 地区</dt><dd>{{ line.region || "未设置" }}</dd></div>
          <div><dt><Route :size="15" /> 运营商</dt><dd>{{ line.carrier || "未设置" }}</dd></div>
          <div><dt><Activity :size="15" /> 权重 / 范围</dt><dd>{{ line.weight }} · {{ line.audience === "whitelist" ? "白名单" : "普通" }}</dd></div>
        </dl>
        <p v-if="line.last_error" class="line-error">{{ line.last_error }}</p>
        <footer>
          <small>上次探测 {{ formatDate(line.last_checked_at, "尚未探测") }}</small>
          <div v-if="canUpdate">
            <button class="icon-button" title="编辑" @click="openEditor(line)"><Pencil :size="16" /></button>
            <button class="secondary-button compact-action" :disabled="busyId === line.id" @click="toggleMaintenance(line)"><Wrench :size="15" /> {{ line.maintenance ? "结束维护" : "维护" }}</button>
            <button class="primary-button compact-action" :disabled="busyId === line.id" @click="probe(line)"><RefreshCw :size="15" :class="{ spin: busyId === line.id }" /> 探测</button>
          </div>
        </footer>
      </article>
    </section>

    <div v-if="modalOpen" class="modal-layer">
      <form class="modal-card" @submit.prevent="save">
        <header><div><span class="section-kicker">LINE CONFIGURATION</span><h2>{{ editing ? "编辑线路" : "新增线路" }}</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header>
        <div class="form-grid">
          <label><span>线路名称</span><input v-model.trim="form.name" required minlength="2" maxlength="100" placeholder="例如：香港 BGP" /></label>
          <label><span>访问地址</span><input v-model.trim="form.base_url" required type="url" placeholder="https://emby.example.com" /></label>
          <label><span>地区</span><input v-model.trim="form.region" maxlength="100" placeholder="香港" /></label>
          <label><span>运营商</span><input v-model.trim="form.carrier" maxlength="100" placeholder="BGP / 联通 / 移动" /></label>
          <label><span>适用用户</span><select v-model="form.audience"><option value="all">普通线路</option><option value="whitelist">白名单专属</option></select></label>
          <label><span>流量权重</span><input v-model.number="form.weight" type="number" min="0" max="1000" /></label>
          <label><span>显示顺序</span><input v-model.number="form.sort_order" type="number" min="0" max="10000" /></label>
        </div>
        <label class="check-row"><input v-model="form.enabled" type="checkbox" /><span>启用该线路</span></label>
        <label class="check-row"><input v-model="form.maintenance" type="checkbox" /><span>标记为维护中</span></label>
        <p v-if="error" class="form-error">{{ error }}</p>
        <footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button">保存线路</button></footer>
      </form>
    </div>
  </div>
</template>
