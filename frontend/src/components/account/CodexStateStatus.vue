<template>
  <div
    :class="[
      'inline-flex min-w-0 items-center',
      compact ? 'gap-1.5' : 'gap-2'
    ]"
    data-test="codex-state-status"
  >
    <span
      :class="[
        'relative flex shrink-0 items-center justify-center rounded-full',
        compact ? 'h-5 w-5' : 'h-7 w-7',
        tone.iconBackground
      ]"
      aria-hidden="true"
    >
      <span
        :class="[
          'rounded-full',
          compact ? 'h-1.5 w-1.5' : 'h-2 w-2',
          tone.dot,
          animated ? 'animate-pulse' : ''
        ]"
      />
    </span>

    <div class="min-w-0">
      <div :class="['truncate font-medium leading-4', compact ? 'text-xs' : 'text-sm', tone.text]">
        {{ label }}
      </div>
      <div v-if="showDetails" class="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] leading-4 text-gray-400 dark:text-dark-400">
        <span v-if="stateLength > 0" class="font-mono font-semibold text-gray-600 dark:text-gray-300">
          {{ stateLength }}
        </span>
        <span v-if="stateLength > 0 && model" aria-hidden="true">/</span>
        <span v-if="model" class="max-w-32 truncate font-mono" :title="model">{{ model }}</span>
      </div>
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
  model?: string
  compact?: boolean
  showDetails?: boolean
}>(), {
  status: 'missing',
  stateLength: 0,
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
      iconBackground: 'bg-emerald-50 dark:bg-emerald-950/50',
      text: 'text-emerald-700 dark:text-emerald-300'
    }
  }
  if (props.status === 'running' || props.status === 'pending') {
    return {
      dot: 'bg-blue-500',
      iconBackground: 'bg-blue-50 dark:bg-blue-950/50',
      text: 'text-blue-700 dark:text-blue-300'
    }
  }
  if (props.status === 'expiring' || props.status === 'retry_wait') {
    return {
      dot: 'bg-amber-500',
      iconBackground: 'bg-amber-50 dark:bg-amber-950/50',
      text: 'text-amber-700 dark:text-amber-300'
    }
  }
  if (props.status === 'failed') {
    return {
      dot: 'bg-red-500',
      iconBackground: 'bg-red-50 dark:bg-red-950/50',
      text: 'text-red-700 dark:text-red-300'
    }
  }
  return {
    dot: 'bg-gray-400 dark:bg-dark-400',
    iconBackground: 'bg-gray-100 dark:bg-dark-700',
    text: 'text-gray-600 dark:text-gray-300'
  }
})
</script>
