import { mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import App from '@/App.vue'

const { appStoreState } = vi.hoisted(() => ({
  appStoreState: {
    siteLogo: '',
    siteName: 'Sub2API',
    cachedPublicSettings: null,
    fetchPublicSettings: vi.fn().mockResolvedValue(undefined),
  },
}))
const appStore = reactive(appStoreState)

vi.mock('vue-router', () => ({
  RouterView: { template: '<main />' },
  useRoute: () => ({ path: '/', fullPath: '/', name: 'Home', params: {}, meta: {} }),
  useRouter: () => ({ afterEach: vi.fn(), replace: vi.fn() }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({ isAdmin: false, isAuthenticated: false }),
  useSubscriptionStore: () => ({ clear: vi.fn(), startPolling: vi.fn(), fetchActiveSubscriptions: vi.fn() }),
  useAnnouncementStore: () => ({ reset: vi.fn(), fetchAnnouncements: vi.fn() }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn().mockResolvedValue({ needs_setup: false }) }))

describe('App favicon URL consumer', () => {
  beforeEach(() => {
    appStore.siteLogo = ''
    document.head.innerHTML = '<link rel="icon" href="/stale.png">'
  })

  it('renders a safe configured favicon', async () => {
    appStore.siteLogo = '/branding/favicon.png'
    const wrapper = mount(App, { global: { stubs: { NavigationProgress: true, Toast: true, AnnouncementPopup: true } } })
    await nextTick()

    expect(document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.getAttribute('href'))
      .toBe('/branding/favicon.png')
    wrapper.unmount()
  })

  it('resets a previously valid favicon when the setting becomes unsafe', async () => {
    appStore.siteLogo = '/branding/favicon.png'
    const wrapper = mount(App, { global: { stubs: { NavigationProgress: true, Toast: true, AnnouncementPopup: true } } })
    await nextTick()

    appStore.siteLogo = 'javascript:alert(1)'
    await nextTick()

    expect(document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.getAttribute('href'))
      .toBe('/logo.png')
    wrapper.unmount()
  })

  it.each(['javascript:alert(1)', '/\\attacker.example/favicon.png', ''])(
    'resets an unsafe or empty favicon value %j to the default',
    async (siteLogo) => {
      appStore.siteLogo = siteLogo
      const wrapper = mount(App, { global: { stubs: { NavigationProgress: true, Toast: true, AnnouncementPopup: true } } })
      await nextTick()

      expect(document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.getAttribute('href'))
        .toBe('/logo.png')
      wrapper.unmount()
    }
  )
})
