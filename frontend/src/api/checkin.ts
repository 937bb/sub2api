/**
 * Checkin API endpoints
 * Handles daily check-in operations
 */

import { apiClient } from './client'

export interface CheckinRecord {
  id: number
  checkin_date: string
  streak: number
  base_amount: number
  milestone_amount: number
  total_amount: number
  balance_type: string
  created_at: string
}

export interface CheckinMilestone {
  days: number
  amount: number
  balance_type: string
  expiry_days: number
}

export interface CheckinConfig {
  mode: string
  fixed_amount: number
  random_min: number
  random_max: number
  balance_type: string
  expiry_days: number
  milestones: CheckinMilestone[]
}

export interface CheckinStatus {
  enabled: boolean
  checked_in_today: boolean
  current_streak: number
  total_checkins: number
  last_checkin_date: string
  config?: CheckinConfig
}

export interface CheckinCalendar {
  year: number
  month: number
  records: CheckinRecord[]
}

/**
 * Perform daily check-in
 */
export async function checkin(): Promise<CheckinRecord> {
  const { data } = await apiClient.post<CheckinRecord>('/user/checkin')
  return data
}

/**
 * Get check-in status for current user
 */
export async function getStatus(): Promise<CheckinStatus> {
  const { data } = await apiClient.get<CheckinStatus>('/user/checkin/status')
  return data
}

/**
 * Get monthly check-in calendar
 */
export async function getCalendar(year: number, month: number): Promise<CheckinCalendar> {
  const { data } = await apiClient.get<CheckinCalendar>('/user/checkin/calendar', {
    params: { year, month }
  })
  return data
}

export const checkinAPI = {
  checkin,
  getStatus,
  getCalendar,
}
