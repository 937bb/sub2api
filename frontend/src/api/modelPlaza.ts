/**
 * Model Plaza API endpoints
 * 模型广场接口：直接从分组+全局定价聚合，不依赖渠道配置。
 */

import { apiClient } from './client'

export interface ModelPlazaGroup {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  is_exclusive: boolean
  subscription_type: string
}

export interface ModelPlazaPricing {
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
}

export interface ModelPlazaModel {
  name: string
  platform: string
  mode: string
  pricing: ModelPlazaPricing | null
  groups: ModelPlazaGroup[]
}

export interface ModelPlazaResponse {
  models: ModelPlazaModel[]
  groups: ModelPlazaGroup[]
}

/** 获取模型广场数据（基于用户分组+全局定价）。 */
export async function getModelPlaza(options?: { signal?: AbortSignal }): Promise<ModelPlazaResponse> {
  const { data } = await apiClient.get<ModelPlazaResponse>('/model-plaza', {
    signal: options?.signal
  })
  return data
}

export const modelPlazaAPI = { getModelPlaza }

export default modelPlazaAPI
