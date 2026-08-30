import { beforeEach, describe, expect, it, vi } from 'vitest'
import { parse } from '../lib/pdql/parse'
import { findingUuidQuery } from '../lib/pdql'
import { demoDayInterval } from '../components/time-interval/model'
import type { components as Gw } from '@ir/contract/gateway'
import { searchQueue } from './search'

const { gatewayGet, gatewayPost } = vi.hoisted(() => ({
  gatewayGet: vi.fn(),
  gatewayPost: vi.fn(),
}))

vi.mock('./clients', () => ({
  gatewayClient: {
    GET: gatewayGet,
    POST: gatewayPost,
  },
}))

vi.mock('./env', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./env')>()
  return { ...actual, getProjectId: () => 'project-1' }
})

type GwEvent = Gw['schemas']['Event']

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
