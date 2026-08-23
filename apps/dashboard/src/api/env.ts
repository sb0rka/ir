const PROJECT_ID_KEY = 'ir.projectId'

function stripSlash(value: string): string {
  return value.replace(/\/+$/, '')
}

function readStorage(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

export const env = {
  authUrl: stripSlash(import.meta.env.VITE_AUTH_BASE_URL || 'http://localhost:8020'),
  platformUrl: stripSlash(import.meta.env.VITE_PLATFORM_API_BASE_URL || 'http://localhost:8080'),
  irUrl: stripSlash(import.meta.env.VITE_IR_URL || 'http://localhost:8090'),
  gatewayUrl: stripSlash(import.meta.env.VITE_GATEWAY_URL || 'http://localhost:8091'),
}

let activeProjectId = readStorage(PROJECT_ID_KEY)?.trim() || null

export function irBaseUrl(): string {
  return `${env.irUrl}/api/v1`
}

export function getProjectId(): string | null {
  return activeProjectId
}

export function setProjectId(projectId: string | null): void {
  activeProjectId = projectId
  try {
    if (projectId) localStorage.setItem(PROJECT_ID_KEY, projectId)
    else localStorage.removeItem(PROJECT_ID_KEY)
  } catch {
    /* Storage is an optimization; the authenticated shell still owns the live value. */
  }
}

export function timeRangeForPreset(preset: string): { from: string; to: string } {
  const to = new Date()
  const from = new Date(to)
  switch (preset) {
    case '1h':
      from.setHours(from.getHours() - 1)
      break
    case '6h':
      from.setHours(from.getHours() - 6)
      break
    case '24h':
      from.setDate(from.getDate() - 1)
      break
    case '7d':
      from.setDate(from.getDate() - 7)
      break
    case '30d':
      from.setDate(from.getDate() - 30)
      break
    case '90d':
      from.setDate(from.getDate() - 90)
      break
    default:
      from.setDate(from.getDate() - 30)
  }
  return { from: from.toISOString(), to: to.toISOString() }
}
