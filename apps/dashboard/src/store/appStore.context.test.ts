import { afterEach, expect, it, vi } from 'vitest'
import { emptyContextQueue, useAppStore } from './appStore'
import * as irApi from '../api/ir'
import type { AlertEvent } from '../types'

const initial = useAppStore.getState()
afterEach(() => {
  vi.restoreAllMocks()
  useAppStore.setState(initial, true)
})

it.each([true, false])('passes expandFindings=%s for findings and explicit events', async (resolve) => {
  const findingRef: NonNullable<AlertEvent['findingRef']> = {
    source_code: 'pt-maxpatrol-siem', record_type: 'siem_incident', external_id: 'incident-1',
    time_range: { from: '2026-09-01T00:00:00Z', to: '2026-09-02T00:00:00Z' },
  }
  const alert: AlertEvent = {
    id: 'finding', title: 'Incident', time: findingRef.time_range.from,
    severity: 'high', rule: '', source: 'pt-maxpatrol-siem', status: 'new',
    entityIds: [], description: '', findingRef,
  }
  const add = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
  const reload = vi.fn().mockResolvedValue(undefined)
  useAppStore.setState({
    loadInvestigation: reload,
    contextQueue: { inv: {
      ...emptyContextQueue, expandFindings: resolve, selectedIds: ['finding', 'event'],
      alerts: {
        finding: alert,
        event: { ...alert, id: 'event', findingRef: undefined, sourceEventId: 'event-1' },
      },
    } },
  })
  await useAppStore.getState().addEventsToContext('inv', ['finding', 'event'])
  expect(add).toHaveBeenCalledWith('inv', {
    expandFindings: resolve, findings: [findingRef],
    events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'event-1' }],
  })
  expect(reload).toHaveBeenCalledWith('inv')
  expect(useAppStore.getState().contextQueue.inv.expandFindings).toBe(resolve)
})
