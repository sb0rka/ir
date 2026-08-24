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

interface GatewaySource {
  code: string
  kind: string
  capabilities?: string[]
}

async function listGatewaySources(projectId: string): Promise<GatewaySource[]> {
  const response = await authorizedFetch(`${env.gatewayUrl}/api/v1/sources`, {
    headers: { 'X-Project-ID': projectId },
  })
  if (!response.ok) throw await integrationError(response)
  const body = (await response.json()) as { items?: GatewaySource[] }
  return body.items ?? []
}

function pickSiemSourceCode(sources: GatewaySource[]): string {
  const siem = sources.find(
    (item) => item.kind === 'siem' && item.capabilities?.includes('account_userinfo'),
  )
  if (!siem) {
    throw new Error('В проекте нет SIEM-источника с account_userinfo')
  }
  return siem.code
}

export async function probePTUser(projectId: string): Promise<string> {
  const sourceCode = pickSiemSourceCode(await listGatewaySources(projectId))
  const response = await authorizedFetch(
    `${env.gatewayUrl}/api/v1/sources/${encodeURIComponent(sourceCode)}/account/userinfo`,
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
