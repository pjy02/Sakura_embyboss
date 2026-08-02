<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Archive, DatabaseBackup, Download, FileCheck2, RefreshCw, ShieldCheck } from "lucide-vue-next";
import AdminDataTable from "@/components/admin/AdminDataTable.vue";
import AdminPageHeader from "@/components/admin/AdminPageHeader.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import { api } from "@/lib/api";
import { formatDate, formatFileSize, formatNumber } from "@/lib/format";
import { runtime } from "@/lib/runtime";
import { useSessionStore } from "@/stores/session";
import type { BackupArtifact } from "@/types";

const session = useSessionStore();
const items = ref<BackupArtifact[]>([]);
const totalSize = ref(0);
const directory = ref("");
const loading = ref(true);
const busy = ref(false);
const canCreate = computed(() => session.session?.permissions.some((item) => item === "*" || item === "backups:*" || item === "backups:manage"));

async function load() {
  loading.value = true;
  try {
    const result = await api<{ items: BackupArtifact[]; total_size: number; directory: string }>("/admin/backups");
    items.value = result.items;
    totalSize.value = result.total_size;
    directory.value = result.directory;
  } finally { loading.value = false; }
}

async function createBackup() {
  busy.value = true;
  try {
    await api("/admin/backups", { method: "POST" });
    window.setTimeout(load, 2500);
  } finally { busy.value = false; }
}

function downloadUrl(item: BackupArtifact) { return `${runtime.apiBase}/admin/backups/${encodeURIComponent(item.name)}/download`; }
onMounted(load);
</script>

<template>
  <div class="page-stack">
    <AdminPageHeader eyebrow="BACKUP & RECOVERY" title="备份与恢复中心" description="查看数据库备份、校验摘要并按需下载离线保存。恢复操作保留为人工维护步骤，避免网页误操作覆盖生产数据。" :icon="DatabaseBackup"><template #actions><button class="secondary-button" @click="load"><RefreshCw :size="16" />刷新</button><button v-if="canCreate" class="primary-button" :disabled="busy" @click="createBackup"><DatabaseBackup :size="16" />{{ busy ? '已加入队列' : '立即备份' }}</button></template></AdminPageHeader>
    <div class="metric-grid compact-grid"><MetricCard label="备份文件" :value="formatNumber(items.length)" caption="当前保留的 SQL 文件" :icon="Archive" /><MetricCard label="占用空间" :value="formatFileSize(totalSize)" caption="备份目录总大小" :icon="DatabaseBackup" tone="cyan" /><MetricCard label="完整性校验" :value="items.length ? 'SHA-256' : '等待备份'" caption="下载前可核对摘要" :icon="ShieldCheck" tone="green" /></div>
    <section class="panel recovery-note"><FileCheck2 :size="22" /><div><strong>上线前建议</strong><p>保留一份数据库备份和当前 <code>config.json</code>，下载到宿主机之外；镜像回退不会自动降级数据库。</p><small>{{ directory }}</small></div></section>
    <section class="panel table-panel"><AdminDataTable :loading="loading" :empty="!items.length" empty-title="暂无数据库备份" empty-description="点击“立即备份”，任务将交给独立 Worker 执行。" min-width="780px"><template #head><tr><th>文件</th><th>大小</th><th>创建时间</th><th>SHA-256</th><th>操作</th></tr></template><template #body><tr v-for="item in items" :key="item.name"><td><div class="table-primary"><strong>{{ item.name }}</strong><small>MySQL SQL Dump</small></div></td><td>{{ formatFileSize(item.size) }}</td><td>{{ formatDate(item.created_at) }}</td><td><code :title="item.sha256">{{ item.sha256.slice(0, 16) }}…</code></td><td><a class="text-button" :href="downloadUrl(item)"><Download :size="15" />下载</a></td></tr></template></AdminDataTable></section>
  </div>
</template>

<style scoped>
.recovery-note{display:flex;gap:14px;align-items:flex-start;padding:20px}.recovery-note>svg{color:var(--primary)}.recovery-note p{margin:5px 0;color:var(--text-muted)}.recovery-note small{color:var(--text-subtle)}code{font-size:12px;color:var(--primary)}
</style>
