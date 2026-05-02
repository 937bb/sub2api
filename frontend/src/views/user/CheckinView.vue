<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>

      <!-- Disabled -->
      <div v-else-if="!status?.enabled" class="py-16 text-center">
        <EmptyState :message="t('checkin.disabled')" />
      </div>

      <!-- Main Content -->
      <template v-else>
        <!-- Checkin Card -->
        <div class="card overflow-hidden">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('checkin.title') }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('checkin.description') }}
            </p>
          </div>

          <div class="p-6">
            <!-- Stats Row -->
            <div class="mb-6 grid grid-cols-3 gap-4">
              <div class="text-center">
                <p class="text-2xl font-bold text-primary-600 dark:text-primary-400">{{ status?.current_streak ?? 0 }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('checkin.streak') }} ({{ t('checkin.days') }})</p>
              </div>
              <div class="text-center">
                <p class="text-2xl font-bold text-gray-900 dark:text-white">{{ status?.total_checkins ?? 0 }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('checkin.totalCheckins') }}</p>
              </div>
              <div class="text-center">
                <p class="text-2xl font-bold text-amber-600 dark:text-amber-400">
                  <template v-if="status?.config?.mode === 'random'">
                    {{ Number(status.config.random_min).toFixed(2) }}~{{ Number(status.config.random_max).toFixed(2) }}
                  </template>
                  <template v-else>
                    {{ status?.config?.fixed_amount?.toFixed(2) ?? '0.00' }}
                  </template>
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ status?.config?.mode === 'random' ? t('checkin.randomMode') : t('checkin.fixedMode') }}
                </p>
              </div>
            </div>

            <!-- Checkin Button -->
            <div class="flex justify-center">
              <button
                v-if="!status?.checked_in_today"
                class="btn btn-primary px-10 py-3 text-base font-semibold shadow-glow transition-all hover:scale-105"
                :disabled="checkinLoading"
                @click="doCheckin"
              >
                <template v-if="checkinLoading">
                  <LoadingSpinner class="mr-2 inline h-4 w-4" />
                </template>
                {{ t('checkin.button') }}
              </button>
              <div v-else class="flex flex-col items-center gap-2">
                <div class="flex h-16 w-16 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
                  <svg class="h-8 w-8 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                </div>
                <p class="text-sm font-medium text-green-600 dark:text-green-400">
                  {{ t('checkin.alreadyCheckedIn') }}
                </p>
              </div>
            </div>

            <!-- Last Checkin Result -->
            <div v-if="lastResult" class="mt-6 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800/50 dark:bg-green-900/20">
              <h3 class="mb-2 text-sm font-semibold text-green-800 dark:text-green-300">
                {{ t('checkin.todayReward') }}
              </h3>
              <div class="space-y-1 text-sm text-green-700 dark:text-green-400">
                <div class="flex justify-between">
                  <span>{{ t('checkin.baseReward') }}</span>
                  <span class="font-medium">+${{ lastResult.base_amount.toFixed(4) }}</span>
                </div>
                <div v-if="lastResult.milestone_amount > 0" class="flex justify-between">
                  <span>{{ t('checkin.milestoneReward') }}</span>
                  <span class="font-medium">+${{ lastResult.milestone_amount.toFixed(4) }}</span>
                </div>
                <div class="flex justify-between border-t border-green-300 pt-1 font-semibold dark:border-green-700">
                  <span>{{ t('common.total') }}</span>
                  <span>+${{ lastResult.total_amount.toFixed(4) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Milestones Preview -->
        <div v-if="status?.config?.milestones && status.config.milestones.length > 0" class="card overflow-hidden">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('checkin.milestones') }}
            </h2>
          </div>
          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <div
              v-for="ms in status.config.milestones"
              :key="ms.days"
              class="flex items-center justify-between px-6 py-3"
              :class="(status?.current_streak ?? 0) >= ms.days ? 'bg-green-50/50 dark:bg-green-900/10' : ''"
            >
              <div class="flex items-center gap-3">
                <div
                  class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold"
                  :class="(status?.current_streak ?? 0) >= ms.days
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                    : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'"
                >
                  {{ ms.days }}
                </div>
                <span class="text-sm text-gray-700 dark:text-gray-300">
                  {{ t('checkin.milestoneDays', { days: ms.days }) }}
                </span>
              </div>
              <div class="text-right">
                <span class="text-sm font-semibold text-primary-600 dark:text-primary-400">+${{ ms.amount.toFixed(2) }}</span>
                <span class="ml-1 text-xs text-gray-400">
                  {{ ms.balance_type === 'expirable' ? t('checkin.balanceType.expirable') : t('checkin.balanceType.permanent') }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- Calendar -->
        <div class="card overflow-hidden">
          <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('checkin.calendar') }}
            </h2>
            <div class="flex items-center gap-2">
              <button class="rounded p-1 hover:bg-gray-100 dark:hover:bg-dark-700" @click="prevMonth">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
                </svg>
              </button>
              <span class="min-w-[120px] text-center text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ calendarYear }}-{{ String(calendarMonth).padStart(2, '0') }}
              </span>
              <button class="rounded p-1 hover:bg-gray-100 dark:hover:bg-dark-700" @click="nextMonth">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </button>
            </div>
          </div>
          <div class="p-4">
            <!-- Weekday headers -->
            <div class="mb-2 grid grid-cols-7 gap-1 text-center">
              <div v-for="d in weekDays" :key="d" class="text-xs font-medium text-gray-400 dark:text-gray-500">{{ d }}</div>
            </div>
            <!-- Calendar grid -->
            <div class="grid grid-cols-7 gap-1">
              <div v-for="(cell, idx) in calendarCells" :key="idx" class="flex aspect-square items-center justify-center">
                <template v-if="cell.day > 0">
                  <div
                    class="flex h-8 w-8 items-center justify-center rounded-full text-xs"
                    :class="cell.checkedIn
                      ? 'bg-primary-500 font-semibold text-white'
                      : cell.isToday
                        ? 'border border-primary-400 font-medium text-primary-600 dark:text-primary-400'
                        : 'text-gray-600 dark:text-gray-400'"
                  >
                    {{ cell.day }}
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { checkinAPI } from '@/api'
import type { CheckinRecord, CheckinStatus } from '@/api/checkin'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const checkinLoading = ref(false)
const status = ref<CheckinStatus | null>(null)
const lastResult = ref<CheckinRecord | null>(null)

// Calendar state
const now = new Date()
const calendarYear = ref(now.getFullYear())
const calendarMonth = ref(now.getMonth() + 1)
const calendarRecords = ref<CheckinRecord[]>([])

const weekDays = computed(() => {
  const locale = useI18n().locale.value
  return locale === 'zh'
    ? ['一', '二', '三', '四', '五', '六', '日']
    : ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
})

interface CalendarCell {
  day: number
  checkedIn: boolean
  isToday: boolean
}

const calendarCells = computed<CalendarCell[]>(() => {
  const year = calendarYear.value
  const month = calendarMonth.value
  const firstDay = new Date(year, month - 1, 1)
  const daysInMonth = new Date(year, month, 0).getDate()

  // Monday = 0, Sunday = 6
  let startDow = firstDay.getDay() - 1
  if (startDow < 0) startDow = 6

  const checkedInDates = new Set(calendarRecords.value.map(r => r.checkin_date))
  const todayStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`

  const cells: CalendarCell[] = []

  // Empty cells before first day
  for (let i = 0; i < startDow; i++) {
    cells.push({ day: 0, checkedIn: false, isToday: false })
  }

  for (let d = 1; d <= daysInMonth; d++) {
    const dateStr = `${year}-${String(month).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    cells.push({
      day: d,
      checkedIn: checkedInDates.has(dateStr),
      isToday: dateStr === todayStr,
    })
  }

  return cells
})

async function fetchStatus() {
  try {
    status.value = await checkinAPI.getStatus()
  } catch (e) {
    console.error('Failed to load checkin status', e)
  }
}

async function fetchCalendar() {
  try {
    const result = await checkinAPI.getCalendar(calendarYear.value, calendarMonth.value)
    calendarRecords.value = result.records || []
  } catch (e) {
    console.error('Failed to load calendar', e)
    calendarRecords.value = []
  }
}

async function doCheckin() {
  checkinLoading.value = true
  try {
    const result = await checkinAPI.checkin()
    lastResult.value = result
    appStore.showSuccess(t('checkin.success'))
    // Refresh status
    await fetchStatus()
    await fetchCalendar()
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || 'Check-in failed'
    appStore.showError(msg)
  } finally {
    checkinLoading.value = false
  }
}

function prevMonth() {
  if (calendarMonth.value === 1) {
    calendarMonth.value = 12
    calendarYear.value--
  } else {
    calendarMonth.value--
  }
  fetchCalendar()
}

function nextMonth() {
  if (calendarMonth.value === 12) {
    calendarMonth.value = 1
    calendarYear.value++
  } else {
    calendarMonth.value++
  }
  fetchCalendar()
}

onMounted(async () => {
  loading.value = true
  await fetchStatus()
  await fetchCalendar()
  loading.value = false
})
</script>
