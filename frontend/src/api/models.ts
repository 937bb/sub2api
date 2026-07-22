import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface ModelCatalogPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface ModelCatalogPricing {
  billing_mode: BillingMode | 'video'
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_input_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: ModelCatalogPricingInterval[]
}

export interface ModelCatalogTimeRule {
  id: string
  enabled: boolean
  repeat_type: 'daily' | 'weekly' | string
  start_weekday?: number
  end_weekday?: number
  start: string
  end: string
  rate_multiplier: number
}

export interface ModelCatalogGroup {
  id: number
  name: string
  platform: string
  subscription_type: 'standard' | 'subscription' | string
  is_exclusive: boolean
  default_rate_multiplier: number
  user_rate_multiplier?: number
  resolved_rate_multiplier: number
  time_rate_multiplier: number
  effective_rate_multiplier: number
  billing_rate_multiplier?: number
  image_rate_multiplier: number
  video_rate_multiplier: number
  time_billing_rules?: ModelCatalogTimeRule[]
  pricing: ModelCatalogPricing | null
}

export interface ModelCatalogModel {
  name: string
  platform: string
  billing_mode: BillingMode | 'video'
  base_pricing: ModelCatalogPricing | null
  groups: ModelCatalogGroup[]
  channel_count: number
}

export interface ModelCatalogResponse {
  models: ModelCatalogModel[]
  calculated_at: string
  timezone: string
}

export async function getCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogResponse> {
  const { data } = await apiClient.get<ModelCatalogResponse>('/models/catalog', {
    signal: options?.signal
  })
  return data
}

export const userModelsAPI = { getCatalog }

export default userModelsAPI
