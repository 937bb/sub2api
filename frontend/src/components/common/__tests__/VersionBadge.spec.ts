import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import VersionBadge from '../VersionBadge.vue'

const mocks = vi.hoisted(() => ({
  auth: { isAdmin: true },
  app: { versionLoading: false, currentVersion: '', latestVersion: '', hasUpdate: false, releaseInfo: null, buildType: 'source', fetchVersion: vi.fn() }
}))
vi.mock('@/stores', () => ({ useAuthStore: () => mocks.auth, useAppStore: () => mocks.app }))
vi.mock('@/api/admin/system', () => ({ performUpdate: vi.fn(), restartService: vi.fn(), getRollbackVersions: vi.fn(), rollback: vi.fn() }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

afterEach(() => { document.body.innerHTML = ''; mocks.auth.isAdmin = true; vi.restoreAllMocks() })

describe('VersionBadge viewport and dismissal', () => {
  it.each(['0.2.17-937sub2b', 'v0.2.17-937sub2b'])('normalizes the display prefix for %s', (version) => {
    const wrapper = mount(VersionBadge, { props: { version } })
    expect(wrapper.get('button').text()).toBe('v0.2.17-937sub2b')
    wrapper.unmount()
  })

  it('portals above a bottom trigger, closes on Escape, and restores focus', async () => {
    const wrapper = mount(VersionBadge, { attachTo: document.body, props: { version: 'v0.2.17-937sub2b' } })
    const trigger = wrapper.get('button')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({ x: 16, y: 740, top: 740, bottom: 768, left: 16, right: 220, width: 204, height: 28, toJSON: () => ({}) })
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(400)
    await trigger.trigger('click')
    await flushPromises()
    const panel = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(panel.parentElement).toBe(document.body)
    expect(Number.parseFloat(panel.style.top)).toBeLessThan(740)
    expect(Number.parseFloat(panel.style.top) + 400).toBeLessThanOrEqual(window.innerHeight - 16)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    await vi.waitFor(() => expect(document.querySelector('[role="dialog"]')).toBeNull())
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })

  it('closes when another button is clicked, and keeps admin actions hidden from users', async () => {
    const wrapper = mount(VersionBadge, { attachTo: document.body, props: { version: 'v0.2.17' } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const outside = document.createElement('button')
    document.body.append(outside)
    outside.click()
    await flushPromises()
    await vi.waitFor(() => expect(document.querySelector('[role="dialog"]')).toBeNull())
    wrapper.unmount()
    mocks.auth.isAdmin = false
    const regular = mount(VersionBadge, { props: { version: 'v0.2.17' } })
    expect(regular.find('button').exists()).toBe(false)
    expect(regular.text()).toBe('v0.2.17')
    regular.unmount()
  })
})
