import { apiClient } from '../client'
import type { Account } from '@/types'

export interface SIWCAuthorization {
  auth_url: string
  session_id: string
  host_id: string
}

export async function startSIWCAuthorization(proxyId: number | null, hostId?: string, accountId?: number) {
  const { data } = await apiClient.post<SIWCAuthorization>('/admin/openai/siwc/auth-url', {
    proxy_id: proxyId, host_id: hostId, account_id: accountId
  })
  return data
}

export async function createSIWCAccount(input: {
  session_id: string
  callback_url: string
  name: string
  concurrency: number
  group_ids: number[]
  account_id?: number
}) {
  const { data } = await apiClient.post<Account>('/admin/openai/siwc/accounts', input)
  return data
}
