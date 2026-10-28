<template>
  <section class="finance-methods">
    <header class="finance-panel-heading">
      <h3>{{ t('payment.admin.paymentDistribution') }}</h3>
    </header>
    <div
      v-if="!methods?.length"
      class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('payment.admin.noData') }}
    </div>
    <div v-else class="finance-method-list">
      <div v-for="method in methods" :key="method.type" class="finance-method-row">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('payment.methods.' + method.type, method.type) }}
            </span>
          </div>
          <div class="space-y-1 text-right tabular-nums">
            <span v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="block text-sm font-medium text-gray-900 dark:text-white">
              {{ formatMoney(currency, amount) }}
            </span>
            <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
              {{ method.count }} {{ t('payment.admin.orders') }}
            </span>
          </div>
        </div>
        <div v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="flex items-center gap-2">
          <span class="w-10 text-xs text-gray-500 dark:text-gray-400">{{ currency }}</span>
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div
              class="h-full rounded-full bg-primary-500 dark:bg-primary-400"
              :style="{ width: barWidth(currency, amount) + '%' }"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyAmounts, PaymentMethodStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  methods: PaymentMethodStats[]
}>()

const maxAmounts = computed<CurrencyAmounts>(() => {
  return props.methods.reduce<CurrencyAmounts>((maximums, method) => {
    for (const [currency, amount] of Object.entries(method.amount)) {
      maximums[currency] = Math.max(maximums[currency] || 0, amount)
    }
    return maximums
  }, {})
})

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}

function barWidth(currency: string, amount: number): number {
  return Math.min((amount / (maxAmounts.value[currency] || 1)) * 100, 100)
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}
</script>
