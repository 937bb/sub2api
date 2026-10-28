import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpsErrorLogTable from '../OpsErrorLogTable.vue'
import zhLocale from '@/i18n/locales/zh'
import enLocale from '@/i18n/locales/en'
import type { OpsErrorLog } from '@/api/admin/ops'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const TooltipStub = { template: '<div><slot /></div>' }
const PaginationStub = { template: '<div class="pagination-stub" />' }

function mountTable(row: Partial<OpsErrorLog>) {
  const base = {
    id: 1,
    created_at: '2026-06-05T23:59:50Z',
    phase: 'upstream',
    type: '',
    error_owner: 'provider',
    error_source: 'upstream_http',
    severity: 'error',
    status_code: 529,
    platform: 'anthropic',
    model: 'claude-opus-4-8',
    resolved: false,
    client_request_id: '',
    request_id: 'req-1',
    message: 'boom',
    user_email: '',
    account_name: '',
    group_name: '',
    ...row,
  } as OpsErrorLog

  return mount(OpsErrorLogTable, {
    props: { rows: [base], total: 1, loading: false, page: 1, pageSize: 20 },
    global: { stubs: { 'el-tooltip': TooltipStub, Pagination: PaginationStub } },
  })
}

describe('OpsErrorLogTable user/api-key/account columns', () => {
  // Upstream rows must retain user, API key, and account identities separately.
  it('renders user, api key and account in separate columns for an upstream row', () => {
    const wrapper = mountTable({
      user_id: 2,
      user_email: 'alice@test.com',
      api_key_id: 5,
      api_key_name: 'my-key',
      account_id: 9,
      account_name: 'acct-A',
    })

    const text = wrapper.text()
    expect(text).toContain('alice@test.com')
    expect(text).toContain('my-key')
    expect(text).toContain('acct-A')
  })

  it('shows the deleted badge for a soft-deleted api key', () => {
    const wrapper = mountTable({
      api_key_id: 5,
      api_key_name: 'old-key',
      api_key_deleted: true,
    })

    expect(wrapper.text()).toContain('old-key')
    expect(wrapper.text()).toContain('admin.ops.errorLog.keyDeletedBadge')
  })
})

describe('OpsErrorLogTable column order', () => {
  it('puts time and response content first for ops without changing time sorting', async () => {
    const wrapper = mountTable({})
    await wrapper.setProps({ summaryFirst: true })

    const headers = wrapper.findAll('thead th')
    expect(headers.slice(0, 3).map((header) => header.text())).toEqual([
      'admin.ops.errorLog.time',
      'admin.ops.errorLog.message',
      'admin.ops.errorLog.user',
    ])
    expect(wrapper.findAll('tbody td')[1].text()).toBe('boom')

    await headers[0].trigger('click')
    expect(wrapper.emitted('sort')).toEqual([['created_at', 'asc']])
    wrapper.unmount()
  })

  it('preserves the standalone column order and visibility by default', async () => {
    const wrapper = mountTable({})
    await wrapper.setProps({ visibleColumnKeys: ['created_at', 'user', 'message'] })

    expect(wrapper.findAll('thead th').map((header) => header.text())).toEqual([
      'admin.ops.errorLog.user',
      'admin.ops.errorLog.message',
      'admin.ops.errorLog.time',
    ])
    wrapper.unmount()
  })

  it('aligns embedded records while preserving hidden columns, sorting and details', async () => {
    const wrapper = mountTable({ user_id: 2, user_email: 'alice@test.com' })
    await wrapper.setProps({ flat: true, visibleColumnKeys: ['user', 'message', 'created_at', 'actions'] })

    const headers = wrapper.findAll('thead th')
    expect(headers.map((header) => header.text())).toEqual([
      'admin.ops.errorLog.time', 'admin.ops.errorLog.user', 'admin.ops.errorLog.message', 'admin.ops.errorLog.action',
    ])
    expect(wrapper.findAll('tbody td')[1].text()).toContain('alice@test.com')
    await headers[0].trigger('click')
    expect(wrapper.emitted('sort')).toEqual([['created_at', 'asc']])
    await wrapper.get('button[aria-label="admin.ops.errorLog.details"]').trigger('click')
    expect(wrapper.emitted('openErrorDetail')).toEqual([[1]])
    wrapper.unmount()
  })
})

// Check actual locale keys because the runtime-only test translator returns the key.
describe('OpsErrorLogTable i18n keys exist in the errorLog namespace', () => {
  const locales: Record<string, any> = { zh: zhLocale, en: enLocale }
  for (const [name, msgs] of Object.entries(locales)) {
    it(`has apiKey & keyDeletedBadge for ${name}`, () => {
      const errorLog = msgs?.admin?.ops?.errorLog
      expect(errorLog?.apiKey).toBeTruthy()
      expect(errorLog?.keyDeletedBadge).toBeTruthy()
    })
  }
})
