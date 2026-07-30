<script setup lang="ts">
defineProps<{ compact?: boolean }>();
</script>

<template>
  <div class="brand-mark" :class="{ compact }">
    <span class="brand-glyph" aria-hidden="true">桜</span>
    <span v-if="!compact" class="brand-copy">
      <strong>SAKURA</strong>
      <small>EMBYBOSS</small>
    </span>
  </div>
</template>
