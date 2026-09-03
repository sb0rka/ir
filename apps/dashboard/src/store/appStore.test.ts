import { afterEach, describe, expect, it } from 'vitest'
import { findingUuidQuery, pdqlToChips, parseQueuePdql } from '../lib/pdql'
import { emptyContextQueue, useAppStore } from './appStore'

const initial = {
  queuePdql: useAppStore.getState().queuePdql,
  queueSource: useAppStore.getState().queueSource,
  groupValues: useAppStore.getState().groupValues,
  eventGroups: useAppStore.getState().eventGroups,
  findingFilterWarnAt: useAppStore.getState().findingFilterWarnAt,
  contextQueue: useAppStore.getState().contextQueue,
}

afterEach(() => {
  useAppStore.setState(initial)
})

describe('filterByFindingUuid', () => {
  it('sets events source, drops other filters, and keeps an Incident/Correlation chip', () => {
    useAppStore.setState({
      queuePdql: 'filter(action = "login") | select(time) | sort(time desc)',
      queueSource: 'siem_incident',
      groupValues: ['dc01'],
      eventGroups: [{ source_code: 'pt-maxpatrol-siem', values: ['dc01'], count: 3 }],
    })

    useAppStore.getState().filterByFindingUuid(null, '  corr-uuid  ', 'siem_correlation')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('events')
    expect(state.queuePdql).toBe(findingUuidQuery('corr-uuid', 'siem_correlation'))
    expect(state.groupValues).toEqual([])
    expect(state.eventGroups).toEqual([])
    const parsed = parseQueuePdql(state.queuePdql)
    expect(parsed.ok).toBe(true)
    if (!parsed.ok) return
    expect(pdqlToChips(parsed.ast).filter((chip) => chip.kind === 'filter').map((chip) => chip.label)).toEqual([
      'Корреляция = "corr-uuid"',
    ])
  })

  it('updates the investigation context queue the same way', () => {
    useAppStore.getState().filterByFindingUuid('inv-1', 'inc-9', 'siem_incident')

    const queue = useAppStore.getState().contextQueue['inv-1']
    expect(queue?.queueSource).toBe('events')
    expect(queue?.pdql).toBe(findingUuidQuery('inc-9', 'siem_incident'))
    expect(queue?.groupValues).toEqual([])
    expect(queue?.timeInterval).toEqual(emptyContextQueue.timeInterval)
  })

  it('rejects extra filters and warns while a finding chip is set', () => {
    useAppStore.getState().filterByFindingUuid(null, 'inc-1', 'siem_incident')
    const pdql = useAppStore.getState().queuePdql
    const before = useAppStore.getState().findingFilterWarnAt

    useAppStore.getState().appendPdqlFilter(null, 'action', 'login')

    const state = useAppStore.getState()
    expect(state.queuePdql).toBe(pdql)
    expect(state.findingFilterWarnAt).toBeGreaterThan(before)
  })
})
