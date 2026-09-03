import { clearFindingResolveCache } from './findingResolveCache'

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
  const changed = projectId !== activeProjectId
  activeProjectId = projectId
  try {
    if (projectId) localStorage.setItem(PROJECT_ID_KEY, projectId)
    else localStorage.removeItem(PROJECT_ID_KEY)
  } catch {
    /* Storage is an optimization; the authenticated shell still owns the live value. */
  }
  if (changed) clearFindingResolveCache()
}

export const TIME_PRESET_CUSTOM = 'custom'

const TIME_PRESET_LABELS: Record<string, string> = {
  '1h': '1ч',
  '6h': '6ч',
  '24h': '24ч',
  '7d': '7д',
  '30d': '30д',
  '90d': '90д',
}

export function formatDisplayDate(isoDate: string): string {
  const [year, month, day] = isoDate.split('-')
  if (!year || !month || !day) return isoDate
  return `${day}.${month}.${year}`
}

export function timeRangeChipLabel(preset: string, from: string, to: string): string {
  if (preset === TIME_PRESET_CUSTOM && from && to) {
    return `${formatDisplayDate(from)} — ${formatDisplayDate(to)}`
  }
  return TIME_PRESET_LABELS[preset] ?? preset
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

function parseLocalDate(value: string): { year: number; month: number; day: number } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim())
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) return null
  return { year, month, day }
}

function startOfLocalDay(value: string): Date | null {
  const parts = parseLocalDate(value)
  if (!parts) return null
  const date = new Date(parts.year, parts.month - 1, parts.day, 0, 0, 0, 0)
  return Number.isNaN(date.getTime()) ? null : date
}

function endOfLocalDay(value: string): Date | null {
  const parts = parseLocalDate(value)
  if (!parts) return null
  const date = new Date(parts.year, parts.month - 1, parts.day, 23, 59, 59, 999)
  return Number.isNaN(date.getTime()) ? null : date
}

export function resolveTimeRange(
  preset: string,
  customFrom?: string,
  customTo?: string,
): { from: string; to: string } {
  if (preset === TIME_PRESET_CUSTOM) {
    const from = customFrom ? startOfLocalDay(customFrom) : null
    const to = customTo ? endOfLocalDay(customTo) : null
    if (!from || !to) {
      throw new Error('Укажите даты «от» и «до»')
    }
    if (from.getTime() > to.getTime()) {
      throw new Error('Дата «от» не может быть позже даты «до»')
    }
    return { from: from.toISOString(), to: to.toISOString() }
  }
  return timeRangeForPreset(preset)
}
