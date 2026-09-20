import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexTurnStatesView from '../../CodexTurnStatesView.vue'
import type { CodexTurnStateAccountStatus } from '@/api/admin/ops'

const { listAccounts, scanAccount, listProxies, listHistory, getSummary } = vi.hoisted(() => ({
  listAccounts: vi.fn(), scanAccount: vi.fn(), listProxies: vi.fn(), listHistory: vi.fn(), getSummary: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({ opsAPI: {
  listCodexTurnStateAccounts: listAccounts,
  scanCodexTurnState: scanAccount,
  listCodexTurnStateProxies: listProxies,
  listCodexTurnStates: listHistory,
  getCodexTurnStateOperationsSummary: getSummary
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/usePersistedPageSize', () => ({ getPersistedPageSize: () => 50 }))

const accounts: CodexTurnStateAccountStatus[] = [
  { account_id: 1, account_name: 'Team A', account_type: 'oauth', plan_type: 'team', model: 'gpt-5.6-terra', status: 'missing', state_length: 292, target_lengths: [286], attempt_count: 1 },
  { account_id: 2, account_name: 'Team B', account_type: 'oauth', plan_type: 'team', model: 'gpt-6-astra', status: 'missing', state_length: 292, target_lengths: [273], attempt_count: 1 }
]
const page = (items = accounts) => ({ items, total: items.length, page: 1, page_size: 50 })

function render() {
  return mount(CodexTurnStatesView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    TablePageLayout: { template: '<section><slot name="actions"/><slot name="filters"/><slot name="table"/><slot name="pagination"/></section>' },
    DataTable: { props: ['data'], template: '<div><article v-for="row in data" :key="row.row_key" :data-account="row.account_id"><slot name="cell-state" :row="row"/><slot name="cell-actions" :row="row"/></article></div>' },
    CodexStateStatus: { props: ['status', 'stateLength', 'targetLengths'], template: '<span>{{ status }} {{ stateLength }} / {{ targetLengths }}</span>' },
    CodexScanSettingsDialog: { props: ['show', 'selection'], template: '<aside v-if="show" data-test="settings-dialog">{{ selection }}</aside>' },
    Icon: true, Select: true, Pagination: true, ConfirmDialog: true, BaseDialog: true
  } } })
}

describe('CodexTurnStatesView rules', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    listAccounts.mockResolvedValue(page())
    getSummary.mockResolvedValue({})
    listProxies.mockResolvedValue([])
    listHistory.mockResolvedValue(page([]))
    scanAccount.mockResolvedValue({ queued: true })
  })
  afterEach(() => { vi.useRealTimers() })

  it('opens the selected account/model rule without starting any scan', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-account="1"]').text()).toContain('286')
    await wrapper.get('[data-account="1"] [data-test="edit-state-target"]').trigger('click')
    expect(wrapper.get('[data-test="settings-dialog"]').text()).toContain('gpt-5.6-terra')
    expect(wrapper.get('[data-test="settings-dialog"]').text()).toContain('286')
    expect(scanAccount).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps scan settings available on every tab', async () => {
    const wrapper = render()
    await flushPromises()
    for (const tab of wrapper.findAll('[role="tab"]')) {
      await tab.trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-test="state-scan-settings"]').exists()).toBe(true)
    }
    wrapper.unmount()
  })

  it('queues and updates only the clicked account/model while rule actions remain separate', async () => {
    const wrapper = render()
    await flushPromises()
    const selector = '[aria-label="admin.ops.turnState.scanNow"]'
    await wrapper.get(`[data-account="1"] ${selector}`).trigger('click')
    await flushPromises()
    expect(scanAccount).toHaveBeenCalledOnce()
    expect(scanAccount).toHaveBeenCalledWith(1, 'gpt-5.6-terra')
    expect(wrapper.get(`[data-account="1"] ${selector}`).attributes('disabled')).toBeDefined()
    expect(wrapper.get(`[data-account="2"] ${selector}`).attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-account="1"]').text()).toContain('pending')
    expect(wrapper.get('[data-account="2"]').text()).toContain('missing')
    listAccounts.mockResolvedValueOnce(page([{ ...accounts[0], status: 'ready', state_length: 286 }]))
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    expect(wrapper.get('[data-account="1"]').text()).toContain('ready')
    expect(wrapper.get('[data-account="2"]').text()).toContain('missing')
    wrapper.unmount()
  })
})
