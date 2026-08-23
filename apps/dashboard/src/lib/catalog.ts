import type { FilterField, SavedView } from '../types'

export const filterFieldLabels: Record<FilterField, string> = {
  host: 'хост',
  user: 'пользователь',
  process: 'процесс',
  hash: 'хеш',
  ip: 'IP',
  domain: 'домен',
  severity: 'критичность',
  source: 'источник',
  status: 'статус',
}

export const defaultFilterValueOptions: Record<FilterField, string[]> = {
  host: ['ws-alpha.corp.example', 'ws-beta.corp.example'],
  user: [],
  process: [],
  hash: [],
  ip: ['192.0.2.44', '192.0.2.62'],
  domain: ['corp.example'],
  severity: ['critical', 'high', 'medium', 'low', 'info'],
  source: [],
  status: ['new', 'investigating', 'closed'],
}

export const savedViews: SavedView[] = [
  {
    id: 'view-smbexec',
    name: 'impacket_smbexec',
    chips: [],
    timePreset: '30d',
    query: 'impacket_smbexec',
  },
  {
    id: 'view-critical',
    name: 'Только critical/high',
    chips: [
      { id: 'c1', field: 'severity', values: ['critical', 'high'] },
    ],
    timePreset: '30d',
  },
  {
    id: 'view-alpha',
    name: 'Хост ws-alpha',
    chips: [{ id: 'c3', field: 'host', values: ['ws-alpha.corp.example'] }],
    timePreset: '30d',
  },
]

export const issueTemplates = [
  {
    id: 'tpl-enrich',
    title: 'Насыщение контекста',
    description: 'Найти связанные события, сущности и связи по текущему контексту расследования',
  },
  {
    id: 'tpl-hash-hunt',
    title: 'Поиск хеша на хостах',
    description: 'Проверить присутствие IOC-хеша на других хостах проекта',
  },
  {
    id: 'tpl-reputation',
    title: 'Проверка репутации',
    description: 'Проверить IP/домен/хеш во внешних источниках Threat Intel',
  },
  {
    id: 'tpl-parent-chain',
    title: 'Цепочка родительских процессов',
    description: 'Восстановить дерево процессов до точки входа',
  },
]
