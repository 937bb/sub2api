<template>
  <section class="min-w-0" :aria-label="t('admin.ops.turnState.scanSettings.planSwitches')">
    <div class="flex flex-wrap items-end justify-between gap-x-4 gap-y-1">
      <div>
        <h3 class="text-xs font-semibold text-gray-800 dark:text-dark-100">
          {{ t('admin.ops.turnState.scanSettings.planSwitches') }}
        </h3>
        <p class="mt-0.5 text-[11px] leading-relaxed text-gray-500 dark:text-dark-400">
          {{ t('admin.ops.turnState.scanSettings.planSwitchesHint') }}
        </p>
      </div>
    </div>

    <div class="mt-2 grid grid-cols-1 border-y border-gray-100 sm:grid-cols-2 dark:border-dark-700" data-test="plan-scan-switches">
      <div
        v-for="plan in plans"
        :key="plan"
        class="flex min-h-10 items-center justify-between gap-3 border-b border-gray-100 py-2 sm:odd:pr-4 sm:even:border-l sm:even:pl-4 dark:border-dark-700"
      >
        <div class="flex min-w-0 items-center gap-2">
          <span :class="['h-1.5 w-1.5 shrink-0 rounded-full', isEnabled(plan) ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-500']" />
          <span class="w-20 shrink-0 text-xs font-semibold text-gray-800 dark:text-dark-100">{{ plan.toUpperCase() }}</span>
          <span :class="['truncate text-[11px]', isEnabled(plan) ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400 dark:text-dark-400']">
            {{ t(isEnabled(plan) ? 'admin.ops.turnState.scanSettings.scanEnabled' : 'admin.ops.turnState.scanSettings.scanDisabled') }}
          </span>
        </div>
        <Toggle
          :model-value="isEnabled(plan)"
          :aria-label="`${plan.toUpperCase()} ${t('admin.ops.turnState.scanSettings.planSwitches')}`"
          :data-test="`${testPrefix}${plan}`"
          @update:model-value="setEnabled(plan, $event)"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

const props = withDefaults(defineProps<{
  modelValue: Record<string, boolean>
  testPrefix?: string
}>(), {
  testPrefix: 'plan-scan-'
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: Record<string, boolean>): void
}>()

const { t } = useI18n()
const plans = ['pro', 'team', 'plus', 'free', 'enterprise']

function isEnabled(plan: string) {
  return props.modelValue[plan] ?? true
}

function setEnabled(plan: string, enabled: boolean) {
  emit('update:modelValue', { ...props.modelValue, [plan]: enabled })
}
</script>
