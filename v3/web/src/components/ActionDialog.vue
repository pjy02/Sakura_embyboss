<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { CheckCircle2, Clipboard, X } from 'lucide-vue-next'
import type { PageAction } from '../resource-config'
import { api, idempotencyKey } from '../lib/api'

const props = defineProps<{ action: PageAction | null; seed?: Record<string, unknown> }>()
const emit = defineEmits<{ close: []; completed: [] }>()
// Widgets are presentation metadata for API operations; validation and all
// business decisions remain in the shared service called by the API.
const values = reactive<Record<string, any>>({})
const busy = ref(false)
const error = ref('')
const done = ref(false)
const result = ref<unknown>(null)
const resultText = computed(() => result.value == null ? '' : JSON.stringify(result.value, null, 2))

watch(() => [props.action, props.seed] as const, ([action, seed]) => {
  Object.keys(values).forEach((key) => delete values[key])
  for (const field of action?.fields || []) {
    const initial = seed?.[field.key] ?? action?.defaults?.[field.key]
    values[field.key] = typeof initial === 'string' || typeof initial === 'number' || typeof initial === 'boolean'
      ? initial
      : field.type === 'json' && initial !== undefined
        ? JSON.stringify(initial, null, 2)
        : field.type === 'checkbox' ? false : ''
  }
  error.value = ''
  done.value = false
  result.value = null
}, { immediate: true, deep: true })

async function submit() {
  if (!props.action) return
  busy.value = true
  error.value = ''
  try {
    const body: Record<string, unknown> = { ...props.action.defaults }
    const path: Record<string, string | number> = {}
    for (const field of props.action.fields) {
      if (!field.required && field.type !== 'checkbox' && String(values[field.key] ?? '').trim() === '') {
        if (!field.path) delete body[field.key]
        continue
      }
      let value: unknown = field.type === 'number' ? Number(values[field.key] || 0) : values[field.key]
      if (field.type === 'json') value = JSON.parse(String(value || 'null'))
      if (field.path) path[field.key] = String(value || '')
      else body[field.key] = value ?? (field.type === 'checkbox' ? false : '')
    }
    if (props.action.idempotent) body.idempotency_key = idempotencyKey(props.action.operation)
    result.value = await api.call(props.action.operation, { body, path })
    done.value = true
    emit('completed')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败'
  } finally {
    busy.value = false
  }
}

async function copyResult() {
  await window.navigator.clipboard.writeText(resultText.value)
}
</script>

<template>
  <Transition name="modal">
    <div v-if="action" class="modal-layer" @mousedown.self="$emit('close')">
      <form class="action-dialog" @submit.prevent="submit">
        <header>
          <div><p class="eyebrow">SHARED BUSINESS ACTION</p><h2>{{ action.title }}</h2></div>
          <button type="button" class="icon-button" @click="$emit('close')"><X /></button>
        </header>
        <div v-if="done" class="success-state">
          <CheckCircle2 /><h3>操作已提交</h3>
          <p>Web 与 Bot 会看到同一份处理结果，异步进度将在实时任务中更新。</p>
          <div v-if="resultText" class="result-output"><pre>{{ resultText }}</pre><button type="button" class="secondary-button" @click="copyResult"><Clipboard :size="15" />复制结果</button></div>
          <button type="button" class="primary-button" @click="$emit('close')">完成</button>
        </div>
        <template v-else>
          <div class="form-grid">
            <label v-for="field in action.fields" :key="field.key" :class="{ wide: field.type === 'textarea' || field.type === 'json' }">
              <template v-if="field.type === 'checkbox'"><span class="check-label"><input v-model="values[field.key]" type="checkbox" />{{ field.label }}</span></template>
              <template v-else>
                {{ field.label }}
                <textarea v-if="field.type === 'textarea' || field.type === 'json'" v-model="values[field.key]" :required="field.required" :placeholder="field.placeholder" />
                <select v-else-if="field.type === 'select'" v-model="values[field.key]" :required="field.required"><option v-for="option in field.options" :key="option.value" :value="option.value">{{ option.label }}</option></select>
                <input v-else v-model="values[field.key]" :type="field.type || 'text'" :required="field.required" :placeholder="field.placeholder" />
              </template>
            </label>
          </div>
          <p v-if="error" class="form-error">{{ error }}</p>
          <footer><button type="button" class="text-button" @click="$emit('close')">取消</button><button class="primary-button" :disabled="busy">{{ busy ? '正在提交…' : '确认提交' }}</button></footer>
        </template>
      </form>
    </div>
  </Transition>
</template>
