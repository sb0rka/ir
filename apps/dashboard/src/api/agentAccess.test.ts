import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const context = vi.hoisted(() => ({ projectEpoch: 0 }))
vi.mock('./env', () => ({ env: { irUrl: 'http://localhost:8090' }, getProjectEpoch: () => context.projectEpoch, setProjectId: () => { context.projectEpoch++ } }))

let auth: typeof import('./auth')
let agentAccess: typeof import('./agentAccess')
const bearer = `header.${btoa(JSON.stringify({ sub: 'user', sid: 'session', exp: Math.floor(Date.now() / 1000) + 300 }))}.signature`
const issued = { configured: true, version: 2, prefix: 's0_ir_agent_ab', postfix: 'xyz', token: `s0_ir_agent_${'a'.repeat(52)}` }

function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done }); return { promise, resolve } }

beforeEach(async () => {
  vi.resetModules()
  context.projectEpoch = 0
  vi.stubGlobal('BroadcastChannel', undefined)
  vi.stubGlobal('window', { location: { reload: vi.fn() } })
  auth = await import('./auth')
  agentAccess = await import('./agentAccess')
  auth.setAccessToken(bearer)
})
afterEach(() => { auth.setAccessToken(null); vi.unstubAllGlobals() })

describe('project agent access requests', () => {
  it('reads metadata and issues tokens without HTTP caching', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(Response.json({ configured: false })).mockResolvedValueOnce(Response.json(issued, { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    const signal = new AbortController().signal
    await expect(agentAccess.readAgentAccess('project-a', signal)).resolves.toEqual({ configured: false })
    await expect(agentAccess.createAgentAccess('project-a', signal)).resolves.toEqual(issued)
    const read = fetchMock.mock.calls[0][0] as Request
    const create = fetchMock.mock.calls[1][0] as Request
    expect(read.method).toBe('GET')
    expect(create.method).toBe('POST')
    for (const request of [read, create]) {
      expect(request.url).toBe('http://localhost:8090/api/v1/agent-access')
      expect(request.headers.get('X-Project-ID')).toBe('project-a')
      expect(request.headers.get('Authorization')).toBe(`Bearer ${bearer}`)
      expect(request.cache).toBe('no-store')
    }
    expect(await create.text()).toBe('')
  })

  it('rotates only the expected version', async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json(issued))
    vi.stubGlobal('fetch', fetchMock)
    await agentAccess.rotateAgentAccess('project-a', 1, new AbortController().signal)
    const request = fetchMock.mock.calls[0][0] as Request
    expect(request.url).toBe('http://localhost:8090/api/v1/agent-access/rotate')
    expect(request.headers.get('Content-Type')).toBe('application/json')
    expect(await request.json()).toEqual({ expected_version: 1 })
  })

  it('never includes a server error body in the displayed error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ error: { message: issued.token } }, { status: 409 })))
    await expect(agentAccess.rotateAgentAccess('project-a', 1, new AbortController().signal)).rejects.toMatchObject({ status: 409, message: 'Ключ изменился. Перечитайте состояние перед ротацией.' })
  })

  it.each(['project switch', 'logout'])('discards an issuance response after %s', async (change) => {
    const pending = deferred<Response>()
    const fetchMock = vi.fn().mockReturnValue(pending.promise)
    vi.stubGlobal('fetch', fetchMock)
    const response = agentAccess.createAgentAccess('project-a', new AbortController().signal)
    const rejected = expect(response).rejects.toMatchObject({ status: 409 })
    if (change === 'project switch') context.projectEpoch++
    else auth.setAccessToken(null)
    pending.resolve(Response.json(issued, { status: 201 }))
    await rejected
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
