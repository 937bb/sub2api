import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HomeView from '@/views/HomeView.vue'

const { appStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: null as Record<string, string> | null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
}))

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({
    user: null,
    isAuthenticated: false,
    isAdmin: false,
    checkAuth: vi.fn(),
  }),
}))

const stubs = { RouterLink: true, LocaleSwitcher: true, Icon: true }

function expectNoActiveContent(wrapper: ReturnType<typeof mount>) {
  const root = wrapper.element as HTMLElement

  expect(root.querySelector('script, iframe, object, embed, base, meta[http-equiv]')).toBeNull()
  expect(root.querySelector('[srcdoc], [srcset], [formaction], [action], [style]')).toBeNull()

  for (const element of root.querySelectorAll('*')) {
    for (const attribute of element.attributes) {
      expect(attribute.name.toLowerCase()).not.toMatch(/^on/)
      if (['href', 'src', 'xlink:href'].includes(attribute.name.toLowerCase())) {
        const normalizedUrl = [...attribute.value]
          .filter(character => {
            const codePoint = character.codePointAt(0) ?? 0
            return codePoint > 0x20 && (codePoint < 0x7f || codePoint > 0x9f)
          })
          .join('')
          .toLowerCase()
        expect(normalizedUrl).not.toMatch(/^(?:javascript|data):/)
      }
    }
  }
}

describe('HomeView configured content boundary', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = null
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false }),
    })
  })

  it('sanitizes active HTML while preserving ordinary rich content', () => {
    appStore.cachedPublicSettings = {
      home_content: '<section><h1>Welcome</h1><a href="https://example.com">Docs</a><img src=x onerror="alert(1)"><script>alert(1)</script></section>',
    }

    const wrapper = mount(HomeView, { global: { stubs } })

    expect(wrapper.find('section h1').text()).toBe('Welcome')
    expect(wrapper.find('a').attributes('href')).toBe('https://example.com')
    expect(wrapper.html()).not.toContain('onerror')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('iframe').exists()).toBe(false)
  })

  it('normalizes HTTP(S) iframe URLs and applies a restrictive sandbox', () => {
    appStore.cachedPublicSettings = { home_content: ' https://portal.example.com/home ' }

    const iframe = mount(HomeView, { global: { stubs } }).get('iframe')

    expect(iframe.attributes('src')).toBe('https://portal.example.com/home')
    expect(iframe.attributes('sandbox')).toBe('allow-scripts')
    expect(iframe.attributes('sandbox')).not.toContain('allow-popups')
    expect(iframe.attributes('sandbox')).not.toContain('allow-same-origin')
    expect(iframe.attributes('sandbox')).not.toContain('allow-top-navigation')
    expect(iframe.attributes('referrerpolicy')).toBe('strict-origin-when-cross-origin')
  })

  it.each(['javascript:alert(1)', '//attacker.example/home', 'data:text/html,<script>alert(1)</script>'])(
    'does not create an iframe for unsafe URL-like content %j',
    (homeContent) => {
      appStore.cachedPublicSettings = { home_content: homeContent }
      const wrapper = mount(HomeView, { global: { stubs } })

      expect(wrapper.find('iframe').exists()).toBe(false)
      expect(wrapper.find('script').exists()).toBe(false)
    }
  )

  it.each([
    ['encoded javascript URL', '<a href="java&#x73;cript:alert(1)">open</a>'],
    ['SVG mutation payload', '<svg><a xlink:href="javascript:alert(1)"><text>open</text></a><script>alert(1)</script></svg>'],
    ['MathML mutation payload', '<math><mtext><img src=x onerror=alert(1)></mtext></math>'],
    ['srcdoc', '<iframe srcdoc="<script>alert(1)</script>"></iframe>'],
    ['srcset', '<img src="https://example.com/a.png" srcset="javascript:alert(1) 1x">'],
    ['form actions', '<form action="javascript:alert(1)"><button formaction="javascript:alert(2)">go</button></form>'],
    ['CSS injection', '<div style="background:url(javascript:alert(1))">content</div><style>@import "https://attacker.example/x.css"</style>'],
  ])('removes active DOM sinks from %s', (_name, homeContent) => {
    appStore.cachedPublicSettings = { home_content: homeContent }

    const wrapper = mount(HomeView, { global: { stubs } })

    expect(wrapper.text()).toContain(homeContent.includes('content') ? 'content' : '')
    expectNoActiveContent(wrapper)
  })

  it('renders an unsafe URL fallback without creating an active URL sink', () => {
    appStore.cachedPublicSettings = {
      home_content: '<a href="jav&#x61;script:alert(1)">unsafe fallback</a>',
    }

    const wrapper = mount(HomeView, { global: { stubs } })

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.text()).toContain('unsafe fallback')
    expectNoActiveContent(wrapper)
  })
})
