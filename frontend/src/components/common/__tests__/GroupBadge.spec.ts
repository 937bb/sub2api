import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import GroupBadge from '@/components/common/GroupBadge.vue'

const i18n = createI18n({
  legacy: false,
  messages: {
    en: {
      groups: { subscription: 'Subscription' }
    }
  },
  locale: 'en'
})

function mountBadge(props: InstanceType<typeof GroupBadge>['$props']) {
  return mount(GroupBadge, {
    props,
    global: {
      plugins: [i18n],
      stubs: { PlatformIcon: true }
    }
  })
}

describe('GroupBadge palette', () => {
  it('uses the Antigravity standard badge classes', () => {
    const wrapper = mountBadge({
      name: 'Antigravity',
      platform: 'antigravity',
      rateMultiplier: 1
    })

    expect(wrapper.classes()).toEqual(
      expect.arrayContaining([
        'bg-fuchsia-50',
        'text-fuchsia-700',
        'dark:bg-fuchsia-900/20',
        'dark:text-fuchsia-400'
      ])
    )
  })

  it('uses the Antigravity subscription badge and label classes', () => {
    const wrapper = mountBadge({
      name: 'Antigravity',
      platform: 'antigravity',
      subscriptionType: 'subscription'
    })

    expect(wrapper.classes()).toEqual(
      expect.arrayContaining([
        'bg-purple-100',
        'text-purple-700',
        'dark:bg-purple-900/30',
        'dark:text-purple-400'
      ])
    )
    expect(wrapper.get('span > span:last-child').classes()).toEqual(
      expect.arrayContaining([
        'bg-purple-200/60',
        'text-purple-800',
        'dark:bg-purple-800/40',
        'dark:text-purple-300'
      ])
    )
  })

  it.each([
    ['anthropic', 'bg-amber-50', 'text-amber-700'],
    ['openai', 'bg-green-50', 'text-green-700'],
    ['gemini', 'bg-sky-50', 'text-sky-700']
  ] as const)('keeps the %s standard palette', (platform, background, text) => {
    const wrapper = mountBadge({ name: platform, platform })

    expect(wrapper.classes()).toEqual(expect.arrayContaining([background, text]))
  })

  it('keeps the fallback standard palette', () => {
    const wrapper = mountBadge({ name: 'Fallback' })

    expect(wrapper.classes()).toEqual(expect.arrayContaining(['bg-emerald-100', 'text-emerald-700']))
  })
})
