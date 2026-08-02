<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { CirclePlus, HeartHandshake, MessageCircle, Send, X } from "lucide-vue-next";
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";
import StatusBadge from "@/components/admin/StatusBadge.vue";
import { useRealtimeEvents } from "@/composables/useRealtime";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { SupportTicket } from "@/types";

const items = ref<SupportTicket[]>([]);
const selected = ref<SupportTicket | null>(null);
const loading = ref(true);
const modalOpen = ref(false);
const reply = ref("");
const busy = ref(false);
const form = reactive({ subject: "", category: "general", priority: "normal", body: "" });
function label(value: string) { return ({ open: "新工单", pending_user: "等待你的回复", pending_staff: "等待管理员", resolved: "已解决", closed: "已关闭" } as Record<string, string>)[value] || value; }
function tone(value: string): "info" | "warning" | "success" | "muted" { return value === "resolved" ? "success" : value === "closed" ? "muted" : value === "pending_user" ? "warning" : "info"; }
async function load() { loading.value = true; try { items.value = (await api<{ items: SupportTicket[] }>("/me/tickets?limit=100")).items; } finally { loading.value = false; } }
async function open(ticket: SupportTicket) { selected.value = await api<SupportTicket>(`/me/tickets/${ticket.id}`); }
async function createTicket() { busy.value = true; try { const created = await api<SupportTicket>("/me/tickets", { method: "POST", body: JSON.stringify(form) }); modalOpen.value = false; Object.assign(form, { subject: "", category: "general", priority: "normal", body: "" }); await load(); await open(created); } finally { busy.value = false; } }
async function sendReply() { if (!selected.value || !reply.value) return; busy.value = true; try { await api(`/me/tickets/${selected.value.id}/messages`, { method: "POST", body: JSON.stringify({ body: reply.value }) }); reply.value = ""; await open(selected.value); await load(); } finally { busy.value = false; } }
async function syncTickets() { await load(); if (selected.value) await open(selected.value); }
onMounted(load);
useRealtimeEvents(["ticket.created", "ticket.updated"], () => syncTickets());
</script>
<template>
  <div class="page-stack">
    <header class="page-heading"><div><span class="eyebrow">SUPPORT CENTER</span><h1>我的工单</h1><p>提交账户、播放、充值、求片或技术问题，并在这里查看管理员回复。</p></div><button class="primary-button" @click="modalOpen = true"><CirclePlus :size="17" /> 新建工单</button></header>
    <LoadingBlock v-if="loading" />
    <EmptyState v-else-if="!items.length" title="还没有工单" description="遇到问题时可以创建工单与管理员沟通。" />
    <section v-else class="portal-ticket-layout">
      <div class="panel portal-ticket-list"><button v-for="ticket in items" :key="ticket.id" :class="{ active: selected?.id === ticket.id }" @click="open(ticket)"><span><HeartHandshake :size="18" /></span><div><strong>{{ ticket.subject }}</strong><small>{{ ticket.ticket_no }} · {{ formatDate(ticket.updated_at) }}</small></div><StatusBadge :label="label(ticket.status)" :tone="tone(ticket.status)" /></button></div>
      <article class="panel portal-ticket-thread">
        <EmptyState v-if="!selected" title="选择一个工单" description="右侧将显示完整对话。" />
        <template v-else><header><div><span class="section-kicker">{{ selected.ticket_no }}</span><h2>{{ selected.subject }}</h2></div><StatusBadge :label="label(selected.status)" :tone="tone(selected.status)" /></header><div class="ticket-thread"><article v-for="message in selected.messages" :key="message.id" :data-sender="message.sender_kind"><header><strong>{{ message.sender_kind === "admin" ? "管理员" : "我" }}</strong><small>{{ formatDate(message.created_at) }}</small></header><p>{{ message.body }}</p></article></div><form v-if="selected.status !== 'closed'" class="ticket-reply" @submit.prevent="sendReply"><textarea v-model.trim="reply" required maxlength="5000" placeholder="回复工单…" /><button class="primary-button" :disabled="busy"><Send :size="16" /> 发送回复</button></form></template>
      </article>
    </section>
    <div v-if="modalOpen" class="modal-layer"><form class="modal-card" @submit.prevent="createTicket"><header><div><span class="section-kicker">NEW TICKET</span><h2>新建工单</h2></div><button type="button" class="icon-button" @click="modalOpen = false"><X :size="19" /></button></header><label><span>主题</span><input v-model.trim="form.subject" required minlength="3" maxlength="200" /></label><div class="form-grid"><label><span>问题分类</span><select v-model="form.category"><option value="account">账户</option><option value="playback">播放</option><option value="billing">充值</option><option value="request">求片</option><option value="technical">技术</option><option value="general">其他</option></select></label><label><span>优先级</span><select v-model="form.priority"><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></label></div><label><span>问题描述</span><textarea v-model.trim="form.body" required minlength="2" maxlength="5000" /></label><footer><button type="button" class="secondary-button" @click="modalOpen = false">取消</button><button class="primary-button" :disabled="busy"><MessageCircle :size="16" /> 提交工单</button></footer></form></div>
  </div>
</template>
