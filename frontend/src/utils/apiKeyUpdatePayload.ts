import type { ApiKey, UpdateApiKeyRequest } from '@/types'

export const isTerminalApiKeyStatus = (status: ApiKey['status']) =>
  status === 'quota_exhausted' || status === 'expired'

type ApiKeyStatusSource = Pick<ApiKey, 'status' | 'quota_used'>

const payloadFixesTerminalStatus = (payload: UpdateApiKeyRequest, key: ApiKeyStatusSource) => {
  if (key.status === 'quota_exhausted') {
    return payload.reset_quota === true || (typeof payload.quota === 'number' && (payload.quota <= 0 || payload.quota > key.quota_used))
  }
  if (key.status === 'expired') {
    if (payload.expires_at === '') return true
    if (typeof payload.expires_at !== 'string') return false
    const expiresAt = new Date(payload.expires_at).getTime()
    return Number.isFinite(expiresAt) && expiresAt > Date.now()
  }
  return true
}

export const assignApiKeyEditStatus = (
  payload: UpdateApiKeyRequest,
  currentKey: ApiKeyStatusSource,
  formStatus: UpdateApiKeyRequest['status']
) => {
  if (isTerminalApiKeyStatus(currentKey.status)) {
    // The edit modal maps terminal statuses to inactive because the form only has
    // active/inactive choices. While the terminal condition still applies, omit
    // synthetic inactive so the backend can keep the terminal status. Once this
    // payload fixes the terminal condition, send the selected active/inactive
    // status so the backend knows whether to reactivate or preserve inactive.
    if ((formStatus === 'active' || formStatus === 'inactive') && payloadFixesTerminalStatus(payload, currentKey)) {
      payload.status = formStatus
    }
    return payload
  }

  if (formStatus) {
    payload.status = formStatus
  }
  return payload
}
