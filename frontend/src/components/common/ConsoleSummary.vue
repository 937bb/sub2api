<template>
  <section class="console-metrics" :aria-label="t('console.todayOverview')">
    <div class="console-metric-strip" :class="{ 'console-metric-strip-simple': simple }">
      <div class="console-metric"><span><Icon name="chart" size="sm" />{{ t('console.todayOverview') }}</span><strong>{{ number(stats.today_requests) }}</strong><small>{{ t('dashboard.requests') }}</small></div>
      <div v-if="!simple" class="console-metric"><span><Icon name="dollar" size="sm" />{{ t('dashboard.todayCost') }}</span><strong>{{ money(stats.today_actual_cost) }}</strong><small>{{ t('dashboard.standard') }} {{ money(stats.today_cost) }}</small></div>
      <div class="console-metric"><span><Icon name="cube" size="sm" />{{ t('dashboard.todayTokens') }}</span><strong>{{ tokens(stats.today_tokens) }}</strong><small>{{ t('console.input') }} {{ tokens(stats.today_input_tokens) }} · {{ t('console.output') }} {{ tokens(stats.today_output_tokens) }}</small></div>
      <div class="console-metric"><span>RPM</span><strong>{{ number(stats.rpm) }}</strong><small>{{ t('console.requestsPerMinute') }}</small></div>
      <div class="console-metric"><span>TPM</span><strong>{{ tokens(stats.tpm) }}</strong><small>{{ t('console.tokensPerMinute') }}</small></div>
    </div>
    <div class="console-health-strip">
      <span><Icon name="clock" size="sm" />{{ t('dashboard.avgResponse') }} <b>{{ duration(stats.average_duration_ms) }}</b></span>
      <span v-if="admin">{{ t('console.schedulableAccounts') }} <b>{{ number(stats.normal_accounts) }}</b> / {{ number(stats.total_accounts) }}</span>
      <span v-if="admin && stats.error_accounts" class="console-health-error">{{ t('common.error') }} <b>{{ number(stats.error_accounts) }}</b></span>
      <span v-if="!admin && !simple">{{ t('dashboard.balance') }} <b>{{ money(balance) }}</b></span>
      <span>{{ t('nav.apiKeys') }} <b>{{ number(stats.active_api_keys) }}</b> / {{ number(stats.total_api_keys) }}</span>
      <span>{{ t('console.cacheRead') }} <b>{{ tokens(stats.today_cache_read_tokens) }}</b></span>
      <span>{{ t('console.cacheWrite') }} <b>{{ tokens(stats.today_cache_creation_tokens) }}</b></span>
      <span v-if="admin && !simple">{{ t('admin.dashboard.accountCost') }} <b>{{ money(stats.today_account_cost) }}</b></span>
    </div>
    <details class="console-metric-details">
      <summary>{{ t('console.lifetime') }}<Icon name="chevronDown" size="xs" /></summary>
      <dl>
        <div><dt>{{ t('dashboard.requests') }}</dt><dd>{{ number(stats.total_requests) }}</dd></div>
        <div><dt>{{ t('dashboard.totalTokens') }}</dt><dd>{{ tokens(stats.total_tokens) }}</dd></div>
        <div><dt>{{ t('console.input') }} / {{ t('console.output') }}</dt><dd>{{ tokens(stats.total_input_tokens) }} / {{ tokens(stats.total_output_tokens) }}</dd></div>
        <div><dt>{{ t('dashboard.cache') }}</dt><dd>{{ tokens((stats.total_cache_creation_tokens || 0) + (stats.total_cache_read_tokens || 0)) }}</dd></div>
        <template v-if="!simple"><div><dt>{{ t('console.totalCharged') }}</dt><dd>{{ money(stats.total_actual_cost) }}</dd></div><div><dt>{{ t('dashboard.standard') }}</dt><dd>{{ money(stats.total_cost) }}</dd></div><div v-if="admin"><dt>{{ t('admin.dashboard.accountCost') }}</dt><dd>{{ money(stats.total_account_cost) }}</dd></div></template>
        <template v-if="admin"><div><dt>{{ t('admin.dashboard.users') }}</dt><dd>{{ number(stats.total_users) }}</dd></div><div><dt>{{ t('admin.dashboard.activeUsers') }}</dt><dd>{{ number(stats.active_users) }}</dd></div><div><dt>{{ t('console.today') }} · {{ t('admin.dashboard.users') }}</dt><dd>+{{ number(stats.today_new_users) }}</dd></div></template>
      </dl>
    </details>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { DashboardStats } from '@/types'
defineProps<{ stats: Partial<DashboardStats>; admin?: boolean; balance?: number; simple?: boolean }>()
const { t } = useI18n()
const number = (n?: number) => (n ?? 0).toLocaleString()
const tokens = (n = 0) => n >= 1e9 ? `${(n / 1e9).toFixed(2)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(2)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}K` : number(n)
const money = (n = 0) => `$${n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`
const duration = (n = 0) => n >= 1000 ? `${(n / 1000).toFixed(2)}s` : `${Math.round(n)}ms`
</script>
