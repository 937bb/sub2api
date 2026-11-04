import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountFingerprintModal from '../AccountFingerprintModal.vue'
import { getFingerprint, type AccountFingerprint } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({ getFingerprint: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
const result = (id: number): AccountFingerprint => ({ account_id: id, credential_account_id: id, inherited: false, installation_id: `device-${id}`, source: 'persisted_seed', convergence_mode: 'off', convergence_ready: true, user_agent_override: '', ws_account_mode: 'default' })
const render = () => mount(AccountFingerprintModal, { props: { show: true, account: { id: 1, name: 'First' } }, global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, LoadingSpinner: true } } })

describe('AccountFingerprintModal', () => {
  beforeEach(() => vi.clearAllMocks())
  it('discards an earlier account response when switching accounts', async () => {
    let resolveFirst!: (data: AccountFingerprint) => void
    vi.mocked(getFingerprint).mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve })).mockResolvedValueOnce(result(2))
    const wrapper = render()
    const signal = vi.mocked(getFingerprint).mock.calls[0][1]!
    await wrapper.setProps({ account: { id: 2, name: 'Second' } })
    await flushPromises()
    resolveFirst(result(1))
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(wrapper.text()).toContain('device-2')
    expect(wrapper.text()).not.toContain('device-1')
    wrapper.unmount()
  })
  it('clears the previous result on refresh failure and allows retry', async () => {
    vi.mocked(getFingerprint).mockResolvedValueOnce(result(1)).mockRejectedValueOnce(new Error('failed')).mockResolvedValueOnce(result(1))
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'common.refresh')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('device-1')
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('device-1')
    wrapper.unmount()
  })
  it('cancels the request when closed and reloads when reopened', async () => {
    vi.mocked(getFingerprint).mockResolvedValue(result(1))
    const wrapper = render()
    await flushPromises()
    const signal = vi.mocked(getFingerprint).mock.calls[0][1]!
    await wrapper.setProps({ show: false })
    expect(signal.aborted).toBe(true)
    expect(wrapper.text()).not.toContain('device-1')
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(getFingerprint).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('device-1')
    wrapper.unmount()
  })
})
