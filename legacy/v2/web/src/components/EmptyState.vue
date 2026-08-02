<script setup lang="ts">
import { Inbox } from "lucide-vue-next";

defineProps<{ title?: string; description?: string }>();
</script>

<template>
  <div class="empty-state">
    <span class="empty-icon"><Inbox :size="22" /></span>
    <strong>{{ title || "暂时没有数据" }}</strong>
    <p>{{ description || "新的记录出现后会显示在这里。" }}</p>
  </div>
</template>
