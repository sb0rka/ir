import type { components } from '@ir/contract/domains/authentication'
import { authorizedFetch, AuthError } from './auth'
import { env } from './env'

export type AgentAccessMetadata = components['schemas']['AgentAccessMetadata']
export type AgentAccessIssued = components['schemas']['AgentAccessIssued']

async function agentAccessRequest<T>(projectId: string, signal: AbortSignal, action?: 'create' | 'rotate', version?: number): Promise<T> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/agent-access${action === 'rotate' ? '/rotate' : ''}`, {
    method: action ? 'POST' : 'GET',
    headers: { 'X-Project-ID': projectId, ...(action === 'rotate' ? { 'Content-Type': 'application/json' } : {}) },
    body: action === 'rotate' ? JSON.stringify({ expected_version: version }) : undefined,
    signal,
    cache: 'no-store',
  })
  if (!response.ok) {
    const message = response.status === 409 ? 'Ключ изменился. Перечитайте состояние перед ротацией.'
      : response.status === 403 ? 'Нужно разрешение на конфигурацию и соответствующие права в проекте.'
      : response.status === 404 ? 'Проект или ключ доступа не найден.'
      : 'Доступ агента недоступен. Повторите запрос.'
    throw new AuthError(response.status, message)
  }
  return response.json() as Promise<T>
}

export const readAgentAccess = (projectId: string, signal: AbortSignal) => agentAccessRequest<AgentAccessMetadata>(projectId, signal)
export const createAgentAccess = (projectId: string, signal: AbortSignal) => agentAccessRequest<AgentAccessIssued>(projectId, signal, 'create')
export const rotateAgentAccess = (projectId: string, version: number, signal: AbortSignal) => agentAccessRequest<AgentAccessIssued>(projectId, signal, 'rotate', version)
