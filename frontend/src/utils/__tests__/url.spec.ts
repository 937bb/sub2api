import { describe, expect, it } from 'vitest'

import { sanitizeUrl } from '@/utils/url'

describe('sanitizeUrl', () => {
  it('normalizes HTTP(S) URLs and trims surrounding whitespace', () => {
    expect(sanitizeUrl(' https://docs.example.com/guide ')).toBe(
      'https://docs.example.com/guide'
    )
    expect(sanitizeUrl('http://example.com')).toBe('http://example.com/')
  })

  it('rejects executable and protocol-relative URLs', () => {
    expect(sanitizeUrl('javascript:alert(1)')).toBe('')
    expect(sanitizeUrl('data:text/html,<script>alert(1)</script>')).toBe('')
    expect(sanitizeUrl('//attacker.example/path', { allowRelative: true })).toBe('')
    expect(sanitizeUrl('/\\attacker.example/path', { allowRelative: true })).toBe('')
    expect(sanitizeUrl('/path\\to\\logo.png', { allowRelative: true })).toBe('')
  })

  it('only accepts relative and image data URLs when explicitly enabled', () => {
    expect(sanitizeUrl('/logo.png')).toBe('')
    expect(sanitizeUrl('/logo.png', { allowRelative: true })).toBe('/logo.png')
    expect(sanitizeUrl('data:image/png;base64,AAAA')).toBe('')
    expect(sanitizeUrl('data:image/png;base64,AAAA', { allowDataUrl: true })).toBe(
      'data:image/png;base64,AAAA'
    )
    expect(sanitizeUrl('data:image/svg+xml,<svg onload=alert(1)>', { allowDataUrl: true })).toBe('')
    expect(sanitizeUrl('data:image/png,not-base64', { allowDataUrl: true })).toBe('')
  })
})
