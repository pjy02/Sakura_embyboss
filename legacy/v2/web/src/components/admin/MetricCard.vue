<script setup lang="ts">
import type { Component } from "vue";

withDefaults(
  defineProps<{
    label: string;
    value: string | number;
    caption?: string;
    icon?: Component;
    tone?: "pink" | "cyan" | "gold" | "violet" | "green" | "red";
    featured?: boolean;
  }>(),
  {
    caption: "",
    tone: "pink",
    featured: false,
  },
);
</script>

<template>
  <article class="metric-card" :class="{ featured }" :data-tone="tone">
    <span v-if="icon" class="metric-icon"><component :is="icon" :size="21" /></span>
    <div>
      <small>{{ label }}</small>
      <strong>{{ value }}</strong>
      <p v-if="caption">{{ caption }}</p>
    </div>
    <slot />
  </article>
</template>

