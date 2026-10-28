<template>
  <section class="finance-trend">
    <header class="finance-panel-heading">
      <h3>{{ t('payment.admin.dailyRevenue') }}</h3>
      <WorkspaceTabs v-model="metric" :label="t('payment.admin.dailyRevenue')" :tabs="metrics" />
    </header>
    <div class="finance-trend-plot">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner size="md" />
      </div>
      <Line v-else-if="chartData" :data="chartData" :options="chartOptions" />
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('payment.admin.noData') }}
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDark } from '@vueuse/core'
import WorkspaceTabs from '@/components/common/WorkspaceTabs.vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { DailyPaymentStats } from '@/types/payment'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const { t } = useI18n()

const props = defineProps<{
  data: DailyPaymentStats[]
  loading?: boolean
}>()

const dark = useDark({ storageKey: 'theme' })
const metric = ref('orders')
const metrics = computed(() => [
  { value: 'orders', label: t('payment.admin.orderCount') },
  ...[...new Set(props.data.flatMap(day => Object.keys(day.amount)))].sort().map(currency => ({ value: currency, label: `${currency} ${t('payment.admin.revenue')}` }))
])

watch(metrics, options => {
  if (!options.some(option => option.value === metric.value)) metric.value = 'orders'
})

const chartData = computed(() => {
  if (!props.data || props.data.length === 0) return null
  return {
    labels: props.data.map(d => d.date),
    datasets: [{
      label: metric.value === 'orders' ? t('payment.admin.orderCount') : `${metric.value} ${t('payment.admin.revenue')}`,
      data: props.data.map(day => metric.value === 'orders' ? day.count : day.amount[metric.value] || 0),
      borderColor: dark.value ? '#80b5d6' : '#42799b',
      backgroundColor: dark.value ? '#80b5d6' : '#42799b',
      borderWidth: 2,
      fill: false,
      tension: 0,
      pointRadius: props.data.length === 1 ? 3 : 0,
      pointHoverRadius: 4,
    }]
  }
})

const chartOptions = computed(() => ({
  animation: false as const,
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  scales: {
    y: {
      type: 'linear' as const,
      position: 'left' as const,
      beginAtZero: true,
      border: { display: false },
      ticks: { precision: metric.value === 'orders' ? 0 : 2, maxTicksLimit: 5, color: dark.value ? '#bec9d9' : '#536175' },
      title: { display: metric.value !== 'orders', text: metric.value, color: dark.value ? '#bec9d9' : '#536175' },
      grid: { color: dark.value ? '#303e50' : '#edf0f4' },
    },
    x: {
      border: { display: false },
      grid: { display: false },
      ticks: {
        color: dark.value ? '#bec9d9' : '#536175',
        maxTicksLimit: 8,
        maxRotation: 0,
        callback: (value: string | number) => props.data[Number(value)]?.date.slice(5) || '',
      }
    }
  },
  plugins: {
    legend: { display: false },
  }
}))
</script>
