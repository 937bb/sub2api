<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="overflow-hidden rounded-2xl border bg-white dark:bg-dark-800"
          :class="platformBorderClass(subscription.group?.platform || '')"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group.description }}
                </p>
                <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-gray-400 dark:text-gray-500">
                  <span>{{ t('payment.planCard.rate') }}: ×{{ subscription.group?.rate_multiplier ?? 1 }}</span>
                  <span v-if="subscriptionHasPeakRate(subscription)" class="text-amber-700 dark:text-amber-300">
                    {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(subscription) }}
                  </span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="subscription.status === 'active'"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
              >
                {{ t('payment.renewNow') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.daily_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.daily_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{ formatDailyUsageWindow(subscription) }}
              </p>
              <button
                v-if="canPreviewDailyQuotaAdvance(subscription, now)"
                type="button"
                class="inline-flex min-h-9 items-center justify-center gap-1.5 self-end rounded-md border border-amber-300 px-3 py-1.5 text-xs font-semibold text-amber-700 transition-colors hover:bg-amber-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-amber-700 dark:text-amber-300 dark:hover:bg-amber-900/20"
                :disabled="advancingQuota"
                @click="openAdvanceDailyQuotaDialog(subscription)"
              >
                <Icon name="refresh" size="sm" />
                {{ t('userSubscriptions.advanceDailyQuota') }}
              </button>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.weekly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.weekly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(quotaResetAt(subscription, 'weekly'))
                  })
                }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.monthly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.monthly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(quotaResetAt(subscription, 'monthly'))
                  })
                }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="selectedAdvanceSubscription !== null"
      :title="t('userSubscriptions.advanceDailyQuotaTitle')"
      :message="advanceDailyQuotaMessage"
      :confirm-text="advancingQuota ? t('userSubscriptions.advancingDailyQuota') : t('userSubscriptions.advanceDailyQuota')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      :confirm-disabled="advanceConfirmDisabled"
      @confirm="confirmAdvanceDailyQuota"
      @cancel="closeAdvanceDailyQuotaDialog"
    >
      <div v-if="advancePreviewLoading" class="py-4 text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('userSubscriptions.loadingAdvancePreview') }}
      </div>

      <template v-else-if="advancePreview">
        <div
          v-if="advancePreview.blockers.length > 0"
          class="border-l-4 border-red-500 bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300"
          role="alert"
        >
          <p class="font-semibold">{{ t('userSubscriptions.advanceUnavailable') }}</p>
          <ul class="mt-1 list-disc space-y-1 pl-5">
            <li v-for="blocker in advancePreview.blockers" :key="blocker">
              {{ advanceBlockerLabel(blocker) }}
            </li>
          </ul>
        </div>

        <dl class="divide-y divide-gray-200 border-y border-gray-200 text-sm dark:divide-dark-600 dark:border-dark-600">
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.dailyQuota') }}</dt>
            <dd class="text-right font-medium text-gray-900 dark:text-white">
              {{ formatUSD(advancePreview.daily_limit_usd) }}
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.naturalDailyReset') }}</dt>
            <dd class="text-right text-gray-900 dark:text-white">
              {{ formatPreviewReset(advancePreview.daily_resets_at) }}
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.weeklyRemaining') }}</dt>
            <dd class="text-right text-gray-900 dark:text-white">
              <div>{{ formatQuotaRemaining(advancePreview.weekly_remaining_usd, advancePreview.weekly_limit_usd) }}</div>
              <div v-if="advancePreview.weekly_limit_usd !== null" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                {{ formatPreviewReset(advancePreview.weekly_resets_at) }}
              </div>
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.monthlyRemaining') }}</dt>
            <dd class="text-right text-gray-900 dark:text-white">
              <div>{{ formatQuotaRemaining(advancePreview.monthly_remaining_usd, advancePreview.monthly_limit_usd) }}</div>
              <div v-if="advancePreview.monthly_limit_usd !== null" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                {{ formatPreviewReset(advancePreview.monthly_resets_at) }}
              </div>
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.recoverableQuota') }}</dt>
            <dd class="text-right font-semibold text-gray-900 dark:text-white">
              {{ formatUSD(advancePreview.recoverable_usd) }}
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.validityDeduction') }}</dt>
            <dd class="text-right font-semibold text-red-600 dark:text-red-400">
              {{ t('userSubscriptions.hours', { hours: advancePreview.deduct_hours }) }}
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.currentExpiry') }}</dt>
            <dd class="text-right text-gray-900 dark:text-white">
              {{ formatDateTimeToMinute(advancePreview.current_expires_at) }}
            </dd>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] gap-3 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.expiryAfterAdvance') }}</dt>
            <dd class="text-right font-medium text-gray-900 dark:text-white">
              {{ formatDateTimeToMinute(advancePreview.expires_at_after) }}
            </dd>
          </div>
        </dl>

        <p v-if="advancePreview.can_advance" class="text-xs font-medium text-amber-700 dark:text-amber-300">
          {{ t('userSubscriptions.periodUsagePreservedWarning') }}
        </p>
      </template>
    </ConfirmDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI, { type DailyQuotaAdvancePreview } from '@/api/subscriptions'
import { useSubscriptionStore } from '@/stores/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import {
  getExpirationDateRelation,
  getRemainingDurationParts,
  isOneTimeDailyQuota,
  canPreviewDailyQuotaAdvance,
  type RemainingDurationParts
} from '@/utils/subscriptionQuota'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(true)
const now = ref(new Date())
const selectedAdvanceSubscription = ref<UserSubscription | null>(null)
const advancePreview = ref<DailyQuotaAdvancePreview | null>(null)
const advancePreviewLoading = ref(false)
const advancingQuota = ref(false)
let clockTimer: ReturnType<typeof setInterval> | undefined
let previousClockTime = Date.now()
let refreshingWindowBoundary = false

const advanceDailyQuotaMessage = computed(() => {
  if (advancePreviewLoading.value) return t('userSubscriptions.advancePreviewLoadingMessage')
  if (advancePreview.value?.can_advance) return t('userSubscriptions.advanceDailyQuotaConfirm')
  return t('userSubscriptions.advanceDailyQuotaBlocked')
})

const advanceConfirmDisabled = computed(
  () => advancePreviewLoading.value || advancingQuota.value || !advancePreview.value?.can_advance
)

function subscriptionHasPeakRate(subscription: UserSubscription): boolean {
  return hasPeakRate(subscription.group)
}

function subscriptionPeakRateLabel(subscription: UserSubscription): string {
  return formatPeakRateWindow(subscription.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

async function loadSubscriptions(showLoading = true) {
  try {
    if (showLoading) loading.value = true
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    if (showLoading) loading.value = false
  }
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatExpirationDate(expiresAt: string): string {
  const currentNow = now.value
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - currentNow.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  const relation = getExpirationDateRelation(expires, currentNow)

  if (relation === null) return ''

  if (relation === 'expired') {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTimeToMinute(expires)

  if (relation === 'today') {
    return `${dateStr} (${t('common.today')})`
  }
  if (relation === 'tomorrow') {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const currentNow = now.value
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - currentNow.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

function formatDailyUsageWindow(subscription: UserSubscription): string {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at, now.value)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(quotaResetAt(subscription, 'daily'))
  })
}

function quotaResetAt(subscription: UserSubscription, period: 'daily' | 'weekly' | 'monthly'): string | null {
  const authoritative = subscription[`${period}_resets_at`]
  if (authoritative) return authoritative

  const windowStart = subscription[`${period}_window_start`]
  if (!windowStart) return null
  const hours = period === 'daily' ? 24 : period === 'weekly' ? 168 : 720
  const resetTime = new Date(windowStart).getTime() + hours * 60 * 60 * 1000
  const expiresAt = subscription.expires_at ? new Date(subscription.expires_at).getTime() : Number.NaN
  if (!Number.isFinite(resetTime) || (Number.isFinite(expiresAt) && resetTime >= expiresAt)) return null
  return new Date(resetTime).toISOString()
}

function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return t('userSubscriptions.noResetBeforeExpiry')
  const parts = getRemainingDurationParts(resetAt, now.value)
  return parts ? formatDurationParts(parts) : t('userSubscriptions.availableNow')
}

function formatPreviewReset(resetAt: string | null): string {
  if (!resetAt) return t('userSubscriptions.noResetBeforeExpiry')
  const parts = getRemainingDurationParts(resetAt, now.value)
  if (!parts) return t('userSubscriptions.availableNow')
  return t('userSubscriptions.resetAtDetail', {
    time: formatDurationParts(parts),
    date: formatDateTimeToMinute(resetAt)
  })
}

function formatUSD(value: number): string {
  return `$${value.toFixed(2)}`
}

function formatQuotaRemaining(remaining: number | null, limit: number | null): string {
  if (remaining === null || limit === null) return t('userSubscriptions.unlimited')
  return `${formatUSD(remaining)} / ${formatUSD(limit)}`
}

function advanceBlockerLabel(blocker: string): string {
  const knownBlockers = new Set([
    'subscription_inactive',
    'one_time_subscription',
    'insufficient_term',
    'daily_quota_not_configured',
    'daily_quota_already_reset',
    'daily_quota_not_exhausted',
    'weekly_quota_insufficient',
    'monthly_quota_insufficient'
  ])
  return knownBlockers.has(blocker)
    ? t(`userSubscriptions.advanceBlockers.${blocker}`)
    : t('userSubscriptions.advanceBlockers.unknown')
}

async function openAdvanceDailyQuotaDialog(subscription: UserSubscription) {
  selectedAdvanceSubscription.value = subscription
  advancePreview.value = null
  advancePreviewLoading.value = true
  try {
    const preview = await subscriptionsAPI.getDailyQuotaAdvancePreview(subscription.id)
    if (selectedAdvanceSubscription.value?.id === subscription.id) {
      advancePreview.value = preview
    }
  } catch (error: any) {
    if (selectedAdvanceSubscription.value?.id === subscription.id) {
      selectedAdvanceSubscription.value = null
    }
    appStore.showError(error?.message || t('userSubscriptions.advancePreviewFailed'))
  } finally {
    advancePreviewLoading.value = false
  }
}

function closeAdvanceDailyQuotaDialog() {
  if (advancingQuota.value) return
  selectedAdvanceSubscription.value = null
  advancePreview.value = null
}

async function confirmAdvanceDailyQuota() {
  const selected = selectedAdvanceSubscription.value
  if (!selected || advancingQuota.value || !advancePreview.value?.can_advance) return

  advancingQuota.value = true
  try {
    const updated = await subscriptionsAPI.advanceDailyQuota(selected.id)
    const index = subscriptions.value.findIndex(subscription => subscription.id === updated.id)
    if (index >= 0) subscriptions.value[index] = updated
    subscriptionStore.invalidateCache()
    selectedAdvanceSubscription.value = null
    appStore.showSuccess(t('userSubscriptions.advanceDailyQuotaSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('userSubscriptions.advanceDailyQuotaFailed'))
    await Promise.allSettled([
      openAdvanceDailyQuotaDialog(selected),
      loadSubscriptions(false)
    ])
  } finally {
    advancingQuota.value = false
  }
}

onMounted(() => {
  clockTimer = setInterval(() => {
    const nextNow = new Date()
    const nextTime = nextNow.getTime()
    const crossedBoundary = subscriptions.value.some(subscription => {
      return (['daily', 'weekly', 'monthly'] as const).some(period => {
        const resetAt = quotaResetAt(subscription, period)
        if (!resetAt) return false
        const resetTime = new Date(resetAt).getTime()
        return resetTime > previousClockTime && resetTime <= nextTime
      })
    })
    now.value = nextNow
    previousClockTime = nextTime
    if (crossedBoundary && !refreshingWindowBoundary) {
      refreshingWindowBoundary = true
      void loadSubscriptions(false).finally(() => {
        refreshingWindowBoundary = false
      })
    }
  }, 1000)
  loadSubscriptions()
})

onBeforeUnmount(() => {
  if (clockTimer) clearInterval(clockTimer)
})
</script>
