<template>
  <div v-if="windowStats && (showEmpty || windowStats.requests > 0 || windowStats.tokens > 0)" class="mb-0.5 flex items-center">
    <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
      <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
        {{ formatCompactNumber(windowStats.requests, { allowBillions: false }) }} req
      </span>
      <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
        {{ formatCompactNumber(windowStats.tokens) }}
      </span>
      <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
        A ${{ windowStats.cost.toFixed(2) }}
      </span>
      <span v-if="windowStats.user_cost != null" class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.userBilled')">
        U ${{ windowStats.user_cost.toFixed(2) }}
      </span>
      <span
        v-if="estimatedTotalCost != null" data-test="estimated-total-cost"
        class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
        :title="t('admin.accounts.usageWindow.estimatedTotalCostTooltip')"
      >
        {{ t('admin.accounts.usageWindow.estimatedTotalCost', { cost: estimatedTotalCost.toFixed(2) }) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'

defineProps<{
  windowStats?: WindowStats | null
  estimatedTotalCost?: number | null
  showEmpty?: boolean
}>()
const { t } = useI18n()
</script>
