import { describe, expect, it } from 'vitest'
import { formatPaymentAmount } from '../currency'

describe('formatPaymentAmount', () => {
  it('uses the currency default fraction digits', () => {
    expect(formatPaymentAmount(100, 'JPY', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'KRW', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'HKD', 'en-US')).toContain('.00')
  })

  it('keeps expected fraction digits for HKD, USD, and JPY', () => {
    expect(formatPaymentAmount(1234.5, 'HKD', 'en-US')).toBe('$1,234.50')
    expect(formatPaymentAmount(1234.5, 'USD', 'en-US')).toBe('$1,234.50')
    expect(formatPaymentAmount(1234.5, 'JPY', 'en-US')).toBe('¥1,235')
  })
})
