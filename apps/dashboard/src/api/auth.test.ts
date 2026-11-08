import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const context = vi.hoisted(() => ({ projectEpoch: 0 }))
vi.mock('./env', () => ({ env: { irUrl: 'http://localhost:8090' }, getProjectEpoch: () => context.projectEpoch, setProjectId: () => { context.projectEpoch++ } }))

let auth: typeof import('./auth')
const token = (subject: string) => `header.${btoa(JSON.stringify({ sub: subject, sid: 'session', exp: Math.floor(Date.now() / 1000) + 300 }))}.signature`
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done }); return { promise, resolve } }

beforeEach(async () => {
  vi.resetModules()
  context.projectEpoch = 0
  vi.stubGlobal('BroadcastChannel', undefined)
  vi.stubGlobal('window', { location: { reload: vi.fn() } })
  auth = await import('./auth')
})
afterEach(() => { auth.setAccessToken(null); vi.unstubAllGlobals() })

describe('IR authorization generations', () => {
  it('does not restore access from a refresh that finishes after logout', async () => {
    auth.setAccessToken(token('user'))
    const pending = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValueOnce(new Response('{}')))
    const refresh = auth.refreshAccessToken()
    const rejected = expect(refresh).rejects.toMatchObject({ status: 409 })
    await auth.logout()
    pending.resolve(Response.json({ access_token: token('user') }))
    await rejected
    expect(auth.getAccessToken()).toBeNull()
  })
  it('ignores old-project responses and does not retry them', async () => {
    auth.setAccessToken(token('user'))
    const pending = deferred<Response>()
    const fetchMock = vi.fn().mockReturnValue(pending.promise)
    vi.stubGlobal('fetch', fetchMock)
    const request = auth.authorizedFetch('http://localhost:8090/api/v1/configuration')
    const rejected = expect(request).rejects.toMatchObject({ status: 409 })
    context.projectEpoch++
    pending.resolve(new Response('{}', { status: 401 }))
    await rejected
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
  it('shares concurrent refresh and rejects cross-account replacement', async () => {
    auth.setAccessToken(token('first'))
    const pending = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(pending.promise))
    const first = auth.refreshAccessToken()
    expect(auth.refreshAccessToken()).toBe(first)
    const rejected = expect(first).rejects.toMatchObject({ status: 409 })
    pending.resolve(Response.json({ access_token: token('second') }))
    await rejected
    expect(auth.getAccessToken()).toBeNull()
  })
})
