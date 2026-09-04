import { beforeEach, describe, expect, it, vi } from 'vitest'
import { parse } from '../lib/pdql/parse'
import { findingUuidQuery } from '../lib/pdql'
import { demoDayInterval } from '../components/time-interval/model'
import type { components as Gw } from '@ir/contract/gateway'
import { clearFindingResolveCache, resolveFindingEvents, searchQueue } from './search'
import type { FindingResolveKey } from '../lib/correlationSubevents'

const { gatewayGet, gatewayPost, projectIdRef } = vi.hoisted(() => ({
  gatewayGet: vi.fn(),
  gatewayPost: vi.fn(),
  projectIdRef: { current: 'project-1' as string | null },
}))

vi.mock('./clients', () => ({
  gatewayClient: {
    GET: gatewayGet,
    POST: gatewayPost,
  },
}))

vi.mock('./env', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./env')>()
  return { ...actual, getProjectId: () => projectIdRef.current }
})

type GwEvent = Gw['schemas']['Event']
type GwFinding = Gw['schemas']['Finding']

function mustParse(text: string) {
  const result = parse(text)
  if (!result.ok) throw new Error(result.error.message)
  return result.ast
}

function gwEvent(
  id: string,
  attrs: Record<string, unknown> = {},
  type = 'event',
  occurredAt = '2025-10-23T12:00:00.000Z',
): GwEvent {
  return {
    source_code: 'pt-maxpatrol-siem',
    source_event_id: id,
    type,
    title: id,
    severity: 'high',
    occurred_at: occurredAt,
    entities: [],
    attributes: attrs,
    fetched_at: '2025-10-23T12:00:00.000Z',
  }
}

function gwFinding(
  id: string,
  occurredAt: string,
  kind: GwFinding['kind'] = 'siem_incident',
): GwFinding {
  return {
    ref: {
      source_code: 'pt-maxpatrol-siem',
      record_type: kind,
      external_id: id,
      time_range: { from: '2025-10-23T00:00:00.000Z', to: '2025-10-23T23:59:59.000Z' },
    },
    kind,
    title: id,
    severity: 'high',
    occurred_at: occurredAt,
    entities: [],
    fetched_at: '2025-10-23T12:00:00.000Z',
  }
}

function emptyResolve(events: GwEvent[] = []) {
  return {
    data: {
      findings: [],
      sessions: [],
      events,
      entities: [],
      relations: [],
      resolutions: [],
      source_errors: [],
    },
    error: undefined,
    response: { status: 200 },
  }
}

beforeEach(() => {
  projectIdRef.current = 'project-1'
  clearFindingResolveCache()
  gatewayGet.mockReset()
  gatewayPost.mockReset()
  gatewayGet.mockResolvedValue({
    data: {
      items: [
        {
          code: 'pt-maxpatrol-siem',
          name: 'MaxPatrol SIEM',
          kind: 'siem',
          mode: 'proxy',
          status: 'online',
          capabilities: ['events', 'findings'],
        },
      ],
    },
    error: undefined,
    response: { status: 200 },
  })
})

describe('searchQueue uuid resolve', () => {
  it('resolves child events instead of searching when the query is a finding uuid', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/context/resolve') {
        return emptyResolve([
          gwEvent('corr-1'),
          gwEvent('evt-2', { parent_source_event_id: 'corr-1', relation_type: 'subevent_of' }),
        ])
      }
      throw new Error(`unexpected ${path}`)
    })

    const result = await searchQueue(
      mustParse(findingUuidQuery('corr-1', 'siem_correlation')),
      demoDayInterval('UTC'),
      'events',
    )

    expect(gatewayPost).toHaveBeenCalledWith(
      '/api/v1/context/resolve',
      expect.objectContaining({
        body: expect.objectContaining({
          findings: [
            expect.objectContaining({
              source_code: 'pt-maxpatrol-siem',
              record_type: 'siem_correlation',
              external_id: 'corr-1',
            }),
          ],
          events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'corr-1' }],
        }),
      }),
    )
    expect(result.queueOrder.map((item) => item.id)).toEqual(['pt-maxpatrol-siem/evt-2'])
  })

  it('falls back to events search when resolve returns no children', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/context/resolve') return emptyResolve([])
      if (path === '/api/v1/events/search') {
        return {
          data: {
            events: [gwEvent('plain-1')],
            entities: [],
            relations: [],
            source_states: [],
            source_errors: [],
          },
          error: undefined,
          response: { status: 200 },
        }
      }
      throw new Error(`unexpected ${path}`)
    })

    const result = await searchQueue(
      mustParse(findingUuidQuery('plain-1', 'siem_incident')),
      demoDayInterval('UTC'),
      'events',
    )

    expect(gatewayPost.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/context/resolve',
      '/api/v1/events/search',
    ])
    expect(result.queueOrder.map((item) => item.id)).toEqual(['pt-maxpatrol-siem/plain-1'])
  })

  it('resolves a finding chip even when extra filters are present', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/context/resolve') {
        return emptyResolve([
          gwEvent('inc-1'),
          gwEvent('evt-9', { parent_finding_id: 'inc-1' }),
        ])
      }
      throw new Error(`unexpected ${path}`)
    })

    const result = await searchQueue(
      mustParse('filter(siem_incident = "inc-1" and action = "login") | select(time) | sort(time desc)'),
      demoDayInterval('UTC'),
      'events',
    )

    expect(gatewayPost).toHaveBeenCalledWith(
      '/api/v1/context/resolve',
      expect.objectContaining({
        body: expect.objectContaining({
          findings: [expect.objectContaining({ record_type: 'siem_incident', external_id: 'inc-1' })],
        }),
      }),
    )
    expect(result.queueOrder.map((item) => item.id)).toEqual(['pt-maxpatrol-siem/evt-9'])
  })

  it('keeps only resolved children inside the time interval', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/context/resolve') {
        return emptyResolve([
          gwEvent('corr-1'),
          gwEvent('evt-in', { parent_source_event_id: 'corr-1', relation_type: 'subevent_of' }),
          gwEvent(
            'evt-out',
            { parent_source_event_id: 'corr-1', relation_type: 'subevent_of' },
            'event',
            '2025-10-22T10:00:00.000Z',
          ),
        ])
      }
      throw new Error(`unexpected ${path}`)
    })

    const result = await searchQueue(
      mustParse(findingUuidQuery('corr-1', 'siem_correlation')),
      demoDayInterval('UTC'),
      'events',
    )

    expect(result.queueOrder.map((item) => item.id)).toEqual(['pt-maxpatrol-siem/evt-in'])
  })

  it('shows an empty queue when every resolved child is outside the time interval', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/context/resolve') {
        return emptyResolve([
          gwEvent('corr-1'),
          gwEvent(
            'evt-out',
            { parent_source_event_id: 'corr-1', relation_type: 'subevent_of' },
            'event',
            '2025-10-22T10:00:00.000Z',
          ),
        ])
      }
      throw new Error(`unexpected ${path}`)
    })

    const result = await searchQueue(
      mustParse(findingUuidQuery('corr-1', 'siem_correlation')),
      demoDayInterval('UTC'),
      'events',
    )

    expect(gatewayPost.mock.calls.map((call) => call[0])).toEqual(['/api/v1/context/resolve'])
    expect(result.queueOrder).toEqual([])
  })
})

describe('searchQueue findings sort', () => {
  function mockFindings(kind: GwFinding['kind']) {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/findings/search') {
        return {
          data: {
            findings: [
              gwFinding('old', '2025-10-23T10:00:00.000Z', kind),
              gwFinding('new', '2025-10-23T18:00:00.000Z', kind),
            ],
            source_states: [],
            source_errors: [],
          },
          error: undefined,
          response: { status: 200 },
        }
      }
      throw new Error(`unexpected ${path}`)
    })
  }

  it('sorts incidents by time asc from the query', async () => {
    mockFindings('siem_incident')
    const result = await searchQueue(
      mustParse('select(time) | sort(time asc)'),
      demoDayInterval('UTC'),
      'siem_incident',
    )
    expect(result.queueOrder.map((item) => item.id)).toEqual([
      'pt-maxpatrol-siem/siem_incident/old',
      'pt-maxpatrol-siem/siem_incident/new',
    ])
  })

  it('keeps default time desc for incidents', async () => {
    mockFindings('siem_incident')
    const result = await searchQueue(
      mustParse('select(time) | sort(time desc)'),
      demoDayInterval('UTC'),
      'siem_incident',
    )
    expect(result.queueOrder.map((item) => item.id)).toEqual([
      'pt-maxpatrol-siem/siem_incident/new',
      'pt-maxpatrol-siem/siem_incident/old',
    ])
  })

  it('sorts correlations by time asc from the query', async () => {
    mockFindings('siem_correlation')
    const result = await searchQueue(
      mustParse('select(time) | sort(time asc)'),
      demoDayInterval('UTC'),
      'siem_correlation',
    )
    expect(result.queueOrder.map((item) => item.id)).toEqual([
      'pt-maxpatrol-siem/siem_correlation/old',
      'pt-maxpatrol-siem/siem_correlation/new',
    ])
  })

  it('preserves gateway findings total', async () => {
    gatewayPost.mockImplementation(async (path: string) => {
      if (path === '/api/v1/findings/search') {
        return {
          data: {
            findings: [gwFinding('one', '2025-10-23T18:00:00.000Z', 'siem_incident')],
            total: 1523,
            source_states: [{ source: 'pt-maxpatrol-siem', status: 'truncated', total: 1523 }],
            source_errors: [],
          },
          error: undefined,
          response: { status: 200 },
        }
      }
      throw new Error(`unexpected ${path}`)
    })
    const result = await searchQueue(mustParse('select(time)'), demoDayInterval('UTC'), 'siem_incident')
    expect(result.total).toBe(1523)
    expect(result.queueOrder).toHaveLength(1)
  })
})

describe('resolveFindingEvents session cache', () => {
  const key: FindingResolveKey = {
    source_code: 'pt-maxpatrol-siem',
    record_type: 'siem_incident',
    external_id: 'inc-1',
    time_range: { from: '2025-10-23T00:00:00.000Z', to: '2025-10-23T23:59:59.000Z' },
  }

  function resolveOk() {
    return {
      data: {
        findings: [
          {
            ...gwFinding('inc-1', '2025-10-23T12:00:00.000Z'),
            entities: [
              { type: 'account', value: 'alice', roles: ['mentions'] },
              { type: 'host', value: 'host-1', roles: ['src'] },
            ],
          },
        ],
        sessions: [],
        events: [gwEvent('child-1')],
        entities: [],
        relations: [],
        resolutions: [],
        source_errors: [],
      },
      error: undefined,
      response: { status: 200 },
    }
  }

  function resolveSoftFail() {
    return {
      data: {
        findings: [{ ...gwFinding('inc-1', '2025-10-23T12:00:00.000Z'), entities: [] }],
        sessions: [],
        events: [],
        entities: [],
        relations: [],
        resolutions: [],
        source_errors: [{ source: 'pt-maxpatrol-siem', message: 'timeout' }],
      },
      error: undefined,
      response: { status: 200 },
    }
  }

  it('reuses a successful response for the same key', async () => {
    gatewayPost.mockResolvedValue(resolveOk())
    const first = await resolveFindingEvents(key)
    const second = await resolveFindingEvents(key)
    expect(gatewayPost).toHaveBeenCalledTimes(1)
    expect(second).toEqual(first)
    expect(first.accounts).toEqual(['alice'])
    expect(first.hosts).toEqual([{ value: 'host-1', roles: ['src'] }])
  })

  it('refetches when force is set', async () => {
    gatewayPost.mockResolvedValue(resolveOk())
    await resolveFindingEvents(key)
    await resolveFindingEvents(key, { force: true })
    expect(gatewayPost).toHaveBeenCalledTimes(2)
  })

  it('does not cache thrown errors', async () => {
    gatewayPost
      .mockResolvedValueOnce({
        data: undefined,
        error: { message: 'boom' },
        response: { status: 500 },
      })
      .mockResolvedValueOnce(resolveOk())
    await expect(resolveFindingEvents(key)).rejects.toThrow()
    await expect(resolveFindingEvents(key)).resolves.toMatchObject({ accounts: ['alice'] })
    expect(gatewayPost).toHaveBeenCalledTimes(2)
  })

  it('does not cache soft-fail empty responses', async () => {
    gatewayPost.mockResolvedValueOnce(resolveSoftFail()).mockResolvedValueOnce(resolveOk())
    const soft = await resolveFindingEvents(key)
    expect(soft.errors.length).toBeGreaterThan(0)
    expect(soft.events).toEqual([])
    const ok = await resolveFindingEvents(key)
    expect(gatewayPost).toHaveBeenCalledTimes(2)
    expect(ok.accounts).toEqual(['alice'])
  })

  it('isolates cache entries by project id', async () => {
    gatewayPost.mockResolvedValue(resolveOk())
    await resolveFindingEvents(key)
    projectIdRef.current = 'project-2'
    await resolveFindingEvents(key)
    expect(gatewayPost).toHaveBeenCalledTimes(2)
  })
})
