import type { Account } from '@/types'

export function isSIWCAccount(account: Account | null | undefined): boolean {
  if (account?.platform !== 'openai' || account.type !== 'oauth') return false
  return String(account.credentials?.auth_mode ?? '').toLowerCase() === 'siwc'
    || String(account.extra?.auth_protocol ?? '').toLowerCase() === 'siwc'
    || String(account.credentials?.client_id ?? '').startsWith('oaiapp_')
}
