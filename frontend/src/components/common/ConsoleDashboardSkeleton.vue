<template>
  <div class="dashboard-skeleton" role="status" :aria-label="t('common.loading')">
    <div class="dashboard-skeleton-metrics">
      <div v-for="item in 5" :key="item" class="dashboard-skeleton-metric">
        <Skeleton width="42%" height="10px" />
        <Skeleton width="64%" height="28px" />
        <Skeleton width="78%" height="8px" />
      </div>
    </div>
    <div class="dashboard-skeleton-workspace">
      <section class="dashboard-skeleton-chart">
        <div class="dashboard-skeleton-heading">
          <Skeleton width="128px" height="12px" />
          <Skeleton width="86px" height="30px" />
        </div>
        <div class="dashboard-skeleton-plot">
          <span v-for="bar in 9" :key="bar" :style="{ height: `${plotHeights[bar - 1]}%` }"></span>
        </div>
      </section>
      <section class="dashboard-skeleton-feed">
        <Skeleton width="112px" height="12px" />
        <div v-for="row in 4" :key="row" class="dashboard-skeleton-row">
          <Skeleton variant="circle" width="28px" height="28px" />
          <div><Skeleton width="68%" height="10px" /><Skeleton width="42%" height="8px" /></div>
        </div>
      </section>
    </div>
    <span class="sr-only">{{ t('common.loading') }}</span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Skeleton from '@/components/common/Skeleton.vue'

const { t } = useI18n()
const plotHeights = [34, 52, 43, 68, 58, 77, 63, 88, 74]
</script>

<style scoped>
.dashboard-skeleton { display: grid; gap: 16px; }
.dashboard-skeleton-metrics { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); border: 1px solid var(--console-line); background: var(--console-surface); }
.dashboard-skeleton-metric { display: grid; min-height: 118px; align-content: center; gap: 13px; padding: 18px; }
.dashboard-skeleton-metric + .dashboard-skeleton-metric { border-left: 1px solid var(--console-line); }
.dashboard-skeleton-workspace { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(280px, .8fr); gap: 16px; }
.dashboard-skeleton-chart, .dashboard-skeleton-feed { min-height: 286px; border: 1px solid var(--console-line); background: var(--console-surface); padding: 20px; }
.dashboard-skeleton-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.dashboard-skeleton-plot { display: flex; height: 196px; align-items: end; gap: 5%; margin-top: 28px; border-bottom: 1px solid var(--console-line); }
.dashboard-skeleton-plot span { width: 6.2%; min-height: 22px; background: color-mix(in srgb, var(--console-accent) 22%, var(--console-surface)); animation: dashboard-bar 1.8s ease-in-out infinite; }
.dashboard-skeleton-plot span:nth-child(3n + 2) { animation-delay: -.6s; }.dashboard-skeleton-plot span:nth-child(3n) { animation-delay: -1.2s; }
.dashboard-skeleton-feed { display: grid; align-content: start; gap: 20px; }
.dashboard-skeleton-row { display: grid; grid-template-columns: 28px 1fr; align-items: center; gap: 12px; padding-top: 14px; border-top: 1px solid var(--console-line); }
.dashboard-skeleton-row > div { display: grid; gap: 8px; }
@keyframes dashboard-bar { 0%, 100% { opacity: .48; } 50% { opacity: 1; } }
@media (max-width: 1023px) { .dashboard-skeleton-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }.dashboard-skeleton-metric + .dashboard-skeleton-metric { border-left: 0; }.dashboard-skeleton-metric:nth-child(even) { border-left: 1px solid var(--console-line); }.dashboard-skeleton-metric:nth-child(n + 3) { border-top: 1px solid var(--console-line); }.dashboard-skeleton-workspace { grid-template-columns: 1fr; } }
@media (max-width: 520px) { .dashboard-skeleton-metrics { grid-template-columns: 1fr; }.dashboard-skeleton-metric:nth-child(n) { min-height: 98px; border-left: 0; border-top: 1px solid var(--console-line); }.dashboard-skeleton-metric:first-child { border-top: 0; }.dashboard-skeleton-feed { min-height: 250px; } }
@media (prefers-reduced-motion: reduce) { .dashboard-skeleton-plot span { animation: none; } }
</style>
