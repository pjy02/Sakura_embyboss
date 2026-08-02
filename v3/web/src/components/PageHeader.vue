<script setup lang="ts">
import { RefreshCw } from 'lucide-vue-next'
defineProps<{ eyebrow: string; title: string; description: string; busy?: boolean }>()
defineEmits<{ refresh: [] }>()
</script>
<template><header class="page-header"><div><p class="eyebrow">{{ eyebrow }}</p><h1>{{ title }}</h1><p>{{ description }}</p></div><div class="page-actions"><slot /><button class="secondary-button" :disabled="busy" @click="$emit('refresh')"><RefreshCw :size="16" :class="{ spin: busy }" />刷新</button></div></header></template>
