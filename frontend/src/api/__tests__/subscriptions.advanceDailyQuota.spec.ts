import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import { advanceDailyQuota, getDailyQuotaAdvancePreview } from '@/api/subscriptions'

describe('advanceDailyQuota API', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 7 }))
    post.mockReset()
    get.mockReset()
    post.mockResolvedValue({ data: { id: 42, daily_usage_usd: 0 } })
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('11111111-1111-4111-8111-111111111111')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('sends and clears a user-scoped idempotency key after success', async () => {
    await expect(advanceDailyQuota(42)).resolves.toEqual({ id: 42, daily_usage_usd: 0 })

    expect(post).toHaveBeenCalledWith('/subscriptions/42/advance-daily-quota', undefined, {
      headers: {
        'Idempotency-Key': 'subscription-daily-quota-7-42-11111111-1111-4111-8111-111111111111'
      }
    })
    expect(sessionStorage.length).toBe(0)
  })

  it('reuses the key after an ambiguous transport failure', async () => {
    post.mockRejectedValueOnce({ status: 0, message: 'Network error. Please check your connection.' })
    await expect(advanceDailyQuota(43)).rejects.toMatchObject({ status: 0 })

    post.mockResolvedValueOnce({ data: { id: 43, daily_usage_usd: 0 } })
    await advanceDailyQuota(43)

    expect(post.mock.calls[1][2].headers).toEqual(post.mock.calls[0][2].headers)
    expect(sessionStorage.length).toBe(0)
  })

  it('discards the key after a definitive server rejection', async () => {
    post.mockRejectedValueOnce({ status: 409, reason: 'DAILY_QUOTA_NOT_EXHAUSTED' })
    await expect(advanceDailyQuota(44)).rejects.toMatchObject({ status: 409 })

    vi.mocked(globalThis.crypto.randomUUID).mockReturnValueOnce('22222222-2222-4222-8222-222222222222')
    post.mockResolvedValueOnce({ data: { id: 44, daily_usage_usd: 0 } })
    await advanceDailyQuota(44)

    expect(post.mock.calls[1][2].headers).not.toEqual(post.mock.calls[0][2].headers)
    expect(sessionStorage.length).toBe(0)
  })

  it('retains the key after an ambiguous server or idempotency response', async () => {
    post.mockRejectedValueOnce({ status: 503, reason: 'SERVICE_UNAVAILABLE' })
    await expect(advanceDailyQuota(45)).rejects.toMatchObject({ status: 503 })

    post.mockResolvedValueOnce({ data: { id: 45, daily_usage_usd: 0 } })
    await advanceDailyQuota(45)
    expect(post.mock.calls[1][2].headers).toEqual(post.mock.calls[0][2].headers)

    post.mockRejectedValueOnce({ status: 409, reason: 'IDEMPOTENCY_IN_PROGRESS' })
    await expect(advanceDailyQuota(46)).rejects.toMatchObject({ reason: 'IDEMPOTENCY_IN_PROGRESS' })
    post.mockResolvedValueOnce({ data: { id: 46, daily_usage_usd: 0 } })
    await advanceDailyQuota(46)
    expect(post.mock.calls[3][2].headers).toEqual(post.mock.calls[2][2].headers)
  })

  it('loads the authoritative confirmation preview', async () => {
    const preview = { subscription_id: 42, can_advance: true, deduct_hours: 24 }
    get.mockResolvedValueOnce({ data: preview })

    await expect(getDailyQuotaAdvancePreview(42)).resolves.toEqual(preview)
    expect(get).toHaveBeenCalledWith('/subscriptions/42/advance-daily-quota-preview')
  })
})
