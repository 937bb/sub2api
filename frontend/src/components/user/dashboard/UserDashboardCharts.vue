<template>
  <section class="console-analysis-layout">
    <div class="console-analysis-toolbar">
      <div class="flex flex-wrap items-center gap-3">
        <span class="text-sm font-medium">{{ t('console.rangeData') }}</span>
        <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">{{ t('common.refresh') }}</button>
        <div class="ml-auto flex items-center gap-2">
          <span class="text-sm">{{ t('dashboard.granularity') }}</span>
          <div class="w-28"><Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" /></div>
        </div>
      </div>
    </div>
    <div class="console-charts-grid">
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
      <ModelDistributionChart :model-stats="models" :loading="loading" :enable-breakdown="false" :show-account-cost="false" />
    </div>
  </section>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
defineProps<{ loading: boolean; startDate: string; endDate: string; granularity: string; trend: TrendDataPoint[]; models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()
</script>
