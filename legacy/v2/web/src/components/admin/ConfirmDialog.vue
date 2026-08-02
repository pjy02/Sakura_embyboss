<script setup lang="ts">
import { CircleAlert, X } from "lucide-vue-next";

withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description: string;
    confirmLabel?: string;
    cancelLabel?: string;
    tone?: "normal" | "warning" | "danger";
    busy?: boolean;
  }>(),
  {
    confirmLabel: "确认",
    cancelLabel: "取消",
    tone: "normal",
    busy: false,
  },
);

const emit = defineEmits<{ confirm: []; close: [] }>();
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal-layer" role="presentation" @click.self="emit('close')">
      <section class="admin-confirm-dialog" role="dialog" aria-modal="true" :aria-label="title" :data-tone="tone">
        <header>
          <span><CircleAlert :size="21" /></span>
          <button class="icon-button" type="button" aria-label="关闭" @click="emit('close')"><X :size="19" /></button>
        </header>
        <h2>{{ title }}</h2>
        <p>{{ description }}</p>
        <div v-if="$slots.default" class="admin-confirm-context"><slot /></div>
        <footer>
          <button class="secondary-button" type="button" :disabled="busy" @click="emit('close')">{{ cancelLabel }}</button>
          <button :class="tone === 'danger' ? 'danger-button' : 'primary-button'" type="button" :disabled="busy" @click="emit('confirm')">
            {{ busy ? "正在处理…" : confirmLabel }}
          </button>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

