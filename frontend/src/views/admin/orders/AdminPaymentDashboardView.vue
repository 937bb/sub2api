<template>
  <AppLayout>
    <WorkspacePage variant="finance">
      <!-- Header with Day Switcher -->
      <template #actions>
        <div class="flex flex-wrap items-center gap-2">
          <div class="finance-period" role="group" :aria-label="t('payment.admin.dailyRevenue')">
            <button
              v-for="d in DAYS_OPTIONS"
              :key="d"
              type="button"
              :aria-pressed="days === d"
              :disabled="loading"
              @click="days = d"
            >
              {{ d }}{{ t('payment.admin.daySuffix') }}
            </button>
          </div>
          <button @click="loadDashboard" :disabled="loading" class="btn btn-secondary" :aria-label="t('common.refresh')" :title="t('common.refresh')">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </template>

      <!-- Dashboard Content -->
      <div v-if="loading && !stats" class="finance-loading" role="status" :aria-label="t('common.loading')">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <OrderStatsCards :stats="stats" />
        <div class="finance-analysis" :aria-busy="loading">
          <DailyRevenueChart :data="stats.daily_series || []" />
          <div class="finance-breakdown">
            <PaymentMethodChart :methods="stats.payment_methods || []" />
          <section class="finance-ranking">
            <header class="finance-panel-heading">
              <h3>{{ t('payment.admin.topUsers') }}</h3>
            </header>
            <div v-if="!hasTopUsers(stats.top_users)" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
            <div v-else class="finance-ranking-list">
              <div v-for="[currency, users] in sortedTopUsers(stats.top_users)" :key="currency" class="space-y-2">
                <p class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ currency }}</p>
                <div v-for="(user, idx) in users" :key="user.user_id" class="finance-ranking-row">
                  <div class="finance-ranking-identity">
                    <span class="finance-ranking-position">{{ idx + 1 }}</span>
                    <span class="finance-ranking-email" :title="user.email">{{ user.email }}</span>
                  </div>
                  <span class="font-medium">{{ formatMoney(currency, user.amount) }}</span>
                </div>
              </div>
            </div>
          </section>
          </div>
        </div>
      </template>
    </WorkspacePage>
  </AppLayout>
</template>

<script setup lang="ts">
import WorkspacePage from '@/components/layout/WorkspacePage.vue'

import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { DashboardStats, TopUserPaymentStats } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatsCards from '@/components/admin/payment/OrderStatsCards.vue'
import DailyRevenueChart from '@/components/admin/payment/DailyRevenueChart.vue'
import PaymentMethodChart from '@/components/admin/payment/PaymentMethodChart.vue'

const { t } = useI18n()
const appStore = useAppStore()

const DAYS_OPTIONS = [7, 30, 90] as const
const days = ref<number>(30)
const loading = ref(false)
const stats = ref<DashboardStats | null>(null)

function sortedTopUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): [string, TopUserPaymentStats[]][] {
  return Object.entries(usersByCurrency).sort(([left], [right]) => left.localeCompare(right))
}

function hasTopUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): boolean {
  return Object.values(usersByCurrency).some(users => users.length > 0)
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}

async function loadDashboard() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getDashboard(days.value)
    stats.value = res.data
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

watch(days, () => loadDashboard())
onMounted(() => loadDashboard())
</script>
