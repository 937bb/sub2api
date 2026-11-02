import { sanitizeUrl } from '@/utils/url'

export const DEFAULT_SITE_NAME = 'APIHelm'

export function resolveSiteName(value?: string | null): string {
  const name = value?.trim()
  return !name || /^sub2api$/i.test(name) ? DEFAULT_SITE_NAME : name
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = sanitizedLogoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = sanitizedLogoUrl
}
