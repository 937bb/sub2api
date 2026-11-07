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

describe('Kiro nullable credential exports', () => {
  it('accepts Google exports without regenerating their identity', () => {
    const [item] = parseKiroCredentials(JSON.stringify({ ...account(4), credentials: undefined, refreshToken: 'test-only', provider: 'Google', authMethod: 'social', idToken: null, expiresAt: null, region: null, clientId: null, clientSecret: null }))
    expect(item?.credentials.refresh_token).toBe('test-only')
    expect(item?.credentials.machine_id).toBe('a'.repeat(64))
    for (const key of ['id_token', 'expires_at', 'region', 'client_id', 'client_secret']) expect(item?.credentials).not.toHaveProperty(key)
  })
  it.each([
    { refreshToken: null, refresh_token: 'test-only' },
    { refreshToken: 'test-only', refresh_token: null },
    { refreshToken: 'test-only', credentials: { refreshToken: null } }
  ])('treats null aliases and nested fields as absent', row => {
    expect(parseKiroCredentials(JSON.stringify(row))[0]?.credentials.refresh_token).toBe('test-only')
  })
  it('rejects conflicting aliases without revealing either token', () => {
    expect(() => parseKiroCredentials(JSON.stringify({refreshToken:'new-secret',refresh_token:'old-secret'}))).toThrow('conflicting Kiro field refresh_token')
  })
  it('still requires an authentication credential', () => {
    expect(() => parseKiroCredentials('{"refreshToken":null}')).toThrow('required')
  })
})

describe('Kiro API key import', () => {
  it('accepts a bare API key without requiring a refresh token', () => {
    const [item] = parseKiroCredentials('  ksk_test_only_123\n')
    expect(item?.credentials).toEqual({api_key: 'ksk_test_only_123', auth_method: 'api_key'})
    expect(assignKiroProxies([item!], 8)[0]?.proxy_id).toBe(8)
  })
  it('preserves an explicitly selected CLI endpoint from kiro.rs', () => {
    expect(parseKiroCredentials('{"kiroApiKey":"ksk_test_only","authMethod":"api_key","endpoint":"cli"}')[0]?.credentials).toEqual({api_key:'ksk_test_only',auth_method:'api_key',endpoint:'cli'})
  })
  it('does not silently discard malformed nested credentials', () => {
    expect(() => parseKiroCredentials('{"kiroApiKey":"ksk_test_only","credentials":42}')).toThrow('credentials must be an object')
  })
})
