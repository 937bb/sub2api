<template>
  <section class="finance-trend">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.dailyRevenue') }}
    </h3>
    <WorkspaceTabs v-model="metric" :label="t('payment.admin.dailyRevenue')" :tabs="metrics" />
    <div class="h-80">
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
import { computed, ref } from 'vue'
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
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { DailyPaymentStats } from '@/types/payment'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

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

const colors = [
  ['rgb(59, 130, 246)', 'rgba(59, 130, 246, 0.1)'],
  ['rgb(168, 85, 247)', 'rgba(168, 85, 247, 0.1)'],
  ['rgb(245, 158, 11)', 'rgba(245, 158, 11, 0.1)'],
  ['rgb(239, 68, 68)', 'rgba(239, 68, 68, 0.1)'],
]

const chartData = computed(() => {
  if (!props.data || props.data.length === 0) return null
  const currencies = [...new Set(props.data.flatMap(day => Object.keys(day.amount)))].sort()
  return {
    labels: props.data.map(d => d.date),
    datasets: [
      ...currencies.map((currency, index) => {
        const [borderColor, backgroundColor] = colors[index % colors.length]
        return {
          label: `${currency} ${t('payment.admin.revenue')}`,
          data: props.data.map(day => day.amount[currency] || 0),
          borderColor,
          backgroundColor,
          fill: false,
          tension: 0.1,
          pointRadius: 0,
          pointHoverRadius: 5,
          hidden: metric.value !== currency,
        }
      }),
      {
        label: t('payment.admin.orderCount'),
        data: props.data.map(d => d.count),
        borderColor: 'rgb(99, 91, 206)',
        backgroundColor: 'rgba(99, 91, 206, 0.1)',
        fill: false,
        tension: 0.1,
        pointRadius: 0,
        pointHoverRadius: 5,
        yAxisID: 'y1',
        hidden: metric.value !== 'orders',
      }
    ]
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
      display: metric.value !== 'orders',
      position: 'left' as const,
      title: { display: true, text: metric.value },
      beginAtZero: true,
      ticks: { color: dark.value ? '#bec9d9' : '#536175' },
      grid: { color: dark.value ? '#303e50' : '#edf0f4' },
    },
    y1: {
      type: 'linear' as const,
      display: metric.value === 'orders',
      position: 'left' as const,
      beginAtZero: true,
      ticks: { precision: 0, color: dark.value ? '#bec9d9' : '#536175' },
      title: { display: true, text: t('payment.admin.orderCount') },
      grid: { color: dark.value ? '#303e50' : '#edf0f4' },
    },
    x: { grid: { display: false }, ticks: { color: dark.value ? '#bec9d9' : '#536175', maxTicksLimit: 10, maxRotation: 0 } }
  },
  plugins: {
    legend: { display: false },
  }
}))
</script>
