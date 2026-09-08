import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { demoDayInterval } from '../components/time-interval/model'
import type { QueryHistoryEntry } from '../types'

const projectIdRef = { current: 'project-1' as string | null }
const memory = new Map<string, string>()

vi.stubGlobal('localStorage', {
  getItem: (key: string) => memory.get(key) ?? null,
  setItem: (key: string, value: string) => {
    memory.set(key, value)
  },
  removeItem: (key: string) => {
    memory.delete(key)
  },
  clear: () => {
    memory.clear()
  },
  key: (index: number) => [...memory.keys()][index] ?? null,
  get length() {
    return memory.size
  },
})

vi.mock('./env', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./env')>()
  return { ...actual, getProjectId: () => projectIdRef.current }
})

import {
  readEventQueueSnapshot,
  rememberEventQueueSnapshots,
} from './eventQueueSnapshots'

const STORAGE_KEY = 'ir.project-1.eventQueueSnapshots'

const entry = (pdql: string): QueryHistoryEntry => ({
  pdql,
  timeInterval: demoDayInterval(),
  queueSource: 'events',
  groupValues: ['host-a'],
})

const event = { source: 'pt-maxpatrol-siem', sourceEventId: 'evt-1' }

describe('eventQueueSnapshots', () => {
  beforeEach(() => {
    projectIdRef.current = 'project-1'
    memory.clear()
  })

  afterEach(() => {
    memory.clear()
  })

  it('writes and reads a snapshot by investigation + source/sourceEventId', () => {
    rememberEventQueueSnapshots('inv-1', [event], entry('filter(action = "login")'))

    expect(readEventQueueSnapshot('inv-1', event)?.pdql).toBe('filter(action = "login")')
    expect(readEventQueueSnapshot('inv-1', event)?.queueSource).toBe('events')
    expect(readEventQueueSnapshot('inv-1', event)?.groupValues).toEqual(['host-a'])
    expect(readEventQueueSnapshot('inv-2', event)).toBeNull()
    expect(
      readEventQueueSnapshot('inv-1', { source: 'pt-maxpatrol-siem', sourceEventId: 'other' }),
    ).toBeNull()
  })

  it('keeps the first snapshot when the same event is remembered again', () => {
    rememberEventQueueSnapshots('inv-1', [event], entry('first'))
    rememberEventQueueSnapshots('inv-1', [event], entry('second'))

    expect(readEventQueueSnapshot('inv-1', event)?.pdql).toBe('first')
  })

  it('skips events without sourceEventId', () => {
    rememberEventQueueSnapshots(
      'inv-1',
      [{ source: 'pt-maxpatrol-siem' }, undefined, event],
      entry('ok'),
    )

    expect(readEventQueueSnapshot('inv-1', { source: 'pt-maxpatrol-siem' })).toBeNull()
    expect(readEventQueueSnapshot('inv-1', event)?.pdql).toBe('ok')
  })

  it('returns null without a project id or when storage is garbage', () => {
    rememberEventQueueSnapshots('inv-1', [event], entry('ok'))
    projectIdRef.current = null
    expect(readEventQueueSnapshot('inv-1', event)).toBeNull()

    projectIdRef.current = 'project-1'
    memory.set(STORAGE_KEY, '{not-json')
    expect(readEventQueueSnapshot('inv-1', event)).toBeNull()
  })
})
