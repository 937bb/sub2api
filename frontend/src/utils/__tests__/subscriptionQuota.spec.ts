import { describe, expect, it } from 'vitest'

import type { UserSubscription } from '@/types'
import {
  advancedDailyQuotaExpiry,
  canAdvanceDailyQuota,
  canPreviewDailyQuotaAdvance,
  getExpirationDateRelation,
  getRemainingExpiryDuration,
  isOneTimeDailyQuota
} from '../subscriptionQuota'

describe('subscription expiry timing', () => {
  it('uses local calendar dates for today and tomorrow', () => {
    const now = new Date(2026, 2, 7, 23, 30)

    expect(getExpirationDateRelation(new Date(2026, 2, 7, 23, 45), now)).toBe('today')
    expect(getExpirationDateRelation(new Date(2026, 2, 8, 3, 30), now)).toBe('tomorrow')
  })

  it('treats the exact expiry instant and elapsed expiries as expired', () => {
    const now = new Date(2026, 6, 30, 9, 0)

    expect(getExpirationDateRelation(now, now)).toBe('expired')
    expect(getRemainingExpiryDuration(now, now)).toBeNull()
    expect(getExpirationDateRelation(new Date(2026, 6, 30, 8, 59), now)).toBe('expired')
    expect(getRemainingExpiryDuration(new Date(2026, 6, 30, 8, 59), now)).toBeNull()
  })

  it('rejects invalid target and current dates', () => {
    const invalid = new Date('invalid')
    const valid = new Date(2026, 6, 30, 9, 0)

    expect(getExpirationDateRelation(invalid, valid)).toBeNull()
    expect(getExpirationDateRelation(valid, invalid)).toBeNull()
    expect(getRemainingExpiryDuration(invalid, valid)).toBeNull()
    expect(getRemainingExpiryDuration(valid, invalid)).toBeNull()
  })

  it('returns rounded-up hours and minutes for an expiry under 24 hours away', () => {
    const now = new Date(2026, 6, 30, 9, 0)

    expect(getRemainingExpiryDuration(new Date(2026, 6, 31, 8, 30), now)).toEqual({
      unit: 'hoursMinutes',
      hours: 23,
      minutes: 30
    })
    expect(getRemainingExpiryDuration(new Date(now.getTime() + 1), now)).toEqual({
      unit: 'hoursMinutes',
      hours: 0,
      minutes: 1
    })
    expect(getRemainingExpiryDuration(new Date(now.getTime() + 23 * 60 * 60 * 1000 + 1), now)).toEqual({
      unit: 'hoursMinutes',
      hours: 23,
      minutes: 1
    })
  })

  it('preserves rounded-up day display from 24 hours onward', () => {
    const now = new Date(2026, 6, 30, 9, 0)

    expect(getRemainingExpiryDuration(new Date(now.getTime() + 24 * 60 * 60 * 1000), now)).toEqual({
      unit: 'days',
      days: 1
    })
    expect(getRemainingExpiryDuration(new Date(now.getTime() + 24 * 60 * 60 * 1000 + 1), now)).toEqual({
      unit: 'days',
      days: 2
    })
  })
})

describe('advance daily quota eligibility', () => {
  const now = new Date('2026-09-23T10:00:00.000Z')

  function subscription(overrides: Partial<UserSubscription> = {}): UserSubscription {
    return {
      id: 1,
      user_id: 7,
      group_id: 9,
      status: 'active',
      starts_at: '2026-09-01T10:00:00.000Z',
      expires_at: '2026-10-01T10:00:00.000Z',
      daily_usage_usd: 10,
      weekly_usage_usd: 20,
      monthly_usage_usd: 30,
      daily_window_start: '2026-09-23T00:00:00.000Z',
      weekly_window_start: '2026-09-20T00:00:00.000Z',
      monthly_window_start: '2026-09-01T00:00:00.000Z',
      created_at: '2026-09-01T10:00:00.000Z',
      updated_at: '2026-09-23T09:00:00.000Z',
      group: {
        daily_limit_usd: 10,
        weekly_limit_usd: 100,
        monthly_limit_usd: 300
      } as UserSubscription['group'],
      ...overrides
    }
  }

  it('allows an exhausted multi-day subscription with more than a day remaining', () => {
    expect(canAdvanceDailyQuota(subscription(), now)).toBe(true)
    expect(advancedDailyQuotaExpiry(subscription())?.toISOString()).toBe('2026-09-30T10:00:00.000Z')
  })

  it('rejects one-day cards even when they cross midnight', () => {
    const dailyCard = subscription({
      starts_at: '2026-09-23T08:00:00.000Z',
      expires_at: '2026-09-24T08:00:00.000Z'
    })

    expect(isOneTimeDailyQuota(dailyCard)).toBe(true)
    expect(canAdvanceDailyQuota(dailyCard, now)).toBe(false)
  })

  it('rejects unspent quota, short remaining terms, and exhausted period limits', () => {
    expect(canAdvanceDailyQuota(subscription({ daily_usage_usd: 9.99 }), now)).toBe(false)
    expect(canAdvanceDailyQuota(subscription({ expires_at: '2026-09-24T10:00:00.000Z' }), now)).toBe(false)
    expect(canAdvanceDailyQuota(subscription({ weekly_usage_usd: 100 }), now)).toBe(false)
    expect(canAdvanceDailyQuota(subscription({ monthly_usage_usd: 300 }), now)).toBe(false)
    expect(canAdvanceDailyQuota(subscription({ weekly_usage_usd: 95 }), now)).toBe(false)
    expect(canAdvanceDailyQuota(subscription({ monthly_usage_usd: 295 }), now)).toBe(false)
  })

  it('keeps the preview available when weekly or monthly quota blocks confirmation', () => {
    const weeklyBlocked = subscription({ weekly_usage_usd: 95 })
    const monthlyBlocked = subscription({ monthly_usage_usd: 295 })

    expect(canPreviewDailyQuotaAdvance(weeklyBlocked, now)).toBe(true)
    expect(canAdvanceDailyQuota(weeklyBlocked, now)).toBe(false)
    expect(canPreviewDailyQuotaAdvance(monthlyBlocked, now)).toBe(true)
    expect(canAdvanceDailyQuota(monthlyBlocked, now)).toBe(false)
  })

  it('hides the preview until the daily quota is exhausted and at least 24 hours remain', () => {
    expect(canPreviewDailyQuotaAdvance(subscription({ daily_usage_usd: 9.99 }), now)).toBe(false)
    expect(
      canPreviewDailyQuotaAdvance(
        subscription({ expires_at: '2026-09-24T10:00:00.000Z' }),
        now
      )
    ).toBe(false)
  })

  it('rejects an exhausted view after its authoritative daily reset time', () => {
    expect(
      canAdvanceDailyQuota(
        subscription({ daily_resets_at: '2026-09-23T09:59:59.000Z' }),
        now
      )
    ).toBe(false)
  })
})
