<template>
  <div class="min-w-0" data-test="codex-account-state-summary">
    <div class="mb-1 flex min-w-0 items-center justify-between gap-1.5">
      <div class="flex min-w-0 items-center gap-1">
        <span :class="['h-1.5 w-1.5 shrink-0 rounded-full', summaryTone.dot, summaryTone.pulse ? 'animate-pulse' : '']" />
        <span :class="['truncate text-[10px] font-semibold leading-4', summaryTone.text]">
          {{ t('admin.ops.turnState.readyProgress', { ready: readyCount, total: states.length }) }}
        </span>
      </div>
      <span v-if="failedCount" class="shrink-0 text-[9px] font-medium leading-4 text-red-600 dark:text-red-400">
        {{ t('admin.ops.turnState.failedCount', { count: failedCount }) }}
      </span>
    </div>

    <div v-if="states.length" class="flex min-w-0 flex-col gap-0.5">
      <div
        v-for="state in states"
        :key="state.model"
        :class="[
          'grid min-w-0 grid-cols-[6px_minmax(0,1fr)_auto] items-center gap-1 text-[10px] font-medium leading-4',
          textTone(state.status)
        ]"
        :title="stateTitle(state)"
        :data-test="`codex-model-state-${state.model}`"
        :data-status="state.status"
      >
        <span :class="['h-1.5 w-1.5 shrink-0 rounded-full', dotTone(state.status)]" />
        <span class="truncate font-mono">{{ compactModel(state.model) }}</span>
        <span class="shrink-0 whitespace-nowrap text-right font-mono tabular-nums">{{ stateValue(state) }}</span>
      </div>
    </div>
    <div v-else class="text-[10px] leading-4 text-red-500 dark:text-red-400">
      {{ t('admin.ops.turnState.noTargetModels') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CodexTurnStateAccountStatus, CodexTurnStateAccountStatusValue } from '@/api/admin/ops'

const props = defineProps<{
  states: CodexTurnStateAccountStatus[]
}>()

const { t } = useI18n()

const readyCount = computed(() => props.states.filter(state => state.status === 'ready').length)
const failedCount = computed(() => props.states.filter(state => state.status === 'failed' || state.status === 'retry_wait').length)
const hasRunning = computed(() => props.states.some(state => state.status === 'running' || state.status === 'pending'))

const summaryTone = computed(() => {
  if (props.states.length > 0 && readyCount.value === props.states.length) {
    return { dot: 'bg-emerald-500', text: 'text-emerald-700 dark:text-emerald-300', pulse: false }
  }
  if (hasRunning.value) {
    return { dot: 'bg-blue-500', text: 'text-blue-700 dark:text-blue-300', pulse: true }
  }
  if (failedCount.value > 0) {
    return { dot: 'bg-red-500', text: 'text-red-700 dark:text-red-300', pulse: false }
  }
  return { dot: 'bg-amber-500', text: 'text-amber-700 dark:text-amber-300', pulse: false }
})

function compactModel(model: string) {
  return model.replace(/^gpt-/i, '').replace(/-(astra|sol|terra|luna)$/i, ' $1')
}

function compactStatus(status: CodexTurnStateAccountStatusValue) {
  if (status === 'running' || status === 'pending') return t('admin.ops.turnState.compact.scanning')
  if (status === 'retry_wait') return t('admin.ops.turnState.compact.retry')
  if (status === 'failed') return t('admin.ops.turnState.compact.failed')
  if (status === 'expiring') return t('admin.ops.turnState.compact.expiring')
  return t('admin.ops.turnState.compact.missing')
}

function dotTone(status: CodexTurnStateAccountStatusValue) {
  if (status === 'ready') return 'bg-emerald-500'
  if (status === 'running' || status === 'pending') return 'bg-blue-500 animate-pulse'
  if (status === 'expiring') return 'bg-amber-500'
  if (status === 'missing' || status === 'retry_wait' || status === 'failed') return 'bg-red-500'
  return 'bg-gray-400'
}

function textTone(status: CodexTurnStateAccountStatusValue) {
  if (status === 'ready') return 'text-emerald-700 dark:text-emerald-300'
  if (status === 'running' || status === 'pending') return 'text-blue-700 dark:text-blue-300'
  if (status === 'expiring') return 'text-amber-700 dark:text-amber-300'
  if (status === 'missing' || status === 'retry_wait' || status === 'failed') return 'text-red-600 dark:text-red-400'
  return 'text-gray-500 dark:text-dark-300'
}

function stateValue(state: CodexTurnStateAccountStatus) {
  if (state.status === 'ready' && state.state_length) return String(state.state_length)
  const status = compactStatus(state.status)
  return state.state_length ? `${state.state_length} · ${status}` : status
}

function stateTitle(state: CodexTurnStateAccountStatus) {
  const lines = [
    state.model,
    t(`admin.ops.turnState.status.${state.status}`),
    t('admin.ops.turnState.attempts', { count: state.attempt_count })
  ]
  if (state.state_length) lines.push(`State ${state.state_length}`)
  if (state.target_lengths?.length) lines.push(t('admin.ops.turnState.targetLengths', { lengths: state.target_lengths.join(' / ') }))
  if (state.last_attempt_at) lines.push(`${t('admin.ops.turnState.lastScan')}: ${new Date(state.last_attempt_at).toLocaleString()}`)
  if (state.last_error) lines.push(state.last_error)
  return lines.join('\n')
}
</script>
