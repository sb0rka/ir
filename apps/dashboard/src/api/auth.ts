import { env } from './env'

const ACCESS_TOKEN_KEY = 'ir.accessToken'
const REFRESH_EARLY_MS = 30_000
const REFRESH_RETRY_MS = 60_000

export interface AuthSubject {
  subject_id: string
  kind: string
  is_active: boolean
  user?: {
    user_id: string
    username: string
    email: string
    phone?: string
  }
}

export class AuthError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'AuthError'
    this.status = status
  }
}

function readStoredToken(): string | null {
  try {
    return localStorage.getItem(ACCESS_TOKEN_KEY)
  } catch {
    return null
  }
}

let accessToken = readStoredToken()
let refreshPromise: Promise<string> | null = null
let refreshTimer: ReturnType<typeof setTimeout> | null = null
let bootstrapPromise: Promise<AuthSubject | null> | null = null
const listeners = new Set<() => void>()

function decodeExpiry(token: string): number | null {
  try {
    const encoded = token.split('.')[1]
    if (!encoded) return null
    const padded = encoded.replace(/-/g, '+').replace(/_/g, '/').padEnd(
      Math.ceil(encoded.length / 4) * 4,
      '=',
    )
    const payload = JSON.parse(atob(padded)) as { exp?: unknown }
    return typeof payload.exp === 'number' ? payload.exp * 1000 : null
  } catch {
    return null
  }
}

function notify(): void {
  for (const listener of listeners) listener()
}

function scheduleRefresh(token: string): void {
  if (refreshTimer) clearTimeout(refreshTimer)
  const expiry = decodeExpiry(token)
  if (!expiry) return
  const delay = Math.max(1_000, expiry - Date.now() - REFRESH_EARLY_MS)
  refreshTimer = setTimeout(() => {
    void refreshAccessToken().catch((error: unknown) => {
      if (error instanceof AuthError && error.status === 401) return
      if (!accessToken) return
      refreshTimer = setTimeout(() => {
        if (accessToken) scheduleRefresh(accessToken)
      }, REFRESH_RETRY_MS)
    })
  }, delay)
}

export function getAccessToken(): string | null {
  return accessToken
}

export function subscribeAuth(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function setAccessToken(token: string | null): void {
  accessToken = token
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
  try {
    if (token) localStorage.setItem(ACCESS_TOKEN_KEY, token)
    else localStorage.removeItem(ACCESS_TOKEN_KEY)
  } catch {
    /* The in-memory token remains authoritative for this page. */
  }
  if (token) scheduleRefresh(token)
  notify()
}

async function responseMessage(response: Response): Promise<string> {
  const text = await response.text()
  if (!text) return `HTTP ${response.status}`
  try {
    const parsed = JSON.parse(text) as { error?: { message?: string }; message?: string }
    return parsed.error?.message || parsed.message || text
  } catch {
    return text
  }
}

async function readAccessToken(response: Response): Promise<string> {
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  const body = (await response.json()) as { access_token?: unknown }
  if (typeof body.access_token !== 'string' || !body.access_token) {
    throw new AuthError(response.status, 'Auth не вернул access token')
  }
  return body.access_token
}

export function refreshAccessToken(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = fetch(`${env.authUrl}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    })
      .then(readAccessToken)
      .then((token) => {
        setAccessToken(token)
        return token
      })
      .catch((error: unknown) => {
        if (error instanceof AuthError && error.status === 401) setAccessToken(null)
        throw error
      })
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

function withToken(request: Request, token: string | null): Request {
  if (!token) return request
  const headers = new Headers(request.headers)
  headers.set('Authorization', `Bearer ${token}`)
  return new Request(request, { headers })
}

/** Adds the current JWT, refreshes once on 401, and never clears state for a vendor 401. */
export const authorizedFetch: typeof fetch = async (input, init) => {
  const original = new Request(input, init)
  const first = original.clone()
  const retry = original.clone()
  let response = await fetch(withToken(first, accessToken))
  if (response.status !== 401 || original.url.endsWith('/auth/refresh')) return response

  try {
    const token = await refreshAccessToken()
    response = await fetch(withToken(retry, token))
  } catch (error) {
    if (error instanceof AuthError && error.status === 401) {
      throw new AuthError(401, 'Сессия истекла')
    }
    throw error
  }
  return response
}

async function getSubject(): Promise<AuthSubject> {
  const response = await authorizedFetch(`${env.authUrl}/auth/subject`)
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  return response.json() as Promise<AuthSubject>
}

export async function login(loginValue: string, password: string): Promise<AuthSubject> {
  const isEmail = loginValue.includes('@')
  const response = await fetch(`${env.authUrl}/auth/login`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      password,
      ...(isEmail ? { email: loginValue } : { username: loginValue }),
    }),
  })
  const token = await readAccessToken(response)
  setAccessToken(token)
  try {
    return await getSubject()
  } catch (error) {
    setAccessToken(null)
    throw error
  }
}

export function bootstrapAuth(): Promise<AuthSubject | null> {
  if (!bootstrapPromise) {
    bootstrapPromise = (async () => {
      try {
        if (!accessToken) await refreshAccessToken()
        return await getSubject()
      } catch (error) {
        if (error instanceof AuthError && error.status === 401) {
          setAccessToken(null)
          return null
        }
        throw error
      }
    })().finally(() => {
      bootstrapPromise = null
    })
  }
  return bootstrapPromise
}

export async function logout(): Promise<void> {
  try {
    await fetch(`${env.authUrl}/auth/logout`, {
      method: 'POST',
      credentials: 'include',
      headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
    })
  } finally {
    setAccessToken(null)
  }
}

if (accessToken) scheduleRefresh(accessToken)
