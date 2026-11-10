import { env, getProjectEpoch, setProjectId } from './env'
import type { components as AuthComponents } from '@ir/contract/domains/authentication'

const REFRESH_EARLY_MS = 30_000
const HANDOFF_VERIFIER_KEY = 'ir.oidcVerifier'

export type AuthSubject = AuthComponents['schemas']['AuthSubject']

export class AuthError extends Error {
  readonly status: number
  constructor(status: number, message: string) { super(message); this.name = 'AuthError'; this.status = status }
}

// Older dashboard tokens addressed Platform API and cannot be reused by IR.
try { localStorage.removeItem('ir.accessToken') } catch { /* Storage can be disabled. */ }
let authNotice: string | null = null
export function getAuthNotice(): string | null { return authNotice }
export function clearAuthNotice(): void { authNotice = null }

let accessToken: string | null = null
let authEpoch = 0
let signingOut = false
export function getAuthEpoch(): number { return authEpoch }
export function isSigningOut(): boolean { return signingOut }
let refreshPromise: Promise<string> | null = null
let refreshTimer: ReturnType<typeof setTimeout> | null = null
let bootstrapPromise: Promise<AuthSubject | null> | null = null
const listeners = new Set<() => void>()
const authChannel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('sb0rka-ir-auth')
if (authChannel) authChannel.onmessage = () => { setAccessToken(null); window.location.reload() }

function principalKey(token: string): string | null {
  try {
    const raw = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const value = JSON.parse(atob(raw.padEnd(Math.ceil(raw.length / 4) * 4, '='))) as { sub?: unknown; sid?: unknown }
    return typeof value.sub === 'string' && typeof value.sid === 'string' ? `${value.sub}:${value.sid}` : null
  } catch { return null }
}

function decodeExpiry(token: string): number | null {
  try {
    const encoded = token.split('.')[1]
    const normalized = encoded.replace(/-/g, '+').replace(/_/g, '/')
    const payload = JSON.parse(atob(normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '='))) as { exp?: unknown }
    return typeof payload.exp === 'number' ? payload.exp * 1000 : null
  } catch { return null }
}

export function getAccessToken(): string | null { return accessToken }
export function subscribeAuth(listener: () => void): () => void { listeners.add(listener); return () => listeners.delete(listener) }

export function setAccessToken(token: string | null): void {
  if (token === null) { authEpoch++; refreshPromise = null }
  accessToken = token
  if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null }
  if (token) {
    const expiry = decodeExpiry(token)
    if (expiry) refreshTimer = setTimeout(() => { void refreshAccessToken().catch(() => undefined) }, Math.max(1000, expiry - Date.now() - REFRESH_EARLY_MS))
  }
  for (const listener of listeners) listener()
}

async function responseMessage(response: Response): Promise<string> {
  try { const body = await response.json() as { error?: { message?: string } }; return body.error?.message || `HTTP ${response.status}` }
  catch { return `HTTP ${response.status}` }
}

async function readAccessToken(response: Response, epoch: number): Promise<string> {
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  const body = await response.json() as { access_token?: unknown }
  if (epoch !== authEpoch || signingOut) throw new AuthError(409, 'Сессия браузера изменилась')
  if (typeof body.access_token !== 'string' || !body.access_token) throw new AuthError(503, 'IR не вернул access token')
  const nextPrincipal = principalKey(body.access_token)
  if (!nextPrincipal) throw new AuthError(503, 'IR вернул некорректное удостоверение')
  if (accessToken && principalKey(accessToken) !== nextPrincipal) {
    // A login in another tab may replace the shared cookie. Never combine the
    // previous user's in-memory workspace with the new session's bearer.
    setProjectId(null)
    setAccessToken(null)
    window.location.reload()
    throw new AuthError(409, 'Сессия браузера изменилась')
  }
  setAccessToken(body.access_token)
  return body.access_token
}

export function refreshAccessToken(): Promise<string> {
  if (signingOut) return Promise.reject(new AuthError(401, 'Выход из сессии'))
  if (!refreshPromise) {
    const epoch = authEpoch
    const pending: Promise<string> = fetch(`${env.irUrl}/api/v1/auth/refresh`, { method: 'POST', credentials: 'include' })
      .then((response) => readAccessToken(response, epoch))
      .catch((error: unknown) => { if (epoch === authEpoch && error instanceof AuthError && error.status === 401) setAccessToken(null); throw error })
      .finally(() => { if (refreshPromise === pending) refreshPromise = null })
    refreshPromise = pending
  }
  return refreshPromise
}

function withToken(request: Request, token: string | null): Request {
  const headers = new Headers(request.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  return new Request(request, { headers })
}

/** Only the IR backend receives the browser's access token. */
export const authorizedFetch: typeof fetch = async (input, init) => {
  const epoch = authEpoch
  const projectEpoch = getProjectEpoch()
  const assertCurrent = () => { if (epoch !== authEpoch || projectEpoch !== getProjectEpoch() || signingOut) throw new AuthError(409, 'Контекст IR изменился') }
  assertCurrent()
  const original = new Request(input, init)
  const target = new URL(original.url)
  if (target.origin !== new URL(env.irUrl).origin || !target.pathname.startsWith('/api/v1/')) throw new AuthError(403, 'Недопустимый адрес запроса IR')
  const retry = original.clone()
  let response = await fetch(withToken(original, accessToken))
  assertCurrent()
  if (response.status !== 401 || target.pathname.endsWith('/auth/refresh')) return response
  const token = await refreshAccessToken()
  assertCurrent()
  response = await fetch(withToken(retry, token))
  assertCurrent()
  return response
}

function base64url(value: Uint8Array): string { return btoa(String.fromCharCode(...value)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '') }

export async function login(): Promise<void> {
  setAccessToken(null)
  setProjectId(null)
  const epoch = authEpoch
  const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)))
  const challenge = base64url(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))))
  const response = await fetch(`${env.irUrl}/api/v1/auth/oidc/init`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ app_challenge: challenge }) })
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  const body = await response.json() as { authorize_url: string }
  if (epoch !== authEpoch) throw new AuthError(409, 'Запрос входа устарел')
  sessionStorage.setItem(HANDOFF_VERIFIER_KEY, verifier)
  window.location.assign(body.authorize_url)
}

async function getSubject(): Promise<AuthSubject> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/auth/me`)
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  return response.json() as Promise<AuthSubject>
}

export function bootstrapAuth(): Promise<AuthSubject | null> {
  if (!bootstrapPromise) {
    bootstrapPromise = (async () => {
      const epoch = authEpoch
      const url = new URL(window.location.href)
      const handoff = url.searchParams.get('auth_handoff')
      const loginError = url.searchParams.has('auth_error') || url.searchParams.has('integration_error')
      if (handoff || loginError || url.searchParams.has('integration')) {
        for (const key of ['auth_handoff', 'auth_error', 'integration_error', 'integration']) url.searchParams.delete(key)
        window.history.replaceState(null, '', url.pathname + url.search + url.hash)
      }
      if (loginError) authNotice = 'Вход или подтверждение доступа не завершены. Повторите действие.'
      try {
        if (handoff) {
          const verifier = sessionStorage.getItem(HANDOFF_VERIFIER_KEY)
          sessionStorage.removeItem(HANDOFF_VERIFIER_KEY)
          if (!verifier) throw new AuthError(401, 'Запрос входа не найден в этом браузере')
          await readAccessToken(await fetch(`${env.irUrl}/api/v1/auth/oidc/redeem`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ handoff, app_verifier: verifier }) }), epoch)
          authChannel?.postMessage('login')
        } else if (!accessToken) await refreshAccessToken()
        return await getSubject()
      } catch (error) {
        if (error instanceof AuthError && error.status === 401) { setAccessToken(null); return null }
        throw error
      }
    })().finally(() => { bootstrapPromise = null })
  }
  return bootstrapPromise
}

export async function logout(): Promise<void> {
  signingOut = true
  setAccessToken(null)
  setProjectId(null)
  try {
    const response = await fetch(`${env.irUrl}/api/v1/auth/logout`, { method: 'POST', credentials: 'include' })
    if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
    authChannel?.postMessage('logout')
  } finally { signingOut = false }
}

export type IntegrationStatus = AuthComponents['schemas']['IntegrationStatus']
export type IntegrationKind = 'platform' | 'gateway'

export async function integrationStatus(kind: IntegrationKind): Promise<IntegrationStatus> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/integrations/${kind}/status`)
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  return response.json() as Promise<IntegrationStatus>
}

export async function connectIntegration(kind: IntegrationKind): Promise<void> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/integrations/${kind}/connect`, { method: 'POST' })
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
  const body = await response.json() as { authorize_url: string }
  window.location.assign(body.authorize_url)
}

export async function disconnectIntegration(kind: IntegrationKind): Promise<void> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/integrations/${kind}/disconnect`, { method: 'POST' })
  if (!response.ok) throw new AuthError(response.status, await responseMessage(response))
}
