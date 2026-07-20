<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-5">
      <div class="grid gap-4 sm:grid-cols-3">
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('growth.checkin.currentStreak') }}</p>
          <p class="mt-2 text-3xl font-semibold text-gray-900 dark:text-white">{{ status?.current_streak ?? 0 }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('growth.checkin.totalDays') }}</p>
          <p class="mt-2 text-3xl font-semibold text-gray-900 dark:text-white">{{ status?.total_checkins ?? 0 }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('growth.checkin.todayReward') }}</p>
          <p class="mt-2 text-3xl font-semibold text-emerald-600 dark:text-emerald-400">{{ rewardLabel }}</p>
        </div>
      </div>

      <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_300px]">
        <section class="card overflow-hidden">
          <header class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <button class="btn btn-secondary btn-sm" :title="t('growth.checkin.previousMonth')" @click="moveMonth(-1)">
              <Icon name="chevronLeft" size="sm" />
            </button>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ monthTitle }}</h2>
            <button class="btn btn-secondary btn-sm" :title="t('growth.checkin.nextMonth')" @click="moveMonth(1)">
              <Icon name="chevronRight" size="sm" />
            </button>
          </header>
          <div class="grid grid-cols-7 border-b border-gray-100 bg-gray-50 text-center text-xs font-medium text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
            <div v-for="weekday in weekdays" :key="weekday" class="py-3">{{ weekday }}</div>
          </div>
          <div class="grid grid-cols-7">
            <div
              v-for="cell in calendarCells"
              :key="cell.key"
              class="relative aspect-square min-h-16 border-b border-r border-gray-100 p-2 dark:border-dark-700"
              :class="cell.inMonth ? 'bg-white dark:bg-dark-900' : 'bg-gray-50 text-gray-300 dark:bg-dark-800/50 dark:text-dark-600'"
            >
              <span class="text-xs" :class="cell.isToday ? 'font-semibold text-primary-600 dark:text-primary-400' : ''">{{ cell.day }}</span>
              <div v-if="cell.checkin" class="mt-1 flex flex-col items-center text-center">
                <span class="flex h-7 w-7 items-center justify-center rounded-full bg-emerald-100 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400">
                  <Icon name="check" size="sm" />
                </span>
                <span class="mt-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">+{{ formatAmount(cell.checkin.total_reward) }}</span>
              </div>
            </div>
          </div>
        </section>

        <aside class="space-y-4">
          <div class="card p-5">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('growth.checkin.actionTitle') }}</h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ actionDescription }}</p>
            <button
              class="btn btn-primary mt-4 w-full"
              :disabled="claiming || !status?.config.checkin_enabled || status.checked_in_today"
              @click="checkin"
            >
              <Icon :name="status?.checked_in_today ? 'checkCircle' : 'calendar'" size="sm" />
              <span>{{ status?.checked_in_today ? t('growth.checkin.completed') : t('growth.checkin.claim') }}</span>
            </button>
          </div>

          <div v-if="status?.config.checkin_streak_rewards.length" class="card p-5">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('growth.checkin.streakRewards') }}</h3>
            <div class="mt-3 space-y-2">
              <div v-for="reward in status.config.checkin_streak_rewards" :key="reward.days" class="flex items-center justify-between text-sm">
                <span class="text-gray-600 dark:text-dark-300">{{ t('growth.checkin.streakDay', { days: reward.days }) }}</span>
                <span class="font-medium text-emerald-600 dark:text-emerald-400">+{{ formatAmount(reward.amount) }}</span>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { claimCheckin, getCheckinStatus, type GrowthCheckin, type GrowthCheckinStatus } from '@/api/growth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { getBrowserDeviceID } from '@/utils/deviceIdentity'

const { t, locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const currentMonth = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1))
const status = ref<GrowthCheckinStatus | null>(null)
const claiming = ref(false)
const weekdays = computed(() => {
  const base = new Date(2024, 0, 7)
  return Array.from({ length: 7 }, (_, i) => new Intl.DateTimeFormat(locale.value, { weekday: 'short' }).format(new Date(base.getFullYear(), base.getMonth(), base.getDate() + i)))
})
const monthKey = computed(() => `${currentMonth.value.getFullYear()}-${String(currentMonth.value.getMonth() + 1).padStart(2, '0')}`)
const monthTitle = computed(() => new Intl.DateTimeFormat(locale.value, { year: 'numeric', month: 'long' }).format(currentMonth.value))
const rewardLabel = computed(() => {
  const config = status.value?.config
  if (!config) return '$0.00'
  return config.checkin_reward_mode === 'random'
    ? `$${formatAmount(config.checkin_min_reward)} - $${formatAmount(config.checkin_max_reward)}`
    : `$${formatAmount(config.checkin_fixed_reward)}`
})
const actionDescription = computed(() => {
  if (!status.value?.config.checkin_enabled) return t('growth.checkin.disabled')
  if (status.value.checked_in_today) return t('growth.checkin.doneDescription')
  return t('growth.checkin.readyDescription')
})
const calendarCells = computed(() => {
  const year = currentMonth.value.getFullYear()
  const month = currentMonth.value.getMonth()
  const firstWeekday = new Date(year, month, 1).getDay()
  const gridStart = new Date(year, month, 1 - firstWeekday)
  const checkins = new Map((status.value?.month_checkins || []).map((item) => [item.date, item]))
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + index)
    const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
    return { key, day: date.getDate(), inMonth: date.getMonth() === month, isToday: key === status.value?.today, checkin: checkins.get(key) as GrowthCheckin | undefined }
  })
})

function formatAmount(value: number): string { return Number(value || 0).toFixed(2) }
async function load(): Promise<void> {
  try { status.value = await getCheckinStatus(monthKey.value) }
  catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.loadFailed'))) }
}
async function moveMonth(delta: number): Promise<void> {
  currentMonth.value = new Date(currentMonth.value.getFullYear(), currentMonth.value.getMonth() + delta, 1)
  await load()
}
async function checkin(): Promise<void> {
  if (claiming.value) return
  claiming.value = true
  try {
    const result = await claimCheckin({ device_id: await getBrowserDeviceID() })
    appStore.showSuccess(t('growth.checkin.success', { amount: formatAmount(result.total_reward) }))
    await Promise.all([load(), authStore.refreshUser().catch(() => undefined)])
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'growth.checkin.errors', t('growth.checkin.failed')))
  } finally {
    claiming.value = false
  }
}
onMounted(() => void load())
</script>
