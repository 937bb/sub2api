<template>
  <AppLayout>
    <div class="w-full space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="inline-flex rounded-md border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900">
          <button v-for="option in periods" :key="option.value" class="rounded px-4 py-2 text-sm font-medium transition-colors" :class="period === option.value ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'" @click="changePeriod(option.value)">
            {{ option.label }}
          </button>
        </div>
        <p v-if="data" class="text-sm text-gray-500 dark:text-dark-400">{{ formatDate(data.period_start) }} - {{ formatPeriodEnd(data.period_end) }}</p>
      </div>

      <div v-if="loading" class="flex justify-center py-16"><Icon name="refresh" size="lg" class="animate-spin text-primary-500" /></div>
      <template v-else-if="data">
        <div v-if="data.current_user" class="border-y border-primary-200 bg-primary-50 px-5 py-4 dark:border-primary-900/50 dark:bg-primary-900/15">
          <div class="flex items-center justify-between gap-4">
            <div><p class="text-xs font-medium uppercase text-primary-600 dark:text-primary-400">{{ t('growth.leaderboard.yourRank') }}</p><p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">#{{ data.current_user.rank }}</p></div>
            <div class="text-right"><p class="text-sm text-gray-500 dark:text-dark-400">{{ t('growth.leaderboard.spend') }}</p><p class="text-lg font-semibold text-primary-600 dark:text-primary-400">${{ formatAmount(data.current_user.actual_cost) }}</p></div>
          </div>
        </div>

        <div v-if="data.items.length" class="card overflow-x-auto">
          <table class="w-full min-w-[560px] text-left text-sm">
            <thead class="border-b border-gray-100 bg-gray-50 text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
              <tr><th class="px-5 py-3">#</th><th class="px-5 py-3">{{ t('growth.leaderboard.user') }}</th><th class="px-5 py-3 text-right">{{ t('growth.leaderboard.requests') }}</th><th class="px-5 py-3 text-right">{{ t('growth.leaderboard.spend') }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="item in data.items" :key="`${item.rank}-${item.display_name}`" class="border-b border-gray-100 last:border-b-0 dark:border-dark-800" :class="item.is_current_user ? 'bg-primary-50/70 dark:bg-primary-900/10' : ''">
                <td class="px-5 py-4"><span class="inline-flex h-7 min-w-7 items-center justify-center rounded-full px-2 text-xs font-semibold" :class="rankClass(item.rank)">{{ item.rank }}</span></td>
                <td class="break-all px-5 py-4 font-medium text-gray-900 dark:text-white" :title="item.display_name">{{ item.display_name }}</td>
                <td class="px-5 py-4 text-right text-gray-600 dark:text-dark-300">{{ item.requests.toLocaleString() }}</td>
                <td class="px-5 py-4 text-right font-semibold text-gray-900 dark:text-white">${{ formatAmount(item.actual_cost) }}</td>
              </tr>
            </tbody>
          </table>
          <div class="border-t border-gray-100 px-5 py-3 text-right text-xs text-gray-500 dark:border-dark-800 dark:text-dark-400">
            {{ t('growth.leaderboard.visibleTop', { count: data.page_size }) }}
          </div>
        </div>
        <div v-else class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.noData') }}</div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getLeaderboard, type GrowthLeaderboard, type GrowthPeriod } from '@/api/growth'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const period = ref<GrowthPeriod>('daily')
const data = ref<GrowthLeaderboard | null>(null)
const loading = ref(false)
const periods = [
  { value: 'daily' as const, label: t('growth.period.daily') },
  { value: 'weekly' as const, label: t('growth.period.weekly') },
  { value: 'monthly' as const, label: t('growth.period.monthly') },
]
function formatAmount(value: number): string { return Number(value || 0).toFixed(4) }
function formatDate(value: string): string { return new Date(value).toLocaleDateString() }
function formatPeriodEnd(value: string): string { return new Date(new Date(value).getTime() - 1).toLocaleDateString() }
function rankClass(rank: number): string {
  if (rank === 1) return 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300'
  if (rank === 2) return 'bg-gray-200 text-gray-700 dark:bg-gray-500/20 dark:text-gray-300'
  if (rank === 3) return 'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
}
async function load(): Promise<void> {
  loading.value = true
  try { data.value = await getLeaderboard(period.value) }
  catch (error) { data.value = null; appStore.showError(extractApiErrorMessage(error, t('growth.loadFailed'))) }
  finally { loading.value = false }
}
function changePeriod(value: GrowthPeriod): void { period.value = value; void load() }
void load()
</script>
