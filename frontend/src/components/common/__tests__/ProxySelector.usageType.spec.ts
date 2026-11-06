import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProxySelector from '../ProxySelector.vue'
import type { Proxy } from '@/types'

const testProxy = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks() })

async function openSelector() {
  const wrapper = mount(ProxySelector, {
    props: { modelValue: 2, proxies: [
      { id: 1, name: 'Kiro static', host: 'kiro.example', port: 8080, protocol: 'http', usage_type: 'kiro' },
      { id: 2, name: 'Pro IPv6', host: '127.0.0.1', port: 26445, protocol: 'http', usage_type: 'pro' },
      { id: 3, name: 'Legacy proxy', host: 'old.example', port: 8080, protocol: 'http' }
    ] as Proxy[] },
    global: { stubs: { Icon: true } }
  })
  await wrapper.get('.select-trigger').trigger('click')
  return wrapper
}

describe('proxy usage categories', () => {
  it('filters by category and search without replacing the selected proxy', async () => {
    const wrapper = await openSelector()
    await wrapper.get('select').setValue('kiro')
    expect(wrapper.get('.select-options').text()).toContain('Kiro static')
    expect(wrapper.get('.select-options').text()).not.toContain('Pro IPv6')
    expect(wrapper.get('.select-trigger').text()).toContain('Pro IPv6')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.get('input').setValue('missing')
    expect(wrapper.find('.select-empty').exists()).toBe(true)
    await wrapper.get('input').setValue(' KIRO.EXAMPLE ')
    await wrapper.findAll('.select-option')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[1]])
  })

  it('treats legacy proxies as general and keeps the direct option available', async () => {
    const wrapper = await openSelector()
    await wrapper.get('select').setValue('general')
    expect(wrapper.get('.select-options').text()).toContain('Legacy proxy')
    expect(wrapper.get('.select-options').text()).not.toContain('Kiro static')
    await wrapper.findAll('.select-option')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
  })

  it('only connection-tests the filtered category', async () => {
    testProxy.mockResolvedValue({ success: true })
    const wrapper = await openSelector()
    await wrapper.get('select').setValue('pro')
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([2])
  })
})
