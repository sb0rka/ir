const SOM_TOKEN_KEY = 'ir.somToken'

function stripSlash(value: string): string {
  return value.replace(/\/+$/, '')
}

export const env = {
  irUrl: stripSlash(import.meta.env.VITE_IR_URL || 'http://localhost:8090'),
  gatewayUrl: stripSlash(import.meta.env.VITE_GATEWAY_URL || 'http://localhost:8091'),
  projectId: import.meta.env.VITE_PROJECT_ID || 'abcdef1234',
  token: import.meta.env.VITE_TOKEN || '',
  somWorkspaceSelector: import.meta.env.VITE_SOM_WORKSPACE_SELECTOR || 'IR Workspace',
  somBoardSelector: import.meta.env.VITE_SOM_BOARD_SELECTOR || 'Playbook board',
  somVariant: import.meta.env.VITE_SOM_VARIANT || 'DEFAULT',
  somModelId: import.meta.env.VITE_SOM_MODEL_ID || 'openrouter/deepseek/deepseek-v4-flash',
}

export function irBaseUrl(): string {
  return `${env.irUrl}/api/v1`
}

export function getSomToken(): string | null {
  try {
    const stored = localStorage.getItem(SOM_TOKEN_KEY)
    if (stored) return stored
  } catch {
    /* ignore */
  }
  const seeded = import.meta.env.VITE_SOM_TOKEN
  return seeded ? seeded : null
}

export function setSomToken(token: string | null): void {
  try {
    if (!token) localStorage.removeItem(SOM_TOKEN_KEY)
    else localStorage.setItem(SOM_TOKEN_KEY, token)
  } catch {
    /* ignore */
  }
}

/** SOM token is preferred: ir-api forwards it to /som/*, and AUTH_DISABLED accepts it on the rest. */
export function getIrToken(): string | null {
  return getSomToken() || env.token || null
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
    default:
      from.setDate(from.getDate() - 30)
  }
  return { from: from.toISOString(), to: to.toISOString() }
}
