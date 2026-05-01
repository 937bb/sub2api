/**
 * Balance Entries API endpoints
 * Handles balance entry listing and summary retrieval
 */

import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export interface BalanceEntry {
  id: number
  amount: number
  remaining: number
  balance_type: string
  source: string
  note: string
  expires_at: string | null
  expired: boolean
  created_at: string
}

export interface BalanceSummary {
  total_balance: number
  permanent_balance: number
  expirable_balance: number
  expiring_soon: number
  warning_days: number
}

/**
 * Get paginated balance entries for current user
 */
export async function list(
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<BalanceEntry>> {
  const { data } = await apiClient.get<PaginatedResponse<BalanceEntry>>('/user/balance-entries', {
    params: { page, page_size: pageSize }
  })
  return data
}

/**
 * Get balance summary for current user
 */
export async function summary(): Promise<BalanceSummary> {
  const { data } = await apiClient.get<BalanceSummary>('/user/balance-entries/summary')
  return data
}

export const balanceEntriesAPI = {
  list,
  summary,
}
