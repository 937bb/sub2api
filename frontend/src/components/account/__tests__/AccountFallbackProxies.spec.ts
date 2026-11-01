import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import AccountFallbackProxies from '../AccountFallbackProxies.vue'
import type { Proxy } from '@/types'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
const Selector = defineComponent({ props: ['modelValue', 'proxies'], template: '<div />' })
const proxies = [1, 2, 3].map(id => ({ id, name: `proxy-${id}`, status: 'active' })) as Proxy[]
const mountEditor = (ids: number[] = []) => mount(AccountFallbackProxies, {
  props: { modelValue: ids, primaryId: 1, proxies }, global: { stubs: { ProxySelector: Selector } }
})

describe('account fallback proxy selection', () => {
  it('searches without losing selections and keeps the original proxy checked', async () => {
    const wrapper = mountEditor([2])
    const primary = wrapper.find('input[aria-label="proxy-1"]')
    expect((primary.element as HTMLInputElement).checked).toBe(true)
    expect((primary.element as HTMLInputElement).disabled).toBe(true)
    await wrapper.find('input[type="search"]').setValue('proxy-3')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(1)
    await wrapper.find('input[aria-label="proxy-3"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[2, 3]])
  })
  it('allows deselecting an unavailable selected proxy but blocks new unavailable selections', async () => {
    const wrapper = mountEditor([2])
    await wrapper.setProps({ proxies: proxies.map(p => ({ ...p, status: 'inactive' })) })
    expect((wrapper.find('input[aria-label="proxy-3"]').element as HTMLInputElement).disabled).toBe(true)
    await wrapper.find('input[aria-label="proxy-2"]').setValue(false)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]])
  })
  it('excludes the primary and already selected proxies when adding', async () => {
    const wrapper = mountEditor([2])
    await wrapper.find('input[aria-label="proxy-3"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[2, 3]])
  })
  it('preserves priority order and allows removing a binding', async () => {
    const wrapper = mountEditor([2, 3])
    await wrapper.findAll('button')[2].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[3, 2]])
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([[3]])
  })
  it('clears backups when changing to a direct connection', async () => {
    const wrapper = mountEditor([2])
    await wrapper.setProps({ primaryId: null })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]])
  })
  it('removes a duplicate when a backup becomes the primary', async () => {
    const wrapper = mountEditor([2, 3])
    await wrapper.setProps({ primaryId: 2 })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[3]])
  })
})
