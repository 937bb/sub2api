<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Summary Cards -->
      <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div class="card p-4 text-center">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('balanceEntries.summary.totalBalance') }}
          </p>
          <p class="mt-1 text-xl font-bold text-gray-900 dark:text-white">
            ${{ summaryData?.total_balance?.toFixed(4) ?? '0.0000' }}
          </p>
        </div>
        <div class="card p-4 text-center">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('balanceEntries.summary.permanentBalance') }}
          </p>
          <p class="mt-1 text-xl font-bold text-primary-600 dark:text-primary-400">
            ${{ summaryData?.permanent_balance?.toFixed(4) ?? '0.0000' }}
          </p>
        </div>
        <div class="card p-4 text-center">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('balanceEntries.summary.expirableBalance') }}
          </p>
          <p class="mt-1 text-xl font-bold text-amber-600 dark:text-amber-400">
            ${{ summaryData?.expirable_balance?.toFixed(4) ?? '0.0000' }}
          </p>
        </div>
        <div class="card p-4 text-center">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('balanceEntries.summary.expiringSoon') }}
          </p>
          <p class="mt-1 text-xl font-bold" :class="(summaryData?.expiring_soon ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'">
            ${{ summaryData?.expiring_soon?.toFixed(4) ?? '0.0000' }}
          </p>
          <p v-if="summaryData?.warning_days" class="mt-0.5 text-[10px] text-gray-400 dark:text-gray-500">
            {{ t('balanceEntries.summary.withinDays', { days: summaryData.warning_days }) }}
          </p>
        </div>
      </div>

      <!-- Entries Table -->
      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('balanceEntries.title') }}
          </h2>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('balanceEntries.description') }}
          </p>
        </div>

        <!-- Loading -->
        <div v-if="loading" class="flex items-center justify-center py-16">
          <LoadingSpinner />
        </div>

        <!-- Empty -->
        <div v-else-if="entries.length === 0" class="py-16 text-center">
          <EmptyState :message="t('balanceEntries.empty')" />
        </div>

        <!-- Table -->
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-gray-100 bg-gray-50/50 dark:border-dark-700 dark:bg-dark-800/50">
              <tr>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.createdAt') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.source') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.amount') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.type') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.expiresAt') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.status') }}</th>
                <th class="px-4 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('balanceEntries.table.note') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="entry in entries" :key="entry.id" class="hover:bg-gray-50/50 dark:hover:bg-dark-800/30">
                <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                  {{ formatDateTime(entry.created_at) }}
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="sourceClass(entry.source)">
                    {{ sourceLabel(entry.source) }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-sm font-medium" :class="entry.amount >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                  {{ entry.amount >= 0 ? '+' : '' }}{{ entry.amount.toFixed(4) }}
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="entry.balance_type === 'permanent'
                      ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                      : 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'">
                    {{ t(`balanceEntries.type.${entry.balance_type}`) }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                  {{ entry.expires_at ? formatDateTime(entry.expires_at) : t('balanceEntries.noExpiration') }}
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="statusClass(entry)">
                    {{ statusLabel(entry) }}
                  </span>
                </td>
                <td class="max-w-[200px] truncate px-4 py-3 text-xs text-gray-500 dark:text-gray-400" :title="entry.note">
                  {{ entry.note || '-' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="border-t border-gray-100 px-6 py-3 dark:border-dark-700">
          <Pagination
            :page="currentPage"
            :total="totalItems"
            :page-size="pageSize"
            @update:page="handlePageChange"
          />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { balanceEntriesAPI } from '@/api'
import type { BalanceEntry, BalanceSummary } from '@/api/balanceEntries'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()

const loading = ref(true)
const entries = ref<BalanceEntry[]>([])
const summaryData = ref<BalanceSummary | null>(null)
const currentPage = ref(1)
const pageSize = ref(20)
const totalItems = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(totalItems.value / pageSize.value)))

async function fetchEntries() {
  loading.value = true
  try {
    const result = await balanceEntriesAPI.list(currentPage.value, pageSize.value)
    entries.value = result.items || []
    totalItems.value = result.total || 0
  } catch (e) {
    console.error('Failed to load balance entries', e)
    entries.value = []
  } finally {
    loading.value = false
  }
}

async function fetchSummary() {
  try {
    summaryData.value = await balanceEntriesAPI.summary()
  } catch (e) {
    console.error('Failed to load balance summary', e)
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchEntries()
}

function sourceLabel(source: string): string {
  const key = `balanceEntries.source.${source}`
  const val = t(key)
  return val === key ? source : val
}

function sourceClass(source: string): string {
  const map: Record<string, string> = {
    consumption: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    expiry_clear: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
    refund: 'bg-orange-50 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    admin: 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    redeem: 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    promo: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    affiliate: 'bg-teal-50 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300',
    checkin: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
    cashback: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  }
  return map[source] || 'bg-gray-50 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
}

function statusLabel(entry: BalanceEntry): string {
  if (entry.expired) return t('balanceEntries.status.expired')
  if (entry.remaining <= 0 && entry.amount > 0) return t('balanceEntries.status.depleted')
  return t('balanceEntries.status.active')
}

function statusClass(entry: BalanceEntry): string {
  if (entry.expired) return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (entry.remaining <= 0 && entry.amount > 0) return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

onMounted(() => {
  fetchSummary()
  fetchEntries()
})
</script>
