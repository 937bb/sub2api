import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

describe('PaymentMethodSelector', () => {
  it('uses branded presentation only for built-in alipay and wxpay aliases', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'wxpay_direct',
        methods: [
          { type: 'alipay', fee_rate: 0, available: true },
          { type: 'alipay_direct', fee_rate: 0, available: true },
          { type: 'wxpay', fee_rate: 0, available: true },
          { type: 'wxpay_direct', fee_rate: 0, available: true },
        ],
      },
    })

    const buttons = wrapper.findAll('button')
    expect(buttons.slice(0, 2).every(button => button.get('img').attributes('src').includes('alipay'))).toBe(true)
    expect(buttons.slice(2).every(button => button.get('img').attributes('src').includes('wxpay'))).toBe(true)
    expect(buttons[3].classes()).toContain('border-[#09BB07]')
  })

  it.each(['card_alipay', 'card_wxpay'])('uses neutral presentation for custom method %s', (type) => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: type,
        methods: [{ type, fee_rate: 0, available: true }],
      },
    })

    const button = wrapper.get('button')
    expect(button.classes()).toContain('border-primary-500')
    expect(button.classes()).not.toContain('border-[#02A9F1]')
    expect(button.classes()).not.toContain('border-[#09BB07]')
    expect(button.get('img').attributes('src')).toContain('easypay')
    expect(button.get('img').attributes('alt')).toBe(`payment.methods.${type}`)
  })
})
