<template>
  <div class="mt-3 space-y-2">
    <label class="input-label">{{ t('admin.accounts.fallbackProxies.title') }}</label>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.fallbackProxies.hint') }}</p>
    <div v-for="(id, index) in modelValue" :key="index" class="flex items-center gap-2">
      <span class="text-xs text-gray-500">{{ index + 1 }}</span>
      <div class="min-w-0 flex-1">
        <ProxySelector :model-value="id" :proxies="availableFor(id)" @update:model-value="replace(index, $event)" />
      </div>
      <button type="button" class="btn btn-secondary px-2" :aria-label="t('admin.accounts.fallbackProxies.moveUp')" :disabled="index === 0" @click="moveUp(index)">↑</button>
      <button type="button" class="btn btn-secondary px-2" :aria-label="t('common.delete')" @click="remove(index)">×</button>
    </div>
    <button type="button" class="btn btn-secondary text-sm" :disabled="!primaryId || modelValue.length >= 7 || availableFor(null).length === 0" @click="add">
      {{ t('admin.accounts.fallbackProxies.add') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ProxySelector from '@/components/common/ProxySelector.vue'
import type { Proxy } from '@/types'

const props = defineProps<{ modelValue: number[]; primaryId: number | null; proxies: Proxy[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: number[]] }>()
const { t } = useI18n()
const availableFor = (id: number | null) => props.proxies.filter(p => p.id === id || (
  p.id !== props.primaryId && !props.modelValue.includes(p.id) && p.status === 'active' &&
  (!p.expires_at || new Date(p.expires_at).getTime() > Date.now())
))
function replace(index: number, id: number | null) {
  const ids = [...props.modelValue]
  if (id === null) ids.splice(index, 1)
  else ids[index] = id
  emit('update:modelValue', ids)
}
function remove(index: number) { replace(index, null) }
function moveUp(index: number) {
  const ids = [...props.modelValue]
  ;[ids[index - 1], ids[index]] = [ids[index], ids[index - 1]]
  emit('update:modelValue', ids)
}
function add() {
  const first = availableFor(null)[0]
  if (first && props.primaryId && props.modelValue.length < 7) emit('update:modelValue', [...props.modelValue, first.id])
}
watch(() => props.primaryId, id => {
  const ids = id ? props.modelValue.filter(value => value !== id) : []
  if (ids.length !== props.modelValue.length) emit('update:modelValue', ids)
})
</script>
