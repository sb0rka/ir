export function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function formatTimeShort(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function uid(prefix = 'id'): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 9)}`
}

export function clsx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}

export const severityColor: Record<string, string> = {
  critical: 'text-critical',
  high: 'text-high',
  medium: 'text-medium',
  low: 'text-low',
  info: 'text-info',
}

export const severityBg: Record<string, string> = {
  critical: 'bg-critical/20 border-critical/40',
  high: 'bg-high/20 border-high/40',
  medium: 'bg-medium/20 border-medium/40',
  low: 'bg-low/20 border-low/40',
  info: 'bg-info/20 border-info/40',
}

export const severityDot: Record<string, string> = {
  critical: 'bg-critical',
  high: 'bg-high',
  medium: 'bg-medium',
  low: 'bg-low',
  info: 'bg-info',
}

export const statusLabel: Record<string, string> = {
  new: 'новое',
  investigating: 'в расследовании',
  closed: 'закрыто',
  open: 'открыто',
  in_progress: 'в работе',
  running: 'выполняется',
  completed: 'завершена',
  error: 'ошибка',
  cancelled: 'отменена',
  confirmed: 'подтверждено',
  proposed: 'предложено агентом',
  rejected: 'отклонено',
  seed: 'исходный',
  agent: 'агент',
  analyst: 'аналитик',
}

export const kindLabel: Record<string, string> = {
  host: 'хост',
  user: 'пользователь',
  process: 'процесс',
  file: 'файл',
  file_hash: 'файл / хеш',
  ip: 'IP',
  domain: 'домен',
  email: 'email',
  account: 'учётная запись',
  url: 'URL',
  event: 'событие',
  rule: 'правило',
}
