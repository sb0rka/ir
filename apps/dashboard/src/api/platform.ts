import { authorizedFetch } from './auth'
import { env } from './env'

export interface Project {
  id: string
  name: string
  description?: string
  is_active: boolean
}

export interface SecretMetadata {
  secret_id: string
  name: string
  payload_kind: string
  current_version_no: number
  created_at: string
  updated_at: string
}

export interface SecretVersion {
  version_no: number
  state: string
  payload_kind: string
  created_at: string
  updated_at: string
}

export class PlatformError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'PlatformError'
    this.status = status
  }
}

async function platformRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await authorizedFetch(`${env.platformUrl}${path}`, init)
  if (!response.ok) {
    const message = (await response.text()) || `Platform API: HTTP ${response.status}`
    throw new PlatformError(response.status, message)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export async function listProjects(): Promise<Project[]> {
  const result = await platformRequest<{ projects?: Project[] }>('/projects')
  return (result.projects ?? []).filter((project) => project.is_active)
}

export async function listSecrets(projectId: string): Promise<SecretMetadata[]> {
  const result = await platformRequest<{ secrets?: SecretMetadata[] }>(
    `/projects/${encodeURIComponent(projectId)}/secrets`,
  )
  return result.secrets ?? []
}

export async function listSecretVersions(
  projectId: string,
  secretId: string,
): Promise<SecretVersion[]> {
  const result = await platformRequest<{ versions?: SecretVersion[] }>(
    `/projects/${encodeURIComponent(projectId)}/resources/${encodeURIComponent(secretId)}/secret/versions`,
  )
  return result.versions ?? []
}

export async function writeSecret(
  projectId: string,
  name: string,
  value: string,
  current?: SecretMetadata,
): Promise<void> {
  const body = JSON.stringify({ secret_value: value, payload_kind: 'text' })
  if (current) {
    await platformRequest(
      `/projects/${encodeURIComponent(projectId)}/resources/${encodeURIComponent(current.secret_id)}/secret/versions`,
      { method: 'POST', headers: { 'Content-Type': 'application/json' }, body },
    )
    return
  }
  await platformRequest(`/projects/${encodeURIComponent(projectId)}/secret`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      description: `IR demo configuration: ${name}`,
      secret_value: value,
      payload_kind: 'text',
    }),
  })
}

export async function currentSecretVersionCreatedAt(
  projectId: string,
  secretName: string,
): Promise<string | null> {
  const secret = (await listSecrets(projectId)).find((item) => item.name === secretName)
  if (!secret) return null
  const versions = await listSecretVersions(projectId, secret.secret_id)
  return (
    versions.find((version) => version.version_no === secret.current_version_no)?.created_at ??
    null
  )
}

export function secretAgeHours(createdAt: string | null, now = Date.now()): number | null {
  if (!createdAt) return null
  const created = Date.parse(createdAt)
  if (!Number.isFinite(created)) return null
  return Math.max(0, Math.floor((now - created) / 3_600_000))
}
