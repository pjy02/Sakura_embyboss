<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ChevronLeft, ChevronRight, FileClock, RefreshCw } from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import { api } from "@/lib/api";
import { actionLabel, formatDate } from "@/lib/format";
import type { AuditLog } from "@/types";

const items = ref<AuditLog[]>([]);
const loading = ref(true);
const offset = ref(0);
const limit = 30;
const canNext = computed(() => items.value.length === limit);

async function load() {
  loading.value = true;
  try {
    items.value = (await api<{ items: AuditLog[] }>(`/admin/audit?limit=${limit}&offset=${offset.value}`)).items;
  } finally {
    loading.value = false;
  }
}
function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit);
  load();
}
onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader
      eyebrow="AUDIT TRAIL"
      title="操作记录"
      description="集中追踪登录、权限和业务数据变更，便于回溯每一次敏感操作。"
      :icon="FileClock"
    >
      <template #actions><button class="secondary-button" @click="load"><RefreshCw :size="16" /> 刷新</button></template>
    </AdminPageHeader>
    <section class="panel table-panel">
      <div class="panel-heading"><div><span class="section-kicker">SECURITY RECORDS</span><h2>操作记录</h2></div><span class="page-count">第 {{ Math.floor(offset / limit) + 1 }} 页</span></div>
      <LoadingBlock v-if="loading" />
      <EmptyState v-else-if="!items.length" title="暂无审计记录" />
      <div v-else class="audit-timeline">
        <article v-for="item in items" :key="item.id">
          <span class="timeline-icon"><FileClock :size="17" /></span>
          <div class="timeline-main">
            <div><strong>{{ actionLabel(item.action) }}</strong><span class="status-badge" :class="item.outcome === 'success' ? 'active' : 'danger'">{{ item.outcome }}</span></div>
            <p>{{ item.actor_kind }} · {{ item.actor_id }} 对 {{ item.resource_type }} {{ item.resource_id || "" }} 执行操作</p>
            <small>{{ formatDate(item.created_at) }}<template v-if="item.ip_address"> · IP {{ item.ip_address }}</template></small>
          </div>
          <code>#{{ item.id }}</code>
        </article>
      </div>
      <div class="pagination"><button :disabled="offset === 0" @click="page(-1)"><ChevronLeft :size="16" /> 上一页</button><button :disabled="!canNext" @click="page(1)">下一页 <ChevronRight :size="16" /></button></div>
    </section>
  </div>
</template>
