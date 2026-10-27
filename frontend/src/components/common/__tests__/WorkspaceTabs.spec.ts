import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import WorkspaceTabs from '../WorkspaceTabs.vue'

describe('WorkspaceTabs', () => {
  it('supports keyboard navigation and retains a single keyboard entry point', async () => {
    const wrapper = mount(WorkspaceTabs, { attachTo: document.body, props: {
      modelValue: 'identity', label: 'Profile',
      tabs: [{ value: 'identity', label: 'Identity' }, { value: 'security', label: 'Security' }]
    } })
    const tabs = wrapper.findAll('[role="tab"]')
    await tabs[0].trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['security'])
    expect(document.activeElement).toBe(tabs[1].element)
    await wrapper.setProps({ modelValue: 'security' })
    expect(tabs[0].attributes('tabindex')).toBe('-1')
    expect(tabs[1].attributes('aria-selected')).toBe('true')
    await tabs[1].trigger('keydown', { key: 'Home' })
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual(['identity'])
    wrapper.unmount()
  })
})
