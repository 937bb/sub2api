export const CLIENT_DEVICE_HEADER = 'X-Sub2API-Device-ID'

const STORAGE_KEY = 'sub2api_browser_installation_id'
const COOKIE_NAME = 'sub2api_device_id'
const COOKIE_MAX_AGE_SECONDS = 365 * 24 * 60 * 60

let cachedDeviceID: Promise<string> | null = null

function readDeviceCookie(): string {
  if (typeof document === 'undefined') return ''
  try {
    const prefix = `${COOKIE_NAME}=`
    const value = document.cookie.split(';').map((item) => item.trim()).find((item) => item.startsWith(prefix))
    return value ? decodeURIComponent(value.slice(prefix.length)) : ''
  } catch {
    return ''
  }
}

function writeDeviceCookie(deviceID: string): void {
  if (typeof document === 'undefined' || !deviceID) return
  try {
    const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : ''
    document.cookie = `${COOKIE_NAME}=${encodeURIComponent(deviceID)}; Max-Age=${COOKIE_MAX_AGE_SECONDS}; Path=/; SameSite=Lax${secure}`
  } catch {
    // The request header still carries the signal when cookies are unavailable.
  }
}

function randomInstallationID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `${Date.now()}-${Math.random().toString(36).slice(2)}-${Math.random().toString(36).slice(2)}`
}

function fallbackDigest(source: string): string {
  return Array.from({ length: 8 }, (_, block) => {
    let hash = 2166136261 ^ block
    const input = `${block}:${source}`
    for (let index = 0; index < input.length; index += 1) {
      hash = Math.imul(hash ^ input.charCodeAt(index), 16777619)
    }
    return (hash >>> 0).toString(16).padStart(8, '0')
  }).join('')
}

async function digestDeviceSource(source: string): Promise<string> {
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    try {
      const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(source))
      return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
    } catch {
      // Fall through for browsers that expose Web Crypto but reject the call.
    }
  }
  return fallbackDigest(source)
}

async function buildBrowserDeviceID(): Promise<string> {
  if (typeof navigator === 'undefined') return 'server-rendered-device-identity'

  let installationID = ''
  try {
    installationID = localStorage.getItem(STORAGE_KEY) || ''
    if (!installationID) {
      installationID = randomInstallationID()
      localStorage.setItem(STORAGE_KEY, installationID)
    }
  } catch {
    const cookieDeviceID = readDeviceCookie()
    if (cookieDeviceID.length >= 16) return cookieDeviceID
    installationID = randomInstallationID()
  }

  const extendedNavigator = navigator as Navigator & { deviceMemory?: number }
  const source = [
    installationID,
    navigator.userAgent,
    navigator.platform,
    navigator.language,
    navigator.languages?.join(',') || '',
    String(navigator.hardwareConcurrency || 0),
    String(extendedNavigator.deviceMemory || 0),
    typeof screen === 'undefined' ? '' : `${screen.width}x${screen.height}x${screen.colorDepth}`,
    String(navigator.maxTouchPoints || 0),
    (() => {
      try { return Intl.DateTimeFormat().resolvedOptions().timeZone }
      catch { return '' }
    })(),
  ].join('|')
  const deviceID = await digestDeviceSource(source)
  writeDeviceCookie(deviceID)
  return deviceID
}

export function getBrowserDeviceID(): Promise<string> {
  if (!cachedDeviceID) {
    cachedDeviceID = buildBrowserDeviceID().catch(() => {
      const fallback = fallbackDigest(`fallback:${Date.now()}:${randomInstallationID()}`)
      writeDeviceCookie(fallback)
      return fallback
    })
  }
  return cachedDeviceID
}

export function primeBrowserDeviceIdentity(): void {
  void getBrowserDeviceID()
}
