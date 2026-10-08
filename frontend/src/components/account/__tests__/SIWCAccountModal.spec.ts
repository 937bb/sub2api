import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SIWCAccountModal from '../SIWCAccountModal.vue'
import { startSIWCAuthorization, createSIWCAccount } from '@/api/admin/siwc'

vi.mock('@/api/admin/siwc', () => ({ startSIWCAuthorization: vi.fn(), createSIWCAccount: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const authorization = { auth_url: 'https://auth.openai.com/api/accounts/authorize?state=test', session_id: 'session-test', host_id: 'urn:uuid:host-test' }
function form(account: { id: number } | null = null) {
  return mount(SIWCAccountModal, {
    props: { show: true, account, proxies: [{ id: 1, name: 'server-proxy' }], groups: [{ id: 3, name: 'test-group', platform: 'openai' }] },
    global: { stubs: { BaseDialog: { template: '<section><slot /></section>' } } }
  })
}
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  vi.mocked(startSIWCAuthorization).mockResolvedValue(authorization)
  vi.mocked(createSIWCAccount).mockResolvedValue({ id: 7 } as never)
})

describe('SIWC authorization form', () => {
  it('uses the chosen server proxy, persists only host identity and sends the complete callback', async () => {
    const wrapper = form()
    await wrapper.get('select').setValue('1')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(startSIWCAuthorization).toHaveBeenCalledWith(1, undefined, undefined)
    expect(localStorage.getItem('sub2api-siwc-host-id')).toBe(authorization.host_id)
    expect(wrapper.get('a').attributes('rel')).toBe('noopener noreferrer')
    const callback = 'http://127.0.0.1:1455/auth/callback?code=test&state=test&client_id=oaiapp_test'
    await wrapper.get('textarea').setValue(callback)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(createSIWCAccount).toHaveBeenCalledWith(expect.objectContaining({ session_id: 'session-test', callback_url: callback, group_ids: [] }))
    expect(wrapper.emitted('created')).toHaveLength(1)
    expect(localStorage.length).toBe(1)
  })

  it('binds reauthorization to the existing account and keeps a failed save retryable', async () => {
    const wrapper = form({ id: 97 })
    expect(wrapper.find('select').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(startSIWCAuthorization).toHaveBeenCalledWith(null, undefined, 97)
    expect(localStorage.length).toBe(0)
    vi.mocked(createSIWCAccount).mockRejectedValueOnce({ response: { data: { message: 'retry saving' } } })
    await wrapper.get('textarea').setValue('complete-callback')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('retry saving')
    expect(wrapper.emitted('created')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(createSIWCAccount).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 97, session_id: 'session-test' }))
    expect(wrapper.emitted('created')).toHaveLength(1)
  })
})
