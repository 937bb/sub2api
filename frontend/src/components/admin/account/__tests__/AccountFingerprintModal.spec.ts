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
  it('shows resolved runtime values and explicitly unmeasured transport details', async () => {
    vi.mocked(getFingerprint).mockResolvedValue({ ...result(1), runtime: {
      scope: 'serving_admin_process', user_agent: 'codex_cli_rs/1.2.3', originator: 'codex_cli_rs', version: '1.2.3',
      identity_enforced: true, local_cli_transport: true, http_profile: 'native-http', custom_ca: false,
      force_http: true, ws_router_enabled: false, strict_conversation_binding: true, capture_status: 'not_captured', dynamic_headers: ['session-id']
    }, transports: [{
      tls: { Name: 'native-http', MinVersion: 771, MaxVersion: 771, ShuffleExtensions: false, CipherSuites: [49196], Curves: [23], PointFormats: [0], EnableGREASE: false, SignatureAlgorithms: [1025], ALPNProtocols: null, SupportedVersions: null, KeyShareGroups: null, PSKModes: null, Extensions: [0, 10] },
      configuration_hash: 'config-hash', evidence: 'reference_capture_2026-11-03', protocol: 'HTTP/1.1', selected_by_current_http: true, header_order: ['Host', 'User-Agent']
    }] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('codex_cli_rs/1.2.3')
    expect(wrapper.text()).toContain('admin.accounts.fingerprint.strict')
    expect(wrapper.text()).toContain('admin.accounts.fingerprint.evidenceNote')
    expect(wrapper.text()).toContain('TLS 1.2')
    expect(wrapper.find('details').attributes()).toHaveProperty('open')
    wrapper.unmount()
  })

})
