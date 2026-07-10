import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

describe('login agreement locales', () => {
  const enPrompt = en.legal.loginAgreementPrompt
  const zhPrompt = zh.legal.loginAgreementPrompt

  it('keeps locale keys and interpolation placeholders in parity', () => {
    expect(Object.keys(enPrompt).sort()).toEqual(Object.keys(zhPrompt).sort())
    expect(enPrompt.dialogDescription.match(/\{[^}]+\}/g)).toEqual(['{date}'])
    expect(zhPrompt.dialogDescription.match(/\{[^}]+\}/g)).toEqual(['{date}'])
  })

  it('does not leak CJK copy into English', () => {
    expect(Object.values(enPrompt).join('')).not.toMatch(/[\u3000-\u303f\u3400-\u9fff]/u)
  })
})
