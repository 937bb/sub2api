import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DistillationMonitorView from '../DistillationMonitorView.vue'
import type { ContentModerationConfig, UpdateContentModerationConfig } from '@/api/admin/riskControl'

const { getConfig, updateConfig, getStatus, listLogs, getGroups, showError, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getStatus: vi.fn(),
  listLogs: vi.fn(),
  getGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    riskControl: { getConfig, updateConfig, getStatus, listLogs },
    groups: { getAll: getGroups },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const config = (): ContentModerationConfig => ({
  enabled: false,
  mode: 'off',
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null,
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [],
  api_key_statuses: [],
  timeout_ms: 3000,
  sample_rate: 100,
  all_groups: true,
  group_ids: [],
  record_non_hits: false,
  thresholds: {},
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: '',
  email_on_hit: false,
  auto_ban_enabled: false,
  ban_threshold: 10,
  violation_window_hours: 720,
  retry_count: 2,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  blocked_keywords: [],
  keyword_blocking_mode: 'keyword_and_api',
  model_filter: { type: 'all', models: [] },
  cyber_policy_exclude_from_ban_count: false,
  distillation_monitor: {
    enabled: false,
    mode: 'observe',
    group_ids: [],
    window_minutes: 15,
    min_requests: 100,
    min_output_tokens: 300000,
    observe_threshold: 60,
    risk_threshold: 85,
    consecutive_windows: 3,
    min_evidence_classes: 3,
    feature_retention_days: 7,
    assessment_retention_days: 180,
  },
})

const status = () => ({
  enabled: false,
  risk_control_enabled: false,
  mode: 'off',
  worker_count: 4,
  max_workers: 32,
  active_workers: 0,
  idle_workers: 4,
  queue_size: 32768,
  queue_length: 0,
  queue_usage_percent: 0,
  enqueued: 0,
  dropped: 0,
  processed: 0,
  errors: 0,
  pre_block_active: 0,
  pre_block_checked: 0,
  pre_block_allowed: 0,
  pre_block_blocked: 0,
  pre_block_errors: 0,
  pre_block_avg_latency_ms: 0,
  pre_block_api_key_active: 0,
  pre_block_api_key_available_count: 0,
  pre_block_api_key_total_calls: 0,
  pre_block_api_key_loads: [],
  api_key_statuses: [],
  flagged_hash_count: 0,
  last_cleanup_deleted_hit: 0,
  last_cleanup_deleted_non_hit: 0,
  distillation: {
    enabled: false,
    mode: 'observe' as const,
    queue_length: 0,
    queue_size: 8192,
    enqueued: 0,
    dropped: 0,
    processed: 0,
    errors: 0,
    last_flagged: 0,
    last_disabled: 0,
  },
})

describe('admin DistillationMonitorView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue(config())
    getStatus.mockResolvedValue(status())
    getGroups.mockResolvedValue([{ id: 27, name: 'Protected Pool', platform: 'openai' }])
    listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    updateConfig.mockImplementation(async (payload: UpdateContentModerationConfig) => ({ ...config(), ...payload }))
  })

  it('loads independent records and only saves distillation_monitor', async () => {
    const wrapper = mount(DistillationMonitorView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          Pagination: true,
          Select: true,
          Toggle: true,
        },
      },
    })

    await flushPromises()
    expect(listLogs).toHaveBeenCalledWith(expect.objectContaining({ provider: 'distillation_monitor' }))
    const buttons = wrapper.findAll('button')
    const groupButton = buttons.find(button => button.text().includes('Protected Pool'))
    expect(groupButton).toBeDefined()
    await groupButton!.trigger('click')
    const saveButton = buttons.find(button => button.text().includes('admin.distillationMonitor.save'))
    expect(saveButton).toBeDefined()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith({
      distillation_monitor: expect.objectContaining({ group_ids: [27], mode: 'observe' }),
    })
    expect(Object.keys(updateConfig.mock.calls[0][0])).toEqual(['distillation_monitor'])
    expect(showError).not.toHaveBeenCalled()
  })
})
