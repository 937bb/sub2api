import { describe, expect, it } from 'vitest'
import { assignApiKeyEditStatus } from '../apiKeyUpdatePayload'
import type { ApiKey, UpdateApiKeyRequest } from '@/types'

describe('assignApiKeyEditStatus', () => {
  it.each(['quota_exhausted', 'expired'] satisfies ApiKey['status'][])(
    'omits active status for terminal %s keys when the payload does not fix the terminal condition',
    (currentStatus) => {
      const payload: UpdateApiKeyRequest = { name: 'unchanged' }

      assignApiKeyEditStatus(payload, { status: currentStatus, quota_used: 12 }, 'active')

      expect(payload).toEqual({ name: 'unchanged' })
    }
  )

  it('omits inactive status for terminal keys when the payload does not fix the terminal condition', () => {
    const payload: UpdateApiKeyRequest = { name: 'disable' }

    assignApiKeyEditStatus(payload, { status: 'quota_exhausted', quota_used: 12 }, 'inactive')

    expect(payload).toEqual({ name: 'disable' })
  })

  it('sends inactive when expired key expiration is cleared', () => {
    const payload: UpdateApiKeyRequest = { expires_at: '' }

    assignApiKeyEditStatus(payload, { status: 'expired', quota_used: 0 }, 'inactive')

    expect(payload).toEqual({ expires_at: '', status: 'inactive' })
  })

  it('sends inactive when quota_exhausted key is changed to unlimited', () => {
    const payload: UpdateApiKeyRequest = { quota: 0 }

    assignApiKeyEditStatus(payload, { status: 'quota_exhausted', quota_used: 12 }, 'inactive')

    expect(payload).toEqual({ quota: 0, status: 'inactive' })
  })

  it('omits inactive when expired key expiration is still expired', () => {
    const payload: UpdateApiKeyRequest = { expires_at: '2020-01-01T00:00:00Z' }

    assignApiKeyEditStatus(payload, { status: 'expired', quota_used: 0 }, 'inactive')

    expect(payload).toEqual({ expires_at: '2020-01-01T00:00:00Z' })
  })

  it('keeps active status for terminal keys when explicitly selected and terminal condition is fixed', () => {
    const payload: UpdateApiKeyRequest = { quota: 0 }

    assignApiKeyEditStatus(payload, { status: 'quota_exhausted', quota_used: 12 }, 'active')

    expect(payload).toEqual({ quota: 0, status: 'active' })
  })

  it('keeps status for non-terminal keys', () => {
    const payload: UpdateApiKeyRequest = { quota: 10 }

    assignApiKeyEditStatus(payload, { status: 'active', quota_used: 0 }, 'inactive')

    expect(payload).toEqual({ quota: 10, status: 'inactive' })
  })
})
