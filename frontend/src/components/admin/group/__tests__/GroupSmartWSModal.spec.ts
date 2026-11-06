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
