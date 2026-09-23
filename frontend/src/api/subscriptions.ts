/**
 * User Subscription API
 * API for regular users to view their own subscriptions and progress
 */

import { apiClient } from './client'
import type { UserSubscription, SubscriptionProgress } from '@/types'

const advanceDailyQuotaKeys = new Map<string, string>()

export interface DailyQuotaAdvancePreview {
  subscription_id: number
  can_advance: boolean
  daily_limit_usd: number
  daily_usage_usd: number
  daily_remaining_usd: number
  daily_resets_at: string | null
  weekly_limit_usd: number | null
  weekly_usage_usd: number
  weekly_remaining_usd: number | null
  weekly_resets_at: string | null
  monthly_limit_usd: number | null
  monthly_usage_usd: number
  monthly_remaining_usd: number | null
  monthly_resets_at: string | null
  recoverable_usd: number
  deduct_hours: number
  current_expires_at: string
  expires_at_after: string
  blockers: string[]
}

function currentUserID(): string | null {
  try {
    const raw = globalThis.localStorage?.getItem('auth_user')
    if (!raw) return null
    const user = JSON.parse(raw) as { id?: unknown }
    return typeof user.id === 'number' && Number.isSafeInteger(user.id) && user.id > 0
      ? String(user.id)
      : null
  } catch {
    return null
  }
}

function advanceDailyQuotaScope(subscriptionId: number): string | null {
  const userID = currentUserID()
  return userID ? `sub2api:user:advance-daily-quota:${userID}:${subscriptionId}` : null
}

function storedOperationKey(scope: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(scope) ?? null
  } catch {
    return null
  }
}

function storeOperationKey(scope: string, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(scope, key)
    else globalThis.sessionStorage?.removeItem(scope)
  } catch {
    // The in-memory key still protects retries when browser storage is unavailable.
  }
}

/**
 * Subscription summary for user dashboard
 */
export interface SubscriptionSummary {
  active_count: number
  subscriptions: Array<{
    id: number
    group_name: string
    status: string
    daily_progress: number | null
    weekly_progress: number | null
    monthly_progress: number | null
    expires_at: string | null
    days_remaining: number | null
  }>
}

/**
 * Get list of current user's subscriptions
 */
export async function getMySubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions')
  return response.data
}

/**
 * Get current user's active subscriptions
 */
export async function getActiveSubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions/active')
  return response.data
}

/**
 * Get progress for all user's active subscriptions
 */
export async function getSubscriptionsProgress(): Promise<SubscriptionProgress[]> {
  const response = await apiClient.get<SubscriptionProgress[]>('/subscriptions/progress')
  return response.data
}

/**
 * Get subscription summary for dashboard display
 */
export async function getSubscriptionSummary(): Promise<SubscriptionSummary> {
  const response = await apiClient.get<SubscriptionSummary>('/subscriptions/summary')
  return response.data
}

/**
 * Get progress for a specific subscription
 */
export async function getSubscriptionProgress(
  subscriptionId: number
): Promise<SubscriptionProgress> {
  const response = await apiClient.get<SubscriptionProgress>(
    `/subscriptions/${subscriptionId}/progress`
  )
  return response.data
}

/** Load authoritative quota and expiry values before showing the confirmation. */
export async function getDailyQuotaAdvancePreview(
  subscriptionId: number
): Promise<DailyQuotaAdvancePreview> {
  const response = await apiClient.get<DailyQuotaAdvancePreview>(
    `/subscriptions/${subscriptionId}/advance-daily-quota-preview`
  )
  return response.data
}

function shouldRetainAdvanceDailyQuotaKey(error: unknown): boolean {
  const { status, reason } = error as { status?: unknown; reason?: unknown }
  if (typeof status !== 'number' || status <= 0) return true
  if (status >= 500 || status === 408 || status === 425 || status === 429) return true
  return reason === 'IDEMPOTENCY_IN_PROGRESS' || reason === 'IDEMPOTENCY_RETRY_BACKOFF'
}

/** Consume 24 hours of validity to make the next daily quota available now. */
export async function advanceDailyQuota(subscriptionId: number): Promise<UserSubscription> {
  const scope = advanceDailyQuotaScope(subscriptionId)
  let idempotencyKey = scope
    ? advanceDailyQuotaKeys.get(scope) ?? storedOperationKey(scope)
    : null
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `subscription-daily-quota-${currentUserID() ?? 'unknown-user'}-${subscriptionId}-${requestID}`
  }
  if (scope) {
    advanceDailyQuotaKeys.set(scope, idempotencyKey)
    storeOperationKey(scope, idempotencyKey)
  }

  try {
    const response = await apiClient.post<UserSubscription>(
      `/subscriptions/${subscriptionId}/advance-daily-quota`,
      undefined,
      { headers: { 'Idempotency-Key': idempotencyKey } }
    )
    if (scope) {
      advanceDailyQuotaKeys.delete(scope)
      storeOperationKey(scope, null)
    }
    return response.data
  } catch (error) {
    // A server response proves whether the request ran. Only retain the key for
    // ambiguous transport failures where retrying must replay the first result.
    if (scope && !shouldRetainAdvanceDailyQuotaKey(error)) {
      advanceDailyQuotaKeys.delete(scope)
      storeOperationKey(scope, null)
    }
    throw error
  }
}

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress,
  getDailyQuotaAdvancePreview,
  advanceDailyQuota
}
