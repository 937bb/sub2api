const apiMocks = vi.hoisted(() => ({
  getById: vi.fn(),
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    getById: apiMocks.getById,
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UsageDetailDialog from '../UsageDetailDialog.vue'

const detail = {
  id: 42,
  user_id: 1,
  api_key_id: 2,
  account_id: 3,
  request_id: 'req-detail-42',
  model: 'gpt-5.6-sol',
  group_id: null,
  subscription_id: null,
  input_tokens: 10,
  output_tokens: 5,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: 0,
  output_cost: 0,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  total_cost: 0,
  actual_cost: 0,
  rate_multiplier: 1,
  long_context_billing_applied: false,
  billing_type: 0,
  stream: false,
  duration_ms: 120,
  first_token_ms: 80,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  user_agent: null,
  cache_ttl_overridden: false,
  created_at: '2026-07-28T03:00:00Z',
  quota_bypass_applied: true,
  quota_bypass_inject_pairs: 3,
}

describe('UsageDetailDialog lazy loading', () => {
  beforeEach(() => {
    apiMocks.getById.mockReset()
    apiMocks.getById.mockResolvedValue(detail)
  })

  it('does not load while closed and loads only after opening', async () => {
    const wrapper = mount(UsageDetailDialog, {
      props: { show: false, usageId: 42 },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
          DetailItem: true,
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(apiMocks.getById).not.toHaveBeenCalled()

    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(apiMocks.getById).toHaveBeenCalledTimes(1)
    expect(apiMocks.getById).toHaveBeenCalledWith(42)
  })

  it('does not prefetch a changed row while closed', async () => {
    const wrapper = mount(UsageDetailDialog, {
      props: { show: false, usageId: 42 },
      global: { stubs: { BaseDialog: true, DetailItem: true, Icon: true } },
    })

    await wrapper.setProps({ usageId: 43 })
    await flushPromises()
    expect(apiMocks.getById).not.toHaveBeenCalled()
  })
})
