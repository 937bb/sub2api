import { describe, expect, it } from 'vitest'

import en from '../en'
import zh from '../zh'

type LocaleTree = Record<string, unknown>

function leafKeys(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix]

  return Object.entries(value as LocaleTree).flatMap(([key, child]) =>
    leafKeys(child, prefix ? `${prefix}.${key}` : key)
  )
}

describe('custom module translations', () => {
  it('keeps navigation entries in both locales', () => {
    const keys = ['checkin', 'leaderboard', 'growth', 'timeBilling'] as const

    for (const key of keys) {
      expect(en.nav[key]).toBeTypeOf('string')
      expect(zh.nav[key]).toBeTypeOf('string')
    }
  })

  it.each(['growth', 'timeBilling'] as const)('keeps matching %s keys in English and Chinese', (namespace) => {
    expect(leafKeys(en[namespace]).sort()).toEqual(leafKeys(zh[namespace]).sort())
  })
})
