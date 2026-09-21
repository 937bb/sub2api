<template>
  <section class="min-w-0 border-y border-gray-100 py-3 dark:border-dark-700" data-test="state-routing-guard" aria-labelledby="state-routing-guard-title">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <h3 id="state-routing-guard-title" class="text-xs font-semibold text-gray-800 dark:text-dark-100">
          {{ t('admin.ops.turnState.scanSettings.routingGuard') }}
        </h3>
        <p class="mt-1 text-[11px] leading-relaxed text-gray-500 dark:text-dark-400">
          {{ t('admin.ops.turnState.scanSettings.routingGuardHint') }}
        </p>
      </div>
      <Toggle
        :model-value="modelValue"
        :aria-label="t('admin.ops.turnState.scanSettings.routingGuard')"
        data-test="state-routing-guard-toggle"
        @update:model-value="emit('update:modelValue', $event)"
      />
    </div>
    <p :class="['mt-2 text-[11px] font-medium', modelValue ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-300']" data-test="state-routing-guard-status">
      {{ t(modelValue ? 'admin.ops.turnState.scanSettings.routingGuardEnabled' : 'admin.ops.turnState.scanSettings.routingGuardDisabled') }}
    </p>
    <p v-if="modelValue && disabledPlans.length" class="mt-2 border-l-2 border-amber-400 pl-2 text-[11px] leading-relaxed text-amber-700 dark:text-amber-300" role="status" data-test="state-routing-guard-warning">
      {{ t('admin.ops.turnState.scanSettings.routingGuardScanWarning', { plans: disabledPlansLabel }) }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

const props = defineProps<{
  modelValue: boolean
  planScanEnabled: Record<string, boolean>
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
}>()
const { t } = useI18n()
const plans = ['pro', 'team', 'plus', 'free', 'enterprise']
const disabledPlans = computed(() => plans.filter(plan => props.planScanEnabled[plan] === false))
const disabledPlansLabel = computed(() => disabledPlans.value.map(plan => plan.toUpperCase()).join(', '))
</script>
