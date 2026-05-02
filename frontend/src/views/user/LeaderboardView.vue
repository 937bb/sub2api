<template>
  <AppLayout>
  <div class="space-y-6 p-4 sm:p-6 lg:p-8">
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

    <!-- Three-column layout -->
    <div v-else class="grid grid-cols-1 gap-6 animate-fade-in lg:grid-cols-3">
      <div
        v-for="col in columns"
        :key="col.key"
        class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-card dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="border-b border-gray-100 bg-gray-50/50 px-5 py-3 dark:border-dark-700 dark:bg-dark-900/50">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ col.title }}</h2>
        </div>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="entry in col.entries"
            :key="col.key + '-' + entry.rank"
            class="flex items-center gap-3 px-5 py-2.5 transition-colors hover:bg-gray-50/50 dark:hover:bg-dark-700/50"
          >
            <span
              v-if="entry.rank <= 3"
              class="inline-flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full text-sm font-bold"
              :class="{
                'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400': entry.rank === 1,
                'bg-gray-100 text-gray-600 dark:bg-gray-700/50 dark:text-gray-300': entry.rank === 2,
                'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400': entry.rank === 3,
              }"
            >
              {{ entry.rank === 1 ? '🥇' : entry.rank === 2 ? '🥈' : '🥉' }}
            </span>
            <span v-else class="inline-flex h-7 w-7 flex-shrink-0 items-center justify-center text-sm font-medium text-gray-400 dark:text-gray-500">
              {{ entry.rank }}
            </span>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm text-gray-900 dark:text-white">{{ entry.email }}</p>
              <p class="text-xs text-gray-400 dark:text-gray-500">
                {{ entry.requests.toLocaleString() }} {{ t('leaderboard.requests') }}
                · {{ entry.tokens.toLocaleString() }} {{ t('leaderboard.tokens') }}
              </p>
            </div>
            <span class="flex-shrink-0 text-sm font-semibold text-primary-600 dark:text-primary-400">
              {{ t('leaderboard.unit') }}{{ entry.actual_cost.toFixed(4) }}
            </span>
          </div>
        </div>
        <div v-if="!col.entries.length" class="py-10 text-center text-sm text-gray-400 dark:text-gray-500">
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

const columns = computed(() => [
  { key: 'today', title: t('leaderboard.today'), entries: data.value?.today || [] },
  { key: 'yesterday', title: t('leaderboard.yesterday'), entries: data.value?.yesterday || [] },
  { key: 'total', title: t('leaderboard.total'), entries: data.value?.total || [] },
])

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
