<template>
  <div
    :class="[
      'grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center',
      compact ? 'gap-x-2' : 'gap-x-2.5'
    ]"
    data-test="codex-state-status"
  >
    <span
      :class="[
        'relative row-span-2 flex shrink-0 items-center justify-center rounded-full ring-1 ring-inset',
        compact ? 'h-6 w-6' : 'h-8 w-8',
        tone.iconBackground
      ]"
      aria-hidden="true"
    >
      <span
        :class="[
          'rounded-full',
          compact ? 'h-2 w-2' : 'h-2.5 w-2.5',
          tone.dot,
          animated ? 'animate-pulse' : ''
        ]"
      />
    </span>

    <div class="flex min-w-0 items-center gap-1.5">
      <div :class="['truncate font-semibold leading-4', compact ? 'text-xs' : 'text-sm', tone.text]">
        {{ label }}
      </div>
      <span
        v-if="showDetails && stateLength > 0"
        class="inline-flex h-4 shrink-0 items-center rounded border border-gray-200 bg-white px-1 font-mono text-[10px] font-semibold leading-none text-gray-600 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300"
      >
          {{ stateLength }}
      </span>
    </div>
    <div v-if="showDetails" class="mt-0.5 min-w-0 text-[11px] leading-4 text-gray-400 dark:text-dark-400">
      <span v-if="model" class="block max-w-36 truncate font-mono" :title="model">{{ model }}</span>
      <span v-else class="block">State</span>
      <span v-if="targetLengths.length" class="block font-mono" data-test="codex-state-target">{{ t('admin.ops.turnState.targetLengths', { lengths: targetLengths.join(' / ') }) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CodexTurnStateAccountStatusValue } from '@/api/admin/ops'

const props = withDefaults(defineProps<{
  status?: CodexTurnStateAccountStatusValue
  stateLength?: number
  targetLengths?: number[]
  model?: string
  compact?: boolean
  showDetails?: boolean
}>(), {
  status: 'missing',
  stateLength: 0,
  targetLengths: () => [],
  model: '',
  compact: false,
  showDetails: true
})

const { t } = useI18n()

const label = computed(() => t(`admin.ops.turnState.status.${props.status}`))
const animated = computed(() => props.status === 'running' || props.status === 'pending')

const tone = computed(() => {
  if (props.status === 'ready') {
    return {
      dot: 'bg-emerald-500',
      iconBackground: 'bg-emerald-50 ring-emerald-200 dark:bg-emerald-950/50 dark:ring-emerald-900',
      text: 'text-emerald-700 dark:text-emerald-300'
    }
  }
  if (props.status === 'running' || props.status === 'pending') {
    return {
      dot: 'bg-blue-500',
      iconBackground: 'bg-blue-50 ring-blue-200 dark:bg-blue-950/50 dark:ring-blue-900',
      text: 'text-blue-700 dark:text-blue-300'
    }
  }
  if (props.status === 'expiring' || props.status === 'retry_wait') {
    return {
      dot: 'bg-amber-500',
      iconBackground: 'bg-amber-50 ring-amber-200 dark:bg-amber-950/50 dark:ring-amber-900',
      text: 'text-amber-700 dark:text-amber-300'
    }
  }
  if (props.status === 'failed') {
    return {
      dot: 'bg-red-500',
      iconBackground: 'bg-red-50 ring-red-200 dark:bg-red-950/50 dark:ring-red-900',
      text: 'text-red-700 dark:text-red-300'
    }
  }
  return {
    dot: 'bg-gray-400 dark:bg-dark-400',
    iconBackground: 'bg-gray-100 ring-gray-200 dark:bg-dark-700 dark:ring-dark-600',
    text: 'text-gray-600 dark:text-gray-300'
  }
})
</script>
