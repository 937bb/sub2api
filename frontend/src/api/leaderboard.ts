import apiClient from './client'

export interface LeaderboardEntry {
  rank: number
  email: string
  actual_cost: number
  requests: number
  tokens: number
}

export interface LeaderboardRewardRule {
  rank: number
  mode: 'fixed' | 'percent'
  amount: number
  balance_type: string
  expiry_days: number
}

export interface LeaderboardResponse {
  enabled: boolean
  today: LeaderboardEntry[]
  yesterday: LeaderboardEntry[]
  total: LeaderboardEntry[]
  reward_rules: LeaderboardRewardRule[]
}

export const leaderboardAPI = {
  async getLeaderboard(): Promise<LeaderboardResponse> {
    const { data } = await apiClient.get<LeaderboardResponse>('/user/leaderboard')
    return data
  },
}
