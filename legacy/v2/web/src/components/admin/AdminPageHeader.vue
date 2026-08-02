<script setup lang="ts">
import type { Component } from "vue";

defineProps<{
  eyebrow?: string;
  title: string;
  description?: string;
  icon?: Component;
}>();
</script>

<template>
  <header class="admin-page-header">
    <div class="admin-page-copy">
      <span v-if="eyebrow" class="eyebrow">{{ eyebrow }}</span>
      <div class="admin-page-title">
        <span v-if="icon" class="admin-page-icon"><component :is="icon" :size="20" /></span>
        <h1>{{ title }}</h1>
      </div>
      <p v-if="description">{{ description }}</p>
    </div>
    <div v-if="$slots.meta || $slots.actions" class="admin-page-actions">
      <slot name="meta" />
      <slot name="actions" />
    </div>
  </header>
</template>

