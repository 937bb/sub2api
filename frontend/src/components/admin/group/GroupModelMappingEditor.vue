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
      <button type="button" class="btn btn-secondary shrink-0" @click="addRow">
        <Icon name="plus" size="sm" class="mr-1" />
        {{ t('admin.groups.openaiModelMapping.add') }}
      </button>
    </div>
    <div v-if="modelValue.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-5 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400">
      {{ t('admin.groups.openaiModelMapping.empty') }}
    </div>
    <div v-for="(row, index) in modelValue" :key="index" class="grid gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] md:items-end">
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

const props = defineProps<{ modelValue: GroupModelMappingRow[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: GroupModelMappingRow[]] }>()
const { t } = useI18n()
const addRow = () => emit('update:modelValue', [...props.modelValue, { source: '', target: '' }])
const removeRow = (index: number) => emit('update:modelValue', props.modelValue.filter((_, rowIndex) => rowIndex !== index))
</script>
