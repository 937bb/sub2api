import { mount, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppHeader from '@/components/layout/AppHeader.vue'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import HomeView from '@/views/HomeView.vue'
import KeyUsageView from '@/views/KeyUsageView.vue'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: null as Record<string, unknown> | null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    siteVersion: '',
    contactInfo: '',
    publicSettingsLoaded: true,
    sidebarCollapsed: false,
    mobileOpen: false,
    sidebarScrollTop: 0,
    customMenuItems: [],
    fetchPublicSettings: vi.fn(),
    toggleMobileSidebar: vi.fn(),
    closeMobileSidebar: vi.fn(),
  },
  authStore: {
    user: null,
    isAuthenticated: false,
    isAdmin: false,
    isSimpleMode: false,
    checkAuth: vi.fn(),
    logout: vi.fn(),
  },
}))

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard', name: 'Dashboard', params: {}, meta: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
  useOnboardingStore: () => ({ replay: vi.fn() }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    channelMonitor: {}, availableChannels: {}, payment: {}, riskControl: {}, affiliate: {},
  },
  makeSidebarFlag: () => () => true,
}))

const stubs = {
  RouterLink: { template: '<a><slot /></a>' },
  LocaleSwitcher: true,
  Icon: true,
  AnnouncementBell: true,
  SubscriptionProgressMini: true,
  VersionBadge: true,
}

function docsLinks(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('a[target="_blank"][rel="noopener noreferrer"]')
    .filter(link => link.attributes('href') !== 'https://github.com/Wei-Shaw/sub2api')
}

describe('URL sanitization at rendered consumers', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = null
    appStore.siteLogo = ''
    appStore.docUrl = ''
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false }),
    })
  })

  it.each([
    ['AppHeader', AppHeader, shallowMount],
    ['HomeView', HomeView, mount],
    ['KeyUsageView', KeyUsageView, mount],
  ])('suppresses malicious doc_url in %s and renders allowed HTTPS', async (_name, component, mountComponent) => {
    appStore.docUrl = 'javascript:alert(document.domain)'
    const malicious = mountComponent(component, { global: { stubs } })
    expect(docsLinks(malicious as ReturnType<typeof mount>)).toHaveLength(0)
    malicious.unmount()

    appStore.docUrl = 'https://docs.example.com/guide'
    const allowed = mountComponent(component, { global: { stubs } })
    expect(docsLinks(allowed as ReturnType<typeof mount>).map(link => link.attributes('href')))
      .toContain('https://docs.example.com/guide')
    allowed.unmount()
  })

  it.each([
    ['AppSidebar', AppSidebar, shallowMount],
    ['HomeView', HomeView, mount],
    ['KeyUsageView', KeyUsageView, mount],
  ])('suppresses malicious site_logo in %s and permits relative and safe image data logos', (_name, component, mountComponent) => {
    appStore.siteLogo = 'javascript:alert(document.domain)'
    const malicious = mountComponent(component, { global: { stubs } })
    expect(malicious.find('img[alt="Logo"]').attributes('src')).toBe('/logo.png')
    malicious.unmount()

    appStore.siteLogo = '/branding/logo.png'
    const relative = mountComponent(component, { global: { stubs } })
    expect(relative.find('img[alt="Logo"]').attributes('src')).toBe('/branding/logo.png')
    relative.unmount()

    appStore.siteLogo = 'data:image/png;base64,AAAA'
    const dataImage = mountComponent(component, { global: { stubs } })
    expect(dataImage.find('img[alt="Logo"]').attributes('src')).toBe('data:image/png;base64,AAAA')
    dataImage.unmount()
  })
})
