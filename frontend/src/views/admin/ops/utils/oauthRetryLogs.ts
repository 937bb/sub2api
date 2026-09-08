import type { OpsSystemLogQuery } from '@/api/admin/ops'

export interface OAuthRetryLogFilters {
  timeRange: '5m' | '30m' | '1h' | '6h' | '24h' | 'custom'
  customStartTime?: string | null
  customEndTime?: string | null
  platformFilter?: string
  refreshToken: number
}

export function retryLogQuery(filters: OAuthRetryLogFilters): OpsSystemLogQuery {
  return {
    component: 'oauth_retry',
    platform: filters.platformFilter || undefined,
    time_range: filters.timeRange === 'custom' ? undefined : filters.timeRange,
    start_time: filters.timeRange === 'custom' ? filters.customStartTime || undefined : undefined,
    end_time: filters.timeRange === 'custom' ? filters.customEndTime || undefined : undefined
  }
}

const eventLabels: Record<string, string> = {
  retry: '已拦截，准备重试',
  exhausted: '重试次数耗尽',
  cancelled: '请求已取消',
  transport_error: '传输错误',
  response_received: '收到响应头',
  skipped: '未重试',
  not_retryable: '未重试'
}

const reasonLabels: Record<string, string> = {
  mapped_status: '命中系统最终错误码，重新转发完整请求',
  mapped_status_exhausted: '系统错误码重试次数耗尽',
  forward_finished: '转发结束，请结合最终请求状态核对',
  response_already_written: '已向客户端输出，不重发',
  billable_result_present: '已有可计费结果，不重发',
  configured_status: '命中配置的错误状态码',
  status_exhausted: '命中错误状态码，重试次数已耗尽',
  context_cancelled: '请求上下文已取消',
  body_not_replayable: '请求体无法重放',
  body_replay_failed: '请求体重放失败',
  status_not_configured: '状态码不在重试配置中',
  transport: '上游传输错误'
}

export function retryReasonLabel(value: unknown): string {
  const reason = retryLogValue(value)
  return reasonLabels[reason] || reason
}

export function retryLogValue(value: unknown): string {
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
    ? String(value)
    : '-'
}

export function retryEventLabel(value: unknown): string {
  const event = retryLogValue(value)
  return eventLabels[event] || event
}

export function retryLogTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}
