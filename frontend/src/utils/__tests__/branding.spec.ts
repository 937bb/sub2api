import { beforeEach, describe, expect, it } from 'vitest'
import { resolveSiteName, updateFavicon } from '@/utils/branding'

describe('resolveSiteName', () => {
  it.each([undefined, null, '', '  ', 'Sub2API', ' sub2api '])('uses APIHelm for the upstream default %s', (value) => {
    expect(resolveSiteName(value)).toBe('APIHelm')
  })

  it('preserves a configured custom brand', () => {
    expect(resolveSiteName(' Customer Brand ')).toBe('Customer Brand')
  })
})

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
