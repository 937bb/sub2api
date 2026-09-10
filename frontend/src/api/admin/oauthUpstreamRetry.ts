import { apiClient } from '../client'

export interface OAuthUpstreamRetrySettings {
  enabled: boolean
  max_retries: number
  status_codes: number[]
}

export async function getOAuthUpstreamRetrySettings(): Promise<OAuthUpstreamRetrySettings> {
  const { data } = await apiClient.get<OAuthUpstreamRetrySettings>('/admin/settings/oauth-upstream-retry')
  return data
}

export async function updateOAuthUpstreamRetrySettings(settings: OAuthUpstreamRetrySettings): Promise<OAuthUpstreamRetrySettings> {
  const { data } = await apiClient.put<OAuthUpstreamRetrySettings>('/admin/settings/oauth-upstream-retry', settings)
  return data
}
