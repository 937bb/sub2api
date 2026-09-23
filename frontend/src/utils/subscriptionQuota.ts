import type { UserSubscription } from '@/types'

const ONE_DAY_MS = 24 * 60 * 60 * 1000

export type ExpirationDateRelation = 'expired' | 'today' | 'tomorrow' | 'later'

export type RemainingExpiryDuration =
  | { unit: 'days'; days: number }
  | { unit: 'hoursMinutes'; hours: number; minutes: number }

export interface RemainingDurationParts {
  days: number
  hours: number
  minutes: number
}

export function isOneTimeDailyQuota(
  subscription: Pick<UserSubscription, 'starts_at' | 'expires_at'>
): boolean {
  if (!subscription.starts_at || !subscription.expires_at) return false

  const startsAt = new Date(subscription.starts_at).getTime()
  const expiresAt = new Date(subscription.expires_at).getTime()

  if (!Number.isFinite(startsAt) || !Number.isFinite(expiresAt)) return false

  return expiresAt <= startsAt + ONE_DAY_MS
}

export function advancedDailyQuotaExpiry(
  subscription: Pick<UserSubscription, 'expires_at'>
): Date | null {
  if (!subscription.expires_at) return null
  const expiresAt = new Date(subscription.expires_at).getTime()
  if (!Number.isFinite(expiresAt)) return null
  return new Date(expiresAt - ONE_DAY_MS)
}

export function canPreviewDailyQuotaAdvance(
  subscription: UserSubscription,
  now: Date = new Date()
): boolean {
  const dailyLimit = subscription.group?.daily_limit_usd
  const nowTime = now.getTime()
  const expiresAt = subscription.expires_at
    ? new Date(subscription.expires_at).getTime()
    : Number.NaN
  const dailyResetAt = subscription.daily_resets_at
    ? new Date(subscription.daily_resets_at).getTime()
    : subscription.daily_window_start
      ? new Date(subscription.daily_window_start).getTime() + ONE_DAY_MS
      : Number.NaN
  if (
    subscription.status !== 'active' ||
    !Number.isFinite(nowTime) ||
    !Number.isFinite(expiresAt) ||
    expiresAt <= nowTime + ONE_DAY_MS ||
    !dailyLimit ||
    dailyLimit <= 0 ||
    isOneTimeDailyQuota(subscription) ||
    (Number.isFinite(dailyResetAt) && dailyResetAt <= nowTime) ||
    subscription.daily_usage_usd < dailyLimit
  ) {
    return false
  }

  return true
}

export function canAdvanceDailyQuota(
  subscription: UserSubscription,
  now: Date = new Date()
): boolean {
  if (!canPreviewDailyQuotaAdvance(subscription, now)) return false

  const dailyLimit = subscription.group!.daily_limit_usd!

  const weeklyLimit = subscription.group?.weekly_limit_usd
  if (weeklyLimit && weeklyLimit > 0 && weeklyLimit - subscription.weekly_usage_usd < dailyLimit) {
    return false
  }
  const monthlyLimit = subscription.group?.monthly_limit_usd
  return !(
    monthlyLimit &&
    monthlyLimit > 0 &&
    monthlyLimit - subscription.monthly_usage_usd < dailyLimit
  )
}

export function getRemainingDurationParts(
  targetAt: Date | string,
  now: Date = new Date()
): RemainingDurationParts | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null

  const diffMs = targetTime - nowTime
  if (diffMs <= 0) return null

  const totalMinutes = Math.max(1, Math.ceil(diffMs / (1000 * 60)))
  const days = Math.floor(totalMinutes / (24 * 60))
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60)
  const minutes = totalMinutes % 60

  return { days, hours, minutes }
}

export function getExpirationDateRelation(
  targetAt: Date | string,
  now: Date = new Date()
): ExpirationDateRelation | null {
  const target = targetAt instanceof Date ? targetAt : new Date(targetAt)
  const targetTime = target.getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null
  if (targetTime <= nowTime) return 'expired'

  const targetDay = Date.UTC(target.getFullYear(), target.getMonth(), target.getDate())
  const currentDay = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
  const calendarDays = Math.round((targetDay - currentDay) / ONE_DAY_MS)

  if (calendarDays === 0) return 'today'
  if (calendarDays === 1) return 'tomorrow'
  return 'later'
}

export function getRemainingExpiryDuration(
  targetAt: Date | string,
  now: Date = new Date()
): RemainingExpiryDuration | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null

  const diffMs = targetTime - nowTime
  if (diffMs <= 0) return null
  if (diffMs >= ONE_DAY_MS) {
    return { unit: 'days', days: Math.ceil(diffMs / ONE_DAY_MS) }
  }

  const totalMinutes = Math.ceil(diffMs / (60 * 1000))
  return {
    unit: 'hoursMinutes',
    hours: Math.floor(totalMinutes / 60),
    minutes: totalMinutes % 60
  }
}
