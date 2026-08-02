<script setup lang="ts">
import EmptyState from "@/components/EmptyState.vue";
import LoadingBlock from "@/components/LoadingBlock.vue";

withDefaults(
  defineProps<{
    loading?: boolean;
    empty?: boolean;
    emptyTitle?: string;
    emptyDescription?: string;
    minWidth?: string;
  }>(),
  {
    loading: false,
    empty: false,
    emptyTitle: "暂无数据",
    emptyDescription: "",
    minWidth: "780px",
  },
);
</script>

<template>
  <div class="admin-data-table-shell">
    <LoadingBlock v-if="loading" />
    <slot v-else-if="empty" name="empty">
      <EmptyState :title="emptyTitle" :description="emptyDescription" />
    </slot>
    <div v-else class="responsive-table admin-data-table">
      <table :style="{ minWidth }">
        <thead><slot name="head" /></thead>
        <tbody><slot name="body" /></tbody>
      </table>
    </div>
    <slot name="footer" />
  </div>
</template>
