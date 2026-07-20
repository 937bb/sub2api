import { apiClient } from './client'

export type GrowthPeriod = 'daily' | 'weekly' | 'monthly'
export type GrowthRewardMode = 'fixed' | 'random'

export interface GrowthStreakReward {
  days: number
  amount: number
}

export interface GrowthLeaderboardRewardRule {
  id: string
  enabled: boolean
  period: GrowthPeriod
  rank_start: number
  rank_end: number
  reward_amount: number
}

export interface GrowthConfig {
  checkin_enabled: boolean
  checkin_reward_mode: GrowthRewardMode
  checkin_fixed_reward: number
  checkin_min_reward: number
  checkin_max_reward: number
  checkin_streak_rewards: GrowthStreakReward[]
  checkin_min_account_age_days: number
  checkin_min_total_recharged: number
  checkin_max_reward_paid_ratio: number
  max_total_reward_paid_ratio: number
  checkin_min_recent_spend: number
  checkin_recent_spend_days: number
  checkin_max_accounts_per_ip: number
  checkin_max_accounts_per_device: number
  leaderboard_enabled: boolean
  leaderboard_anonymous: boolean
  leaderboard_reward_rules: GrowthLeaderboardRewardRule[]
  updated_at?: string
}

export interface GrowthPublicConfig {
  checkin_enabled: boolean
  checkin_reward_mode: GrowthRewardMode
  checkin_fixed_reward: number
  checkin_min_reward: number
  checkin_max_reward: number
  checkin_streak_rewards: GrowthStreakReward[]
  leaderboard_enabled: boolean
}

export interface GrowthCheckin {
  date: string
  streak_days: number
  base_reward: number
  streak_reward: number
  total_reward: number
  balance_after: number
}

export interface GrowthCheckinStatus {
  config: GrowthPublicConfig
  today: string
  checked_in_today: boolean
  current_streak: number
  total_checkins: number
  month_checkins: GrowthCheckin[]
  eligibility_hint?: string
}

export interface GrowthLeaderboardItem {
  rank: number
  user_id?: number
  display_name: string
  actual_cost: number
  requests: number
  is_current_user: boolean
}

export interface GrowthLeaderboard {
  period: GrowthPeriod
  period_start: string
  period_end: string
  items: GrowthLeaderboardItem[]
  current_user?: GrowthLeaderboardItem
  total: number
  page: number
  page_size: number
  pages: number
}

export interface GrowthRewardLedgerItem {
  id: number
  user_id: number
  email: string
  source_type: string
  source_key: string
  amount: number
  balance_after: number
  metadata: Record<string, unknown>
  created_at: string
}

export interface GrowthRiskEvent {
  id: number
  user_id?: number
  email: string
  decision: string
  reason_code: string
  evidence: Record<string, unknown>
  created_at: string
}

export interface PaginatedGrowthResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export function getCheckinStatus(month: string): Promise<GrowthCheckinStatus> {
  return apiClient.get<GrowthCheckinStatus>('/growth/checkin', { params: { month } }).then(({ data }) => data)
}

export function claimCheckin(payload: { device_id: string }): Promise<GrowthCheckin> {
  return apiClient.post<GrowthCheckin>('/growth/checkin', payload).then(({ data }) => data)
}

export function getLeaderboard(period: GrowthPeriod, page = 1, pageSize = 50): Promise<GrowthLeaderboard> {
  return apiClient.get<GrowthLeaderboard>('/growth/leaderboard', { params: { period, page, page_size: pageSize } }).then(({ data }) => data)
}

export function getGrowthConfig(): Promise<GrowthConfig> {
  return apiClient.get<GrowthConfig>('/admin/growth/config').then(({ data }) => data)
}

export function updateGrowthConfig(config: GrowthConfig): Promise<GrowthConfig> {
  return apiClient.put<GrowthConfig>('/admin/growth/config', config).then(({ data }) => data)
}

export function listGrowthRewards(page = 1, pageSize = 20): Promise<PaginatedGrowthResponse<GrowthRewardLedgerItem>> {
  return apiClient.get<PaginatedGrowthResponse<GrowthRewardLedgerItem>>('/admin/growth/rewards', { params: { page, page_size: pageSize } }).then(({ data }) => data)
}

export function listGrowthRiskEvents(page = 1, pageSize = 20): Promise<PaginatedGrowthResponse<GrowthRiskEvent>> {
  return apiClient.get<PaginatedGrowthResponse<GrowthRiskEvent>>('/admin/growth/risk-events', { params: { page, page_size: pageSize } }).then(({ data }) => data)
}

export function settleGrowthLeaderboard(period: GrowthPeriod): Promise<{ period: GrowthPeriod; rewarded_users: number; total_reward: number }> {
  return apiClient.post(`/admin/growth/leaderboard/settle/${period}`).then(({ data }) => data)
}
