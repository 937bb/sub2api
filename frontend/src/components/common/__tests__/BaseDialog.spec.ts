import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import BaseDialog from '../BaseDialog.vue'

const getByRole = (role: 'button', { name }: { name: string }) => {
  const selector = role === 'button' ? 'button, [role="button"]' : `[role="${role}"]`
  const element = [...document.body.querySelectorAll<HTMLElement>(selector)].find(
    (element) => element.getAttribute('aria-label') === name
  )

  if (!element) throw new Error(`Unable to find ${role} with accessible name "${name}"`)
  return element
}

describe('BaseDialog', () => {
  it('reactively localizes the accessible name of the close button', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh',
      messages: {
        zh: { common: { close: () => '关闭' } },
        en: { common: { close: () => 'Close' } }
      }
    })

    const wrapper = mount(BaseDialog, {
      props: { show: true, title: '设置' },
      global: { plugins: [i18n] },
      attachTo: document.body
    })

    const closeButton = getByRole('button', { name: '关闭' })

    i18n.global.locale.value = 'en'
    await wrapper.vm.$nextTick()

    expect(getByRole('button', { name: 'Close' })).toBe(closeButton)
    wrapper.unmount()
  })
})
