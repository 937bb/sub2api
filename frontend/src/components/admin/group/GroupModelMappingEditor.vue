<template>
  <div class="space-y-3">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.groups.openaiModelMapping.title') }}
        </h4>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.openaiModelMapping.hint') }}
        </p>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <button
          type="button"
          role="switch"
          class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="props.enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'"
          :aria-checked="props.enabled"
          :aria-label="t('admin.groups.openaiModelMapping.toggle')"
          :title="t('admin.groups.openaiModelMapping.toggle')"
          @click="emit('update:enabled', !props.enabled)"
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="props.enabled ? 'translate-x-6' : 'translate-x-1'"
          />
        </button>
        <button v-if="props.enabled" type="button" class="btn btn-secondary" @click="addRow">
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.groups.openaiModelMapping.add') }}
        </button>
      </div>
    </div>
    <div v-if="props.enabled && modelValue.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-5 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400">
      {{ t('admin.groups.openaiModelMapping.empty') }}
    </div>
    <div v-for="(row, index) in (props.enabled ? modelValue : [])" :key="index" class="grid gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] md:items-end">
      <div>
        <label class="input-label">{{ t('admin.groups.openaiModelMapping.requested') }}</label>
        <input v-model="row.source" class="input" :placeholder="t('admin.groups.openaiModelMapping.requestedPlaceholder')" />
      </div>
      <span class="hidden pb-2 text-center text-gray-400 md:block">-&gt;</span>
      <div>
        <label class="input-label">{{ t('admin.groups.openaiModelMapping.upstream') }}</label>
        <input v-model="row.target" class="input" :placeholder="t('admin.groups.openaiModelMapping.upstreamPlaceholder')" />
      </div>
      <button type="button" class="btn btn-ghost text-red-500" :title="t('admin.groups.openaiModelMapping.remove')" @click="removeRow(index)">
        <Icon name="trash" size="sm" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

export interface GroupModelMappingRow {
  source: string
  target: string
}

const props = withDefaults(defineProps<{
  modelValue: GroupModelMappingRow[]
  enabled?: boolean
}>(), {
  enabled: false,
})
const emit = defineEmits<{
  'update:modelValue': [value: GroupModelMappingRow[]]
  'update:enabled': [value: boolean]
}>()
const { t } = useI18n()
const addRow = () => emit('update:modelValue', [...props.modelValue, { source: '', target: '' }])
const removeRow = (index: number) => emit('update:modelValue', props.modelValue.filter((_, rowIndex) => rowIndex !== index))
</script>
