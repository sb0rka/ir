import { beforeEach, describe, expect, it, vi } from 'vitest'

const { irPost, projectIdRef } = vi.hoisted(() => ({
  irPost: vi.fn(),
  projectIdRef: { current: 'project-1' as string | null },
}))

vi.mock('./clients', () => ({
  irClient: {
    POST: irPost,
  },
}))

vi.mock('./env', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./env')>()
  return { ...actual, getProjectId: () => projectIdRef.current }
})

import { addContext } from './ir'
import { addHypothesisContext } from './hypotheses'

const importResult = {
  findings: 1,
  sessions: 0,
  events: 0,
  entities: 0,
  nodes: 1,
  edges: 0,
  warnings: [],
}

function okPost() {
  irPost.mockResolvedValue({
    data: importResult,
    response: new Response(null, { status: 201 }),
  })
}

describe('context import expand_findings', () => {
  beforeEach(() => {
    projectIdRef.current = 'project-1'
    irPost.mockReset()
    okPost()
  })

  it('omits expand_findings from addContext when not provided', async () => {
    await addContext('inv-1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
    })

    expect(irPost).toHaveBeenCalledWith(
      '/investigations/{investigation_id}/context',
      expect.objectContaining({
        body: {
          findings: [],
          sessions: [],
          events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
          entities: [],
          seed: false,
        },
      }),
    )
  })

  it('sends expand_findings from addContext when provided', async () => {
    await addContext('inv-1', {
      findings: [
        {
          source_code: 'pt-maxpatrol-siem',
          record_type: 'siem_incident',
          external_id: 'inc-1',
          time_range: { from: '2026-01-01T00:00:00Z', to: '2026-01-02T00:00:00Z' },
        },
      ],
      expandFindings: false,
    })

    expect(irPost.mock.calls[0]?.[1]?.body).toEqual(
      expect.objectContaining({ expand_findings: false, seed: false }),
    )
  })

  it('omits expand_findings from addHypothesisContext when not provided', async () => {
    await addHypothesisContext('inv-1', 'h1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
    })

    expect(irPost).toHaveBeenCalledWith(
      '/investigations/{investigation_id}/hypotheses/{hypothesis_id}/context',
      expect.objectContaining({
        body: {
          findings: [],
          sessions: [],
          events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
          entities: [],
          seed: false,
        },
      }),
    )
  })

  it('sends expand_findings from addHypothesisContext when provided', async () => {
    await addHypothesisContext('inv-1', 'h1', {
      findings: [
        {
          source_code: 'pt-maxpatrol-siem',
          record_type: 'nad_attack',
          external_id: 'atk-1',
          time_range: { from: '2026-01-01T00:00:00Z', to: '2026-01-02T00:00:00Z' },
        },
      ],
      expandFindings: true,
    })

    expect(irPost.mock.calls[0]?.[1]?.body).toEqual(
      expect.objectContaining({ expand_findings: true }),
    )
  })

  it('sends why from addContext when provided', async () => {
    await addContext('inv-1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
      why: '  suspicious login  ',
    })

    expect(irPost.mock.calls[0]?.[1]?.body).toEqual(
      expect.objectContaining({ why: 'suspicious login' }),
    )
  })

  it('omits blank why from addContext and addHypothesisContext', async () => {
    await addContext('inv-1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
      why: '   ',
    })
    expect(irPost.mock.calls[0]?.[1]?.body).not.toHaveProperty('why')

    irPost.mockClear()
    okPost()
    await addHypothesisContext('inv-1', 'h1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
      why: 'hypothesis note',
    })
    expect(irPost.mock.calls[0]?.[1]?.body).toEqual(
      expect.objectContaining({ why: 'hypothesis note' }),
    )
  })
})
