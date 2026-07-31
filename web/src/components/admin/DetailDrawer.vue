<script setup lang="ts">
import { X } from "lucide-vue-next";

defineProps<{
  open: boolean;
  title: string;
  eyebrow?: string;
  description?: string;
}>();

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <Teleport to="body">
    <button
      v-if="open"
      class="drawer-backdrop"
      type="button"
      aria-label="关闭详情"
      @click="emit('close')"
    />
    <aside class="admin-detail-drawer" :class="{ open }" :aria-hidden="!open">
      <header>
        <div>
          <span v-if="eyebrow" class="section-kicker">{{ eyebrow }}</span>
          <h2>{{ title }}</h2>
          <p v-if="description">{{ description }}</p>
        </div>
        <button class="icon-button" type="button" aria-label="关闭详情" @click="emit('close')">
          <X :size="20" />
        </button>
      </header>
      <div class="admin-drawer-content"><slot /></div>
      <footer v-if="$slots.actions"><slot name="actions" /></footer>
    </aside>
  </Teleport>
</template>

