<template>
  <div class="rounded border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800">
    <div class="mb-2 flex items-center gap-2">
      <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
      <input
        v-model="searchText"
        type="text"
        class="min-w-0 flex-1 bg-transparent text-sm text-gray-900 outline-none dark:text-gray-100"
        :placeholder="t('admin.proxies.searchProxies')"
      />
      <span class="shrink-0 text-xs text-gray-500">{{ modelValue.length }}/{{ max }}</span>
    </div>
    <div class="grid max-h-36 grid-cols-1 gap-1 overflow-y-auto sm:grid-cols-2">
      <label
        v-for="proxy in filteredProxies"
        :key="proxy.id"
        class="flex cursor-pointer items-start gap-2 rounded px-2 py-1.5 hover:bg-white dark:hover:bg-dark-700"
      >
        <input
          type="checkbox"
          class="mt-0.5 h-3.5 w-3.5 rounded border-gray-300 text-primary-500 focus:ring-primary-500"
          :checked="modelValue.includes(proxy.id)"
          :disabled="!modelValue.includes(proxy.id) && modelValue.length >= max"
          @change="toggleProxy(proxy.id, ($event.target as HTMLInputElement).checked)"
        />
        <span class="min-w-0">
          <span class="block truncate text-sm font-medium text-gray-800 dark:text-gray-200">{{ proxy.name }}</span>
          <span class="block truncate text-xs text-gray-500">{{ proxy.host }}:{{ proxy.port }}</span>
        </span>
      </label>
      <div v-if="filteredProxies.length === 0" class="py-2 text-center text-sm text-gray-500 sm:col-span-2">
        {{ t('common.noOptionsFound') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { Proxy } from '@/types'

const { t } = useI18n()
const searchText = ref('')

const props = withDefaults(defineProps<{
  modelValue: number[]
  proxies: Proxy[]
  max?: number
}>(), { max: 5 })

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const filteredProxies = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  const active = props.proxies.filter((proxy) => proxy.status === 'active')
  if (!query) return active
  return active.filter((proxy) =>
    proxy.name.toLowerCase().includes(query) || proxy.host.toLowerCase().includes(query)
  )
})

const toggleProxy = (proxyID: number, checked: boolean) => {
  if (checked) {
    if (props.modelValue.includes(proxyID) || props.modelValue.length >= props.max) return
    emit('update:modelValue', [...props.modelValue, proxyID])
    return
  }
  emit('update:modelValue', props.modelValue.filter((id) => id !== proxyID))
}
</script>
