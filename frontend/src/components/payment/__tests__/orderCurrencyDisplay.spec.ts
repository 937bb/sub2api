import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { PaymentOrder } from '@/types/payment'
import OrderTable from '../OrderTable.vue'
import PaymentQRDialog from '../PaymentQRDialog.vue'
import StripePaymentView from '@/views/user/StripePaymentView.vue'

const pollOrderStatus = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const getOrder = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const toCanvas = vi.hoisted(() => vi.fn())
const confirmPayment = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))
const paymentStore = vi.hoisted(() => ({
  config: { stripe_publishable_key: 'pk_test' } as { stripe_publishable_key?: string },
  fetchConfig: vi.fn(),
  pollOrderStatus,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: 'en-US',
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => paymentStore,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    cancelOrder,
    verifyOrder,
    getOrder,
  },
}))

vi.mock('qrcode', () => ({
  default: {
    toCanvas,
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: vi.fn(),
    resolve: (to: { path: string; query?: Record<string, string | undefined> }) => ({
      href: `${to.path}?${new URLSearchParams(
        Object.entries(to.query || {}).flatMap(([key, value]) => value ? [[key, value]] : []),
      ).toString()}`,
    }),
  }),
}))

vi.mock('@stripe/stripe-js', () => ({
  loadStripe: vi.fn(async () => ({
    elements: () => ({
      create: () => ({
        mount: vi.fn(),
        on: (_event: string, callback: () => void) => callback(),
      }),
    }),
    confirmAlipayPayment: vi.fn(),
    confirmWechatPayPayment: vi.fn(),
    confirmPayment,
  })),
}))

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-pay_amount" :row="row" :value="row.pay_amount" />
      </div>
    </div>
  `,
}

function orderFactory(overrides: Partial<PaymentOrder> = {}): PaymentOrder {
  return {
    id: 42,
    user_id: 9,
    amount: 10,
    pay_amount: 1200,
    currency: 'JPY',
    fee_rate: 0,
    payment_type: 'alipay',
    out_trade_no: 'sub2_202607030001',
    status: 'COMPLETED',
    order_type: 'balance',
    created_at: '2026-07-03T12:00:00Z',
    expires_at: '2099-01-01T12:30:00Z',
    refund_amount: 0,
    ...overrides,
  }
}

describe('order currency display', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    pollOrderStatus.mockReset()
    cancelOrder.mockReset()
    verifyOrder.mockReset()
    getOrder.mockReset()
    showError.mockReset()
    toCanvas.mockReset().mockResolvedValue(undefined)
    confirmPayment.mockReset().mockResolvedValue({})
    paymentStore.config = { stripe_publishable_key: 'pk_test' }
    paymentStore.fetchConfig.mockReset().mockResolvedValue(undefined)
    routeState.query = {}
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('displays order table pay_amount with order currency and keeps balance credited amount in USD', () => {
    const wrapper = mount(OrderTable, {
      props: {
        orders: [orderFactory()],
        loading: false,
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          OrderStatusBadge: true,
        },
      },
    })

    expect(wrapper.text()).toContain('¥1,200')
    expect(wrapper.text()).toContain('payment.orders.creditedAmount: $10.00')
    expect(wrapper.text()).not.toContain('¥10')
    expect(wrapper.text()).not.toContain('¥1,200.00')
  })

  it('keeps subscription order amounts in the payment currency for HKD, USD, and JPY', () => {
    const wrapper = mount(OrderTable, {
      props: {
        orders: [
          orderFactory({ id: 1, order_type: 'subscription', amount: 100, pay_amount: 101, currency: 'HKD' }),
          orderFactory({ id: 2, order_type: 'subscription', amount: 200, pay_amount: 202, currency: 'USD' }),
          orderFactory({ id: 3, order_type: 'subscription', amount: 300, pay_amount: 303, currency: 'JPY' }),
        ],
        loading: false,
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          OrderStatusBadge: true,
        },
      },
    })

    expect(wrapper.text()).toContain('$101.00')
    expect(wrapper.text()).toContain('$100.00')
    expect(wrapper.text()).toContain('$202.00')
    expect(wrapper.text()).toContain('$200.00')
    expect(wrapper.text()).toContain('¥303')
    expect(wrapper.text()).toContain('¥300')
    expect(wrapper.text()).not.toContain('$303.00')
  })

  it('uses order currency in QR dialog success and does not hard-code yen', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory({
      amount: 100,
      pay_amount: 108,
      currency: 'USD',
      order_type: 'subscription',
    }))

    const wrapper = mount(PaymentQRDialog, {
      props: {
        show: false,
        orderId: 42,
        qrCode: '',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
      },
      global: {
        stubs: {
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /><slot name="footer" /></div>',
          },
          Icon: true,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(wrapper.text()).toContain('$100.00')
    expect(wrapper.text()).toContain('$108.00')
    expect(wrapper.text()).not.toContain('¥108')
  })

  it('uses order currency in reachable Stripe route amount display and does not hard-code yen', async () => {
    routeState.query = {
      order_id: '42',
      client_secret: 'pi_secret',
    }
    getOrder.mockResolvedValue({
      data: orderFactory({
        amount: 50,
        pay_amount: 5000,
        currency: 'USD',
        payment_type: 'stripe',
      }),
    })

    const wrapper = mount(StripePaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(getOrder).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('$5,000.00')
    expect(wrapper.text()).not.toContain('¥5,000')
  })
})
