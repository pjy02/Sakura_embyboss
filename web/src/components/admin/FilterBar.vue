<template>
  <div class="admin-filter-bar">
    <div class="admin-filter-primary"><slot /></div>
    <div v-if="$slots.actions" class="admin-filter-actions"><slot name="actions" /></div>
  </div>
</template>

