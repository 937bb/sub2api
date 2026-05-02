<template>
  <AppLayout>
  <div class="mx-auto max-w-4xl space-y-6 p-4 sm:p-6 lg:p-8">
    <!-- Header -->
    <div class="animate-fade-in">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('leaderboard.title') }}</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('leaderboard.description') }}</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-20">
      <LoadingSpinner />
    </div>

    <!-- Disabled state -->
    <div v-else-if="!data?.enabled" class="animate-fade-in">
      <EmptyState :title="t('leaderboard.disabled')" />
    </div>

    <!-- Content -->
    <div v-else class="space-y-8 animate-fade-in">
      <!-- Tab switcher -->
      <div class="flex gap-2">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="rounded-lg px-4 py-2 text-sm font-medium transition-all"
          :class="activeTab === tab.key
            ? 'bg-primary-500 text-white shadow-glow'
            : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- Table -->
      <div class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-card dark:border-dark-700 dark:bg-dark-800">
        <table class="w-full">
          <thead>
            <tr class="border-b border-gray-100 bg-gray-50/50 dark:border-dark-700 dark:bg-dark-900/50">
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('leaderboard.rank') }}
              </th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('leaderboard.email') }}
              </th>
              <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('leaderboard.actualCost') }}
              </th>
              <th class="hidden px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">
                {{ t('leaderboard.requests') }}
              </th>
              <th class="hidden px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">
                {{ t('leaderboard.tokens') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr
              v-for="entry in currentEntries"
              :key="entry.rank"
              class="transition-colors hover:bg-gray-50/50 dark:hover:bg-dark-700/50"
            >
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span
                    v-if="entry.rank <= 3"
                    class="inline-flex h-7 w-7 items-center justify-center rounded-full text-sm font-bold"
                    :class="{
                      'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400': entry.rank === 1,
                      'bg-gray-100 text-gray-600 dark:bg-gray-700/50 dark:text-gray-300': entry.rank === 2,
                      'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400': entry.rank === 3,
                    }"
                  >
                    {{ entry.rank === 1 ? '🥇' : entry.rank === 2 ? '🥈' : '🥉' }}
                  </span>
                  <span v-else class="inline-flex h-7 w-7 items-center justify-center text-sm font-medium text-gray-500 dark:text-gray-400">
                    {{ entry.rank }}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
                {{ entry.email }}
              </td>
              <td class="px-4 py-3 text-right text-sm font-semibold text-primary-600 dark:text-primary-400">
                {{ t('leaderboard.unit') }}{{ entry.actual_cost.toFixed(4) }}
              </td>
              <td class="hidden px-4 py-3 text-right text-sm text-gray-600 dark:text-gray-300 sm:table-cell">
                {{ entry.requests.toLocaleString() }}
              </td>
              <td class="hidden px-4 py-3 text-right text-sm text-gray-600 dark:text-gray-300 md:table-cell">
                {{ entry.tokens.toLocaleString() }}
              </td>
            </tr>
          </tbody>
        </table>

        <!-- Empty -->
        <div v-if="currentEntries.length === 0" class="py-12 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ t('leaderboard.noData') }}
        </div>
      </div>
    </div>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { leaderboardAPI, type LeaderboardResponse } from '@/api/leaderboard'
import AppLayout from '@/components/layout/AppLayout.vue'
import { LoadingSpinner, EmptyState } from '@/components/common'

const { t } = useI18n()

const loading = ref(true)
const data = ref<LeaderboardResponse | null>(null)
const activeTab = ref<'yesterday' | 'total'>('yesterday')

const tabs = computed(() => [
  { key: 'yesterday' as const, label: t('leaderboard.yesterday') },
  { key: 'total' as const, label: t('leaderboard.total') },
])

const currentEntries = computed(() => {
  if (!data.value) return []
  return activeTab.value === 'yesterday' ? data.value.yesterday : data.value.total
})

onMounted(async () => {
  try {
    data.value = await leaderboardAPI.getLeaderboard()
  } catch {
    // silently fail
  } finally {
    loading.value = false
  }
})
</script>
