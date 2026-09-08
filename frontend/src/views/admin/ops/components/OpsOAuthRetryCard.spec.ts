import { beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Card from './OpsOAuthRetryCard.vue'
import Details from './OpsOAuthRetryDetailsModal.vue'
import { retryLogQuery, retryEventLabel } from '../utils/oauthRetryLogs'

const mocks = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listSystemLogs: mocks.list } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<div v-if="show"><slot /></div>' } }))
vi.mock('@/components/common/Pagination.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/icons/Icon.vue', () => ({ default: { template: '<span />' } }))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue({ items: [], total: 0 })
})

it('queries only retry logs and opens independent details', async () => {
  mocks.list.mockResolvedValue({ items: [], total: 12 })
  const wrapper = mount(Card, { props: { timeRange: '1h', refreshToken: 0 } })
  await flushPromises()
  expect(mocks.list).toHaveBeenCalledWith(expect.objectContaining({ component: 'oauth_retry', time_range: '1h', page_size: 1 }))
  expect(wrapper.text()).toContain('12')
  await wrapper.findAll('button')[1].trigger('click')
  await flushPromises()
  expect(wrapper.find('form').exists()).toBe(true)
})

it('does not show a failed fetch as a zero retry count', async () => {
  mocks.list.mockRejectedValue(new Error('unavailable'))
  const wrapper = mount(Card, { props: { timeRange: '1h', refreshToken: 0 } })
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
})

it('supports request lookup and distinguishes response headers from success', async () => {
  mocks.list.mockResolvedValue({ items: [{ id: 1, created_at: '2026-09-08T10:00:00Z', host: 'node-a', request_id: 'req-1', account_id: 42, extra: { event: 'response_received', retry_id: 'chain-1', status: 200, retry: 1, max_retries: 5 } }], total: 1 })
  const wrapper = mount(Details, { props: { show: false, timeRange: '1h', refreshToken: 0 } })
  await wrapper.setProps({ show: true })
  await flushPromises()
  expect(wrapper.text()).toContain('收到响应头')
  expect(wrapper.text()).toContain('chain-1')
  await wrapper.get('input').setValue('req-1')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ request_id: 'req-1', page: 1 }))
})

it('preserves custom time filters without claiming mapped error coverage', () => {
  expect(retryLogQuery({ timeRange: 'custom', customStartTime: 'start', customEndTime: 'end', refreshToken: 0 })).toEqual(expect.objectContaining({ start_time: 'start', end_time: 'end', time_range: undefined }))
  expect(retryEventLabel('skipped')).toBe('未重试')
})
