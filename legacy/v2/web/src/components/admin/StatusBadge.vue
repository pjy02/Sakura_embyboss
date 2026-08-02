<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string;
    tone?: "success" | "warning" | "danger" | "info" | "muted";
    dot?: boolean;
  }>(),
  {
    tone: "muted",
    dot: true,
  },
);
</script>

<template>
  <span class="admin-status-badge" :data-tone="tone">
    <i v-if="dot" />
    {{ label }}
  </span>
</template>

