import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGroup } from '@/types'
import GroupSmartWSModal from '../GroupSmartWSModal.vue'

const api = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn() }))
vi.mock('@/api/admin/groups', () => ({ getSmartWS: api.get, setSmartWS: api.set }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function group(id: number) { return { id, name: `Group ${id}`, platform: 'openai' } as AdminGroup }
function render() {
  return mount(GroupSmartWSModal, {
    props: { show: true, group: group(2) },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
  })
}

describe('GroupSmartWSModal', () => {
  beforeEach(() => vi.resetAllMocks())

  it('shows process-scoped account metrics without treating leases as completed work', async () => {
    api.get.mockResolvedValue({ enabled: true, revision: 2, runtime: {
      scope: 'process', tuning_error: 'invalid SMART_WS_COLD_WAIT_MS', accounts: [{
        account_hash: 'hash', account_ids: [42], connections: 3, active: 2, idle: 1,
        connecting: 0, waiting: 4, waiting_bytes: 100,
        counters: { opened: 2, reused: 6, completed: 5, handshake_429: 1 }
      }]
    } })
    const wrapper = render()
    await flushPromises()
    const runtime = wrapper.get('[data-testid="smartws-runtime"]')
    expect(runtime.text()).toContain('admin.groups.smartWS.runtimeHint')
    expect(runtime.text()).toContain('invalid SMART_WS_COLD_WAIT_MS')
    expect(runtime.findAll('td').map(cell => cell.text())).toEqual(['#42', '2', '1', '0', '4', '75.0%', '5', '1'])
    wrapper.unmount()
  })

  it('supports an empty snapshot and older servers without runtime metrics', async () => {
    api.get.mockResolvedValueOnce({ enabled: true, revision: 2, runtime: { accounts: [] } })
      .mockResolvedValueOnce({ enabled: false, revision: 3 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.groups.smartWS.runtimeEmpty')
    await wrapper.setProps({ group: group(4) })
    await flushPromises()
    expect(wrapper.find('[data-testid="smartws-runtime"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('ignores a late response after switching groups', async () => {
    let first!: (value: unknown) => void
    api.get.mockImplementationOnce(() => new Promise(resolve => { first = resolve }))
      .mockResolvedValueOnce({ enabled: false, revision: 8 })
    api.set.mockResolvedValue({ enabled: true, revision: 9 })
    const wrapper = render()
    await wrapper.setProps({ group: group(4) })
    await flushPromises()
    first({ enabled: true, revision: 2 })
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('input').setValue(true)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.set).toHaveBeenCalledWith(4, { enabled: true, revision: 8 })
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('reloads the latest revision after a conflict without overwriting it', async () => {
    api.get.mockResolvedValueOnce({ enabled: false, revision: 1 })
      .mockResolvedValueOnce({ enabled: true, revision: 3 })
    api.set.mockRejectedValue({ response: { status: 409 } })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('input').setValue(true)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.set).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.groups.smartWS.conflict')
    expect((wrapper.get('input').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
