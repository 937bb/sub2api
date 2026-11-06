<template>
  <div class="mt-3 space-y-2">
    <label class="input-label">{{ t('admin.accounts.fallbackProxies.title') }}</label>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.fallbackProxies.hint') }}</p>
    <p v-if="!primaryId" class="text-sm text-amber-600">{{ t('admin.accounts.fallbackProxies.selectPrimary') }}</p>
    <div v-if="primaryId" class="flex flex-wrap gap-2" aria-live="polite">
      <span class="rounded-lg bg-primary-50 px-2 py-1 text-xs text-primary-700">1 · {{ label(primaryId) }}</span>
      <span v-for="(id, index) in modelValue" :key="id" class="inline-flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">
        {{ index + 2 }} · {{ label(id) }}
        <button type="button" :disabled="index === 0" :aria-label="t('admin.accounts.fallbackProxies.moveUp')" class="px-1 disabled:opacity-30" @click="moveUp(index)">↑</button>
        <button type="button" :aria-label="`${t('common.delete')} ${label(id)}`" class="px-1" @click="remove(index)">×</button>
      </span>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <select v-model="usageType" class="input w-auto" :aria-label="t('admin.proxies.usageType')">
        <option value="">{{ t('admin.proxies.allUsageTypes') }}</option>
        <option v-for="kind in PROXY_USAGE_TYPES" :key="kind" :value="kind">{{ t(`admin.proxies.usageTypes.${kind}`) }}</option>
      </select>
      <input v-model="search" type="search" class="input min-w-0 flex-1" :placeholder="t('admin.proxies.searchProxies')" :aria-label="t('admin.proxies.searchProxies')" />
      <span class="shrink-0 text-xs text-gray-500">{{ modelValue.length + (primaryId ? 1 : 0) }} / 8</span>
    </div>
    <div class="max-h-52 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
      <label v-for="proxy in filtered" :key="proxy.id" class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-3 py-2 last:border-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700">
        <input type="checkbox" :checked="proxy.id === primaryId || modelValue.includes(proxy.id)" :disabled="disabled(proxy)" :aria-label="proxy.name" @change="toggle(proxy.id)" />
        <span class="min-w-0 flex-1"><span class="block truncate text-sm">{{ proxy.name }} · {{ t(`admin.proxies.usageTypes.${proxyUsageType(proxy)}`) }}</span><span class="block truncate text-xs text-gray-500">{{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}</span></span>
        <span v-if="!active(proxy)" class="text-xs text-amber-600">{{ t('admin.accounts.fallbackProxies.unavailable') }}</span>
      </label>
      <p v-if="!filtered.length" class="p-3 text-sm text-gray-500">{{ t('admin.accounts.fallbackProxies.empty') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Proxy, ProxyUsageType } from '@/types'
import { PROXY_USAGE_TYPES, proxyUsageType, filterProxiesByUsage } from '@/utils/proxyUsageType'

const props = defineProps<{ modelValue: number[]; primaryId: number | null; proxies: Proxy[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: number[]] }>()
const { t } = useI18n()
const search = ref('')
const usageType = ref<ProxyUsageType | ''>('')
const filtered = computed(() => filterProxiesByUsage(props.proxies, usageType.value, search.value))
const label = (id: number) => props.proxies.find(p => p.id === id)?.name || `#${id}`
const active = (p: Proxy) => p.status === 'active' && (!p.expires_at || new Date(p.expires_at).getTime() > Date.now())
const disabled = (p: Proxy) => p.id === props.primaryId || !props.primaryId || (!props.modelValue.includes(p.id) && (!active(p) || props.modelValue.length >= 7))
function toggle(id: number) {
  if (props.modelValue.includes(id)) emit('update:modelValue', props.modelValue.filter(value => value !== id))
  else {
    const proxy = props.proxies.find(p => p.id === id)
    if (proxy && !disabled(proxy)) emit('update:modelValue', [...props.modelValue, id])
  }
}
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
watch(() => props.primaryId, id => {
  const ids = id ? props.modelValue.filter(value => value !== id) : []
  if (ids.length !== props.modelValue.length) emit('update:modelValue', ids)
})
</script>
