import { authorizedFetch } from './auth'
import { env } from './env'

async function integrationError(response: Response): Promise<Error> {
  const text = await response.text()
  try {
    const parsed = JSON.parse(text) as { error?: { message?: string }; message?: string }
    return new Error(parsed.error?.message || parsed.message || `HTTP ${response.status}`)
  } catch {
    return new Error(text || `HTTP ${response.status}`)
  }
}

export async function probePTUser(projectId: string): Promise<string> {
  const response = await authorizedFetch(
    `${env.gatewayUrl}/api/v1/sources/maxpatrol-siem/account/userinfo`,
    { headers: { 'X-Project-ID': projectId } },
  )
  if (!response.ok) throw await integrationError(response)
  const body = (await response.json()) as { user_name?: unknown }
  if (typeof body.user_name !== 'string' || !body.user_name.trim()) {
    throw new Error('Источник не вернул user_name')
  }
  return body.user_name
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
