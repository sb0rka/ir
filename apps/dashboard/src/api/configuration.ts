import type { components } from '@ir/contract/domains/authentication'
import { authorizedFetch, AuthError } from './auth'
import { env } from './env'

export type ConfigurationDocument = components['schemas']['ConfigurationDocument']
export type ConfigurationMetadata = components['schemas']['ConfigurationMetadata']

async function configurationRequest<T>(projectId: string, signal: AbortSignal, update?: string): Promise<T> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/configuration`, {
    method: update !== undefined ? 'PUT' : 'GET',
    headers: { 'X-Project-ID': projectId, ...(update !== undefined ? { 'Content-Type': 'application/json' } : {}) },
    body: update,
    signal,
    cache: 'no-store',
  })
  if (!response.ok) {
    const message = response.status === 409 ? 'Конфигурация изменилась. Перечитайте её перед сохранением.'
      : response.status === 403 ? 'Нужно разрешение на конфигурацию и соответствующие права в проекте.'
      : response.status === 400 ? 'Некорректная JSON-конфигурация или превышен лимит 64 КиБ.'
      : 'Конфигурация недоступна. Повторите запрос.'
    throw new AuthError(response.status, message)
  }
  return response.json() as Promise<T>
}

export const readConfiguration = (projectId: string, signal: AbortSignal) => configurationRequest<ConfigurationDocument>(projectId, signal)
export const saveConfiguration = (projectId: string, value: string, signal: AbortSignal) => configurationRequest<ConfigurationMetadata>(projectId, signal, value)
