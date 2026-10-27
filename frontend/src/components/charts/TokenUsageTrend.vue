<template>
  <section class="card console-activity">
    <div class="console-panel-heading console-analysis-heading">
      <div><h3>{{ t('console.analysis') }}</h3><span class="console-chart-total"><strong>{{ summary }}</strong> <small>{{ t(metric === 'cache' ? 'console.cacheRate' : 'console.rangeTotal') }}</small></span></div>
      <div class="console-chart-tabs" role="group" :aria-label="t('console.analysis')">
        <button v-for="tab in tabs" :key="tab.id" type="button" :aria-pressed="metric === tab.id" :class="{ selected: metric === tab.id }" @click="metric = tab.id">{{ t(tab.label) }}</button>
      </div>
    </div>
    <div class="console-plot-toolbar">
      <p>{{ t('console.analysisHint') }}</p>
      <div class="console-density-switch" role="group" :aria-label="t('console.chartType')">
        <button type="button" :aria-pressed="chartForm === 'bar'" @click="chartForm = 'bar'">{{ t('console.barChart') }}</button>
        <button type="button" :aria-pressed="chartForm === 'area'" @click="chartForm = 'area'">{{ t('console.areaChart') }}</button>
      </div>
    </div>
    <div v-if="loading" class="console-plot skeleton" role="status" :aria-label="t('common.loading')" />
    <div v-else-if="trendData.length" class="console-plot">
      <Bar v-if="chartForm === 'bar'" :data="metric === 'tokens' ? tokenData : lineData" :options="barOptions" :aria-label="t(tabs.find(tab => tab.id === metric)!.label)" role="img" />
      <Line v-else :data="metric === 'tokens' ? tokenAreaData : lineData" :options="lineOptions" :aria-label="t(tabs.find(tab => tab.id === metric)!.label)" role="img" />
    </div>
    <div v-else class="console-plot console-chart-empty">{{ t('admin.dashboard.noDataAvailable') }}</div>
    <div class="console-chart-footnote">{{ t('console.rangeData') }}<span v-if="trendData.length">{{ trendData[0].date }} — {{ trendData[trendData.length - 1].date }}</span></div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDark } from '@vueuse/core'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend, Filler } from 'chart.js'
import { Bar, Line } from 'vue-chartjs'
import type { TrendDataPoint } from '@/types'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend, Filler)
const props = withDefaults(defineProps<{ trendData: TrendDataPoint[]; loading?: boolean; showCosts?: boolean }>(), { showCosts: true })
const { t } = useI18n()
const dark = useDark({ storageKey: 'theme' })
const chartForm = ref<'bar' | 'area'>('bar')
const metric = ref<'tokens' | 'requests' | 'cost' | 'cache'>('tokens')
const tabs = computed(() => ([
  { id: 'tokens', label: 'console.tokenComposition' },
  { id: 'requests', label: 'console.requests' },
  { id: 'cost', label: 'console.spending' },
  { id: 'cache', label: 'console.cacheRate' }
] as const).filter(tab => props.showCosts || tab.id !== 'cost'))
watch(() => props.showCosts, show => { if (!show && metric.value === 'cost') metric.value = 'tokens' })
const safe = (n: unknown) => typeof n === 'number' && Number.isFinite(n) ? n : 0
const compact = (n: number) => n >= 1e9 ? `${(n / 1e9).toFixed(2)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(2)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}K` : n.toLocaleString()
const sum = (key: keyof TrendDataPoint) => props.trendData.reduce((n, row) => n + safe(row[key]), 0)
const hitRate = (row: TrendDataPoint) => {
  const prompt = safe(row.input_tokens) + safe(row.cache_creation_tokens) + safe(row.cache_read_tokens)
  return prompt > 0 ? safe(row.cache_read_tokens) / prompt * 100 : 0
}
const summary = computed(() => {
  if (metric.value === 'cost') return `$${sum('actual_cost').toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`
  if (metric.value === 'requests') return sum('requests').toLocaleString()
  if (metric.value === 'cache') {
    const prompt = sum('input_tokens') + sum('cache_creation_tokens') + sum('cache_read_tokens')
    return `${(prompt ? sum('cache_read_tokens') / prompt * 100 : 0).toFixed(1)}%`
  }
  return compact(sum('input_tokens') + sum('output_tokens') + sum('cache_creation_tokens') + sum('cache_read_tokens'))
})
const labels = computed(() => props.trendData.map(row => row.date))
const tokenData = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('console.input'), key: 'input_tokens', color: '#4f78ec' },
    { label: t('console.output'), key: 'output_tokens', color: '#8c6bdd' },
    { label: t('console.cacheRead'), key: 'cache_read_tokens', color: '#58bca5' },
    { label: t('console.cacheWrite'), key: 'cache_creation_tokens', color: '#eab64d' }
  ].map(item => ({ label: item.label, data: props.trendData.map(row => safe(row[item.key as keyof TrendDataPoint])), backgroundColor: item.color, borderWidth: 0, borderRadius: 3, maxBarThickness: 52 }))
}))
const tokenAreaData = computed(() => ({
  labels: tokenData.value.labels,
  datasets: tokenData.value.datasets.map(dataset => ({ ...dataset, borderColor: dataset.backgroundColor, backgroundColor: dataset.backgroundColor + '65', fill: true }))
}))
const lineData = computed(() => ({
  labels: labels.value,
  datasets: metric.value === 'cache' ? [
    { label: t('console.cacheRate'), data: props.trendData.map(hitRate), borderColor: '#27857a', backgroundColor: '#27857a18', fill: true }
  ] : metric.value === 'cost' ? [
    { label: t('dashboard.actual'), data: props.trendData.map(row => safe(row.actual_cost)), borderColor: '#4f78ec', backgroundColor: '#4f78ec18', fill: true },
    { label: t('dashboard.standard'), data: props.trendData.map(row => safe(row.cost)), borderColor: '#8b97ac', borderDash: [4, 4], fill: false }
  ] : [
    { label: t('console.requests'), data: props.trendData.map(row => safe(row.requests)), borderColor: '#4f78ec', backgroundColor: '#4f78ec18', fill: true }
  ]
}))
const commonOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false as const,
  interaction: { intersect: false, mode: 'index' as const },
  plugins: {
    legend: { position: 'bottom' as const, align: 'center' as const, labels: { color: dark.value ? '#bec9d9' : '#536175', usePointStyle: true, pointStyle: 'rectRounded', boxWidth: 8, boxHeight: 8, padding: 20, font: { size: 11 } } },
    tooltip: { padding: 12, cornerRadius: 6, backgroundColor: '#172538', callbacks: {
      label: (context: { dataset: { label?: string }; parsed: { y: number | null } }) => {
        const value = context.parsed.y || 0
        return `${context.dataset.label}: ${metric.value === 'cost' ? '$' + value.toFixed(4) : metric.value === 'cache' ? value.toFixed(1) + '%' : compact(value)}`
      }
    } }
  },
  scales: {
    x: { grid: { display: false }, border: { display: false }, ticks: { color: dark.value ? '#bec9d9' : '#536175', maxRotation: 0, autoSkip: true, maxTicksLimit: 7, font: { size: 11 } } },
    y: { beginAtZero: true, border: { display: false }, grid: { color: dark.value ? '#303e50' : '#edf0f4' }, ticks: { color: dark.value ? '#bec9d9' : '#536175', maxTicksLimit: 5, padding: 10, font: { size: 11 }, callback: (n: string | number) => metric.value === 'cache' ? `${n}%` : metric.value === 'cost' ? `$${compact(Number(n))}` : compact(Number(n)) } }
  }
}))
const barOptions = computed(() => ({
  ...commonOptions.value,
  scales: { x: { ...commonOptions.value.scales.x, stacked: metric.value === 'tokens' }, y: { ...commonOptions.value.scales.y, stacked: metric.value === 'tokens' } }
}))
const lineOptions = computed(() => ({
  ...commonOptions.value,
  elements: { point: { radius: 0, hitRadius: 12, hoverRadius: 4 }, line: { borderWidth: 2, tension: 0.25 } },
  scales: { x: { ...commonOptions.value.scales.x, stacked: metric.value === 'tokens' }, y: { ...commonOptions.value.scales.y, stacked: metric.value === 'tokens', ...(metric.value === 'cache' ? { min: 0, max: 100 } : {}) } }
}))
</script>
