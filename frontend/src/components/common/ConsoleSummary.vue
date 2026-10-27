<template>
  <section class="console-summary" :aria-label="t('console.todayOverview')">
    <article class="console-summary-volume">
      <div class="console-summary-title"><Icon name="chart" size="sm" /><h3>{{ t('console.todayOverview') }}</h3></div>
      <div class="console-summary-primary"><strong>{{ number(stats.today_requests) }}</strong><span>{{ t('dashboard.requests') }}</span></div>
      <dl>
        <div><dt>{{ t('dashboard.todayTokens') }}</dt><dd>{{ tokens(stats.today_tokens) }}</dd></div>
        <div><dt>{{ t('dashboard.avgResponse') }}</dt><dd>{{ duration(stats.average_duration_ms) }}</dd></div>
        <div><dt>RPM / TPM</dt><dd>{{ number(stats.rpm) }} / {{ tokens(stats.tpm) }}</dd></div>
      </dl>
      <details class="console-summary-details"><summary>{{ t('console.lifetime') }}</summary>
        <dl><div><dt>{{ t('dashboard.requests') }}</dt><dd>{{ number(stats.total_requests) }}</dd></div><div><dt>{{ t('dashboard.totalTokens') }}</dt><dd>{{ tokens(stats.total_tokens) }}</dd></div></dl>
        <p>{{ t('dashboard.input') }} {{ tokens(stats.total_input_tokens) }} · {{ t('dashboard.output') }} {{ tokens(stats.total_output_tokens) }} · {{ t('dashboard.cache') }} {{ tokens((stats.total_cache_creation_tokens || 0) + (stats.total_cache_read_tokens || 0)) }}</p>
      </details>
    </article>
    <article class="console-summary-finance" v-if="!simple">
      <div class="console-summary-title"><Icon name="dollar" size="sm" /><h3>{{ t('dashboard.todayCost') }}</h3></div>
      <div class="console-summary-primary"><strong>{{ money(stats.today_actual_cost) }}</strong><span>{{ t('dashboard.actual') }}</span></div>
      <dl>
        <div><dt>{{ t('dashboard.standard') }}</dt><dd>{{ money(stats.today_cost) }}</dd></div>
        <div v-if="admin"><dt>{{ t('admin.dashboard.accountCost') }}</dt><dd>{{ money(stats.today_account_cost) }}</dd></div>
        <div v-else><dt>{{ t('dashboard.balance') }}</dt><dd>{{ money(balance) }}</dd></div>
        <div><dt>{{ t('console.totalCharged') }}</dt><dd>{{ money(stats.total_actual_cost) }}</dd></div>
      </dl>
      <details class="console-summary-details"><summary>{{ t('console.lifetime') }}</summary>
        <dl><div><dt>{{ t('dashboard.standard') }}</dt><dd>{{ money(stats.total_cost) }}</dd></div><div v-if="admin"><dt>{{ t('admin.dashboard.accountCost') }}</dt><dd>{{ money(stats.total_account_cost) }}</dd></div></dl>
      </details>
    </article>
    <article class="console-summary-resources">
      <div class="console-summary-title"><Icon name="server" size="sm" /><h3>{{ t(admin ? 'console.resourceOverview' : 'nav.apiKeys') }}</h3></div>
      <div class="console-summary-primary"><strong>{{ number(admin ? stats.normal_accounts : stats.active_api_keys) }}</strong><span>{{ t(admin ? 'console.schedulableAccounts' : 'common.active') }}</span></div>
      <dl>
        <div v-if="admin"><dt>{{ t('admin.dashboard.accounts') }}</dt><dd>{{ number(stats.total_accounts) }} <span v-if="stats.error_accounts" class="console-error"> / {{ number(stats.error_accounts) }} {{ t('common.error') }}</span></dd></div>
        <div><dt>{{ t('nav.apiKeys') }}</dt><dd>{{ number(stats.active_api_keys) }} / {{ number(stats.total_api_keys) }}</dd></div>
        <div v-if="admin"><dt>{{ t('admin.dashboard.users') }}</dt><dd>{{ number(stats.total_users) }} <span>+{{ number(stats.today_new_users) }} {{ t('console.today') }}</span></dd></div>
        <div v-else><dt>{{ t('dashboard.input') }} / {{ t('dashboard.output') }}</dt><dd>{{ tokens(stats.today_input_tokens) }} / {{ tokens(stats.today_output_tokens) }}</dd></div>
      </dl>
      <details class="console-summary-details"><summary>{{ t('console.moreMetrics') }}</summary>
        <dl><div v-if="admin"><dt>{{ t('admin.dashboard.activeUsers') }}</dt><dd>{{ number(stats.active_users) }}</dd></div><div><dt>{{ t('console.cacheRead') }}</dt><dd>{{ tokens(stats.today_cache_read_tokens) }}</dd></div><div><dt>{{ t('console.cacheWrite') }}</dt><dd>{{ tokens(stats.today_cache_creation_tokens) }}</dd></div><div v-if="admin"><dt>{{ t('dashboard.input') }} / {{ t('dashboard.output') }}</dt><dd>{{ tokens(stats.today_input_tokens) }} / {{ tokens(stats.today_output_tokens) }}</dd></div></dl>
      </details>
    </article>
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
