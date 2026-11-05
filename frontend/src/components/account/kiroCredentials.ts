export interface KiroCachePolicy {
  enabled: boolean
  target_percent: number
  ttl_seconds: number
  max_entries: number
}
export const defaultKiroCachePolicy = (): KiroCachePolicy => ({ enabled: false, target_percent: 100, ttl_seconds: 300, max_entries: 128 })
export interface KiroImportAccount { name: string; credentials: Record<string, unknown>; disabled: boolean; proxy_id?: number }
const isObject = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)
export function parseKiroCredentials(text: string): KiroImportAccount[] {
  const root: unknown = JSON.parse(text)
  const rows = isObject(root) && 'accounts' in root ? root.accounts : Array.isArray(root) ? root : [root]
  if (!Array.isArray(rows) || !rows.length || rows.length > 500) throw new Error('Kiro JSON must contain 1–500 accounts')
  return rows.map((row: unknown, index: number) => {
    if (!isObject(row)) throw new Error(`Kiro account ${index + 1}: invalid object`)
    const credentials: Record<string, unknown> = { ...row, ...(isObject(row.credentials) ? row.credentials : {}) }
    delete credentials.credentials
    delete credentials.csrfToken
    delete credentials.proxy_id
    delete credentials.proxyId
    const aliases: Record<string, string> = { accessToken: 'access_token', refreshToken: 'refresh_token', expiresAt: 'expires_at', clientId: 'client_id', clientSecret: 'client_secret', authMethod: 'auth_method', authRegion: 'auth_region', apiRegion: 'api_region', machineId: 'machine_id', profileArn: 'profile_arn', kiroApiKey: 'api_key' }
    for (const [from, to] of Object.entries(aliases)) {
      if (from in credentials) { credentials[to] = credentials[from]; delete credentials[from] }
    }
    if (typeof credentials.expires_at === 'number') {
      const timestamp = credentials.expires_at > 1e11 ? credentials.expires_at : credentials.expires_at * 1000
      credentials.expires_at = new Date(timestamp).toISOString()
    }
    if (!credentials.refresh_token && !credentials.api_key) throw new Error(`Kiro account ${index + 1}: refreshToken or kiroApiKey is required`)
    const proxyID = row.proxy_id ?? row.proxyId
    if (proxyID != null && (!Number.isSafeInteger(proxyID) || Number(proxyID) <= 0)) throw new Error(`Kiro account ${index + 1}: proxy_id must be a positive integer`)
    return { proxy_id: proxyID as number | undefined, name: String(row.email || row.name || `Kiro ${index + 1}`), credentials, disabled: row.disabled === true || row.status === 'disabled' }
  })
}

// Batch imports must specify one distinct, preconfigured proxy per account.
export function assignKiroProxies(items: KiroImportAccount[], selectedProxyID: number | null): (KiroImportAccount & { proxy_id: number })[] {
  const used = new Set<number>()
  return items.map((item, index) => {
    const id = item.proxy_id ?? (items.length === 1 ? selectedProxyID : null)
    if (!id || !Number.isSafeInteger(id) || id <= 0) throw new Error(`Kiro account ${index + 1}: select a static proxy; batch JSON requires proxy_id on every account`)
    if (used.has(id)) throw new Error(`Kiro account ${index + 1}: each account requires a different proxy_id`)
    used.add(id)
    return { ...item, proxy_id: id }
  })
}
