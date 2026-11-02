import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { nextTick } from 'vue'

import HomeView from '../HomeView.vue'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Fallback site',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
    checkAuth: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = {
    site_name: 'Test site',
    site_subtitle: 'Test subtitle',
    ...settings,
  }

  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

function compactDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('[data-testid="compact-home"]').findComponent(RouterLinkStub).props('to')
}

function modelPlazaDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper
    .findAllComponents(RouterLinkStub)
    .find((link) => link.props('to') === '/model-plaza')
    ?.props('to')
}

describe('HomeView compact mode', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    authStore.checkAuth.mockClear()
    appStore.fetchPublicSettings.mockClear()
    localStorage.clear()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  it('renders custom HTML ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: '<section id="custom-home">Custom home</section>',
    })

    expect(wrapper.get('#custom-home').text()).toBe('Custom home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('renders custom URL content ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: ' https://example.com/home ',
    })

    expect(wrapper.get('iframe').attributes('src')).toBe('https://example.com/home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('treats whitespace-only custom content as empty and selects compact mode', () => {
    const wrapper = mountHome({ compact_home_enabled: true, home_content: ' \n\t ' })

    expect(wrapper.get('[data-testid="compact-home"]').text()).toContain('Test site')
  })

  it.each([undefined, false])('selects the default home when compact mode is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { compact_home_enabled: enabled }
    const wrapper = mountHome(settings)

    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="default-home"]').exists()).toBe(true)
  })

  it('links unauthenticated visitors to login', () => {
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/login')
  })

  it('links authenticated users to their dashboard', () => {
    authStore.isAuthenticated = true

    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/dashboard')
  })

  it('links administrators to the admin dashboard', () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true

    const wrapper = mountHome({ compact_home_enabled: true })
    expect(compactDestination(wrapper)).toBe('/admin/dashboard')
    expect(authStore.checkAuth).toHaveBeenCalledOnce()
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it('shows the model plaza link to anonymous visitors when public access is enabled', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      model_plaza_enabled: true,
      model_plaza_require_auth: false,
    })

    expect(modelPlazaDestination(wrapper)).toBe('/model-plaza')
  })

  it('hides the model plaza link from anonymous visitors when sign-in is required', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      model_plaza_enabled: true,
      model_plaza_require_auth: true,
    })

    expect(modelPlazaDestination(wrapper)).toBeUndefined()
  })

  it('shows the model plaza link to authenticated visitors when sign-in is required', () => {
    authStore.isAuthenticated = true

    const wrapper = mountHome({
      compact_home_enabled: true,
      model_plaza_enabled: true,
      model_plaza_require_auth: true,
    })

    expect(modelPlazaDestination(wrapper)).toBe('/model-plaza')
  })

  it('shows the model plaza link in the default home header', () => {
    const wrapper = mountHome({
      model_plaza_enabled: true,
      model_plaza_require_auth: false,
    })

    expect(modelPlazaDestination(wrapper)).toBe('/model-plaza')
  })

  it('hides the model plaza link when the feature is disabled', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      model_plaza_enabled: false,
      model_plaza_require_auth: false,
    })

    expect(modelPlazaDestination(wrapper)).toBeUndefined()
  })

  it('switches examples without sending a request and uses the configured API base', async () => {
    const wrapper = mountHome({ api_base_url: 'https://api.example.com/v1/' })
    const code = () => wrapper.get('.home-code').text()
    expect(code()).toContain('https://api.example.com/v1/responses')
    expect(code()).toContain('Authorization: Bearer YOUR_API_KEY')

    await wrapper.get('.home-protocols button:nth-child(3)').trigger('click')
    expect(code()).toContain('https://api.example.com/v1/messages')
    expect(code()).toContain('anthropic-version: 2023-06-01')
    expect(code()).toContain('"max_tokens": 1024')
    expect(code()).not.toContain('Authorization: Bearer')

    await wrapper.get('.home-protocols button:nth-child(2)').trigger('click')
    expect(code()).toContain('https://api.example.com/v1/chat/completions')
    expect(code()).toContain('"messages"')
    expect(code()).not.toContain('anthropic-version')
  })

  it('preserves configured branding and subtitle in the default home', () => {
    const wrapper = mountHome({ site_name: 'Custom brand', site_subtitle: 'Custom introduction' })
    expect(wrapper.get('.home-brand').text()).toBe('Custom brand')
    expect(wrapper.get('.home-hero-content').text()).toContain('Custom introduction')
  })

  it('retains visibility tracking after pausing and resumes without losing content', async () => {
    let notify: IntersectionObserverCallback | undefined
    const disconnect = vi.fn()
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback: IntersectionObserverCallback) { notify = callback }
      observe = vi.fn()
      disconnect = disconnect
    })
    const wrapper = mountHome()
    try {
      await nextTick()
      await nextTick()
      const root = wrapper.get('[data-testid="default-home"]')
      expect(root.classes()).toContain('motion-ready')
      const item = wrapper.get('.home-example').element
      notify?.([{ target: item, isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
      expect(item.classList.contains('is-revealed')).toBe(true)
      await wrapper.get('.home-motion-toggle').trigger('click')
      expect(root.classes()).toContain('motion-paused')
      expect(root.classes()).toContain('motion-ready')
      await wrapper.get('.home-motion-toggle').trigger('click')
      expect(root.classes()).not.toContain('motion-paused')
      expect(root.classes()).toContain('motion-ready')
      notify?.([{ target: item, isIntersecting: false } as IntersectionObserverEntry], {} as IntersectionObserver)
      expect(item.classList.contains('is-in-view')).toBe(false)
      expect(item.classList.contains('is-revealed')).toBe(true)
      wrapper.unmount()
      expect(disconnect).toHaveBeenCalledOnce()
    } finally {
      wrapper.unmount()
      vi.unstubAllGlobals()
    }
  })
})
