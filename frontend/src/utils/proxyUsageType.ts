import type { Proxy, ProxyUsageType } from '@/types'

export const PROXY_USAGE_TYPES: readonly ProxyUsageType[] = ['general', 'kiro', 'pro']

export function proxyUsageType(proxy: Pick<Proxy, 'usage_type'>): ProxyUsageType {
  return proxy.usage_type || 'general'
}

export function filterProxiesByUsage(proxies: Proxy[], usageType: ProxyUsageType | '', search = ''): Proxy[] {
  const query = search.trim().toLowerCase()
  return proxies.filter(proxy => (!usageType || proxyUsageType(proxy) === usageType)
    && `${proxy.name} ${proxy.host} ${proxy.port}`.toLowerCase().includes(query))
}
