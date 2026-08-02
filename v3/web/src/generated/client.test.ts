import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GeneratedApiClient, operations } from './client'

describe('OpenAPI generated client', () => {
  beforeEach(() => { vi.restoreAllMocks(); Object.defineProperty(document, 'cookie', { writable: true, value: 'sakura_v3_session_csrf=test-token' }) })
  it('uses generated method and path parameters', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'content-type': 'application/json' } }))
    await new GeneratedApiClient().call('getAccount', { path: { id: 'account/1' } })
    expect(fetchMock).toHaveBeenCalledWith(expect.objectContaining({ pathname: '/api/v3/admin/accounts/account%2F1' }), expect.objectContaining({ method: operations.getAccount.method, credentials: 'include' }))
  })
  it('adds CSRF to generated mutation calls', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
    await new GeneratedApiClient().call('logout')
    const options = fetchMock.mock.calls[0][1] as RequestInit
    expect(new Headers(options.headers).get('X-CSRF-Token')).toBe('test-token')
  })
})
