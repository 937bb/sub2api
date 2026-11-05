import { describe, expect, it } from 'vitest'
import { assignKiroProxies, parseKiroCredentials } from '../kiroCredentials'

const account = (proxy_id?: number) => ({ email: 'test@example.invalid', proxy_id, machineId: 'a'.repeat(64), credentials: { refreshToken: 'test-only', expiresAt: 1793870695992 } })
describe('Kiro static proxy import', () => {
  it('preserves account identity and separates proxy binding from credentials', () => {
    const [item] = parseKiroCredentials(JSON.stringify({ accounts: [account(4)] }))
    expect(item?.proxy_id).toBe(4)
    expect(item?.credentials.machine_id).toBe('a'.repeat(64))
    expect(item?.credentials.refresh_token).toBe('test-only')
    expect(item?.credentials).not.toHaveProperty('proxy_id')
    expect(item?.credentials).not.toHaveProperty('credentials')
  })
  it('uses the selector for a single imported account', () => {
    expect(assignKiroProxies(parseKiroCredentials(JSON.stringify(account())), 8)[0]?.proxy_id).toBe(8)
  })
  it('never reuses the selector for an entire batch', () => {
    expect(() => assignKiroProxies(parseKiroCredentials(JSON.stringify([account(),account()])), 8)).toThrow('proxy_id')
  })
  it('rejects duplicate bindings before creating any accounts', () => {
    expect(() => assignKiroProxies(parseKiroCredentials(JSON.stringify([account(4),account(4)])), null)).toThrow('different')
  })
  it('preserves distinct bindings across batch parsing', () => {
    expect(assignKiroProxies(parseKiroCredentials(JSON.stringify([account(4),account(5)])), 8).map(a => a.proxy_id)).toEqual([4,5])
  })
  it.each([0,-1,1.5,'4'])('rejects invalid proxy IDs %s', id => {
    expect(() => parseKiroCredentials(JSON.stringify({ ...account(), proxy_id:id }))).toThrow('positive integer')
  })
})
