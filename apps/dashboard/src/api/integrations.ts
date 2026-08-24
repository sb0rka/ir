import { authorizedFetch } from './auth'
import { gatewayClient } from './clients'
import { env } from './env'
import { unwrapError } from './error'

async function integrationError(response: Response): Promise<Error> {
  const text = await response.text()
  try {
    const parsed = JSON.parse(text) as { error?: { message?: string }; message?: string }
    return new Error(parsed.error?.message || parsed.message || `HTTP ${response.status}`)
  } catch {
    return new Error(text || `HTTP ${response.status}`)
  }
}

export interface ProjectSource {
  code: string
  name: string
  kind: string
  status: 'online' | 'degraded' | 'offline'
  capabilities?: string[]
}

export async function listProjectSources(projectId: string): Promise<ProjectSource[]> {
  const { data, error, response } = await gatewayClient.GET('/api/v1/sources', {
    params: { header: { 'X-Project-ID': projectId } },
  })
  if (error || !data) throw unwrapError(error, response.status)
  return (data.items ?? []) as ProjectSource[]
}

export async function probeSourceUserinfo(projectId: string, sourceCode: string): Promise<string> {
  const { data, error, response } = await gatewayClient.GET(
    '/api/v1/sources/{source}/account/userinfo',
    {
      params: {
        header: { 'X-Project-ID': projectId },
        path: { source: sourceCode },
      },
    },
  )
  if (error || !data) throw unwrapError(error, response.status)
  if (typeof data.user_name !== 'string' || !data.user_name.trim()) {
    throw new Error('Источник не вернул user_name')
  }
  return data.user_name
}

export interface SomWorkspaceOption {
  id: string
  name: string
  slug: string
}

export interface SomBoardOption {
  id: string
  workspace_id: string
  name: string
}

export async function listSomWorkspaces(projectId: string): Promise<SomWorkspaceOption[]> {
  const response = await authorizedFetch(`${env.irUrl}/api/v1/som/workspaces`, {
    headers: { 'X-Project-ID': projectId },
  })
  if (!response.ok) throw await integrationError(response)
  return response.json() as Promise<SomWorkspaceOption[]>
}

export async function listSomBoards(
  projectId: string,
  workspaceId: string,
): Promise<SomBoardOption[]> {
  const response = await authorizedFetch(
    `${env.irUrl}/api/v1/som/workspaces/${encodeURIComponent(workspaceId)}/boards`,
    { headers: { 'X-Project-ID': projectId } },
  )
  if (!response.ok) throw await integrationError(response)
  return response.json() as Promise<SomBoardOption[]>
}
