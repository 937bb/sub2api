import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import TablePageLayout from '../TablePageLayout.vue'

vi.mock('vue-router', () => ({ useRoute: () => ({ meta: { title: 'Accounts' } }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/router/title', () => ({ resolveRouteMetaKeys: () => ({}) }))

describe('TablePageLayout query dock', () => {
  it('preserves entered filters while hiding and reopening the query dock', async () => {
    const wrapper = mount(TablePageLayout, { attachTo: document.body,
      props: { total: 20, rows: [{ status: 'active' }, { status: 'error' }] },
      slots: { filters: '<input aria-label="Search" />', table: '<table><tbody><tr><td>Account</td></tr></tbody></table>', actions: '<button>Create</button>' }
    })
    const toggle = wrapper.get('.workbench-filter-toggle')
    if(toggle.attributes('aria-expanded') !== 'true') await toggle.trigger('click')
    await wrapper.get('input').setValue('OpenAI')
    await toggle.trigger('click')
    expect(wrapper.get('aside').isVisible()).toBe(false)
    await toggle.trigger('click')
    expect(wrapper.get('aside').isVisible()).toBe(true)
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('OpenAI')
    expect(wrapper.get('.workbench-collection-summary').text()).toContain('20')
    expect(wrapper.get('.workbench-status-counts').text()).toContain('console.currentPageOnly')
    wrapper.unmount()
  })
})
