import { describe, expect, it } from 'vitest'
import { ALL_ENTITY_TYPES, ALL_SEVERITIES } from './constants'
import { buildVisibleGraph, type GraphFilters } from './graph-adapters'
import type { AlertNode, Edge, Entity } from './types'

const filters: GraphFilters = {
  entityTypes: new Set(ALL_ENTITY_TYPES),
  severities: new Set(ALL_SEVERITIES),
  edgeOrigins: new Set(['agent', 'analyst']),
  timeRange: null,
}

function entity(id: string, overrides: Partial<Entity> = {}): Entity {
  return {
    id,
    type_code: 'host',
    key: id,
    display_name: id,
    position: { x: 0, y: 0 },
    origin: 'analyst',
    ...overrides,
  }
}

function alert(id: string, overrides: Partial<AlertNode> = {}): AlertNode {
  return {
    id,
    title: id,
    severity: 'high',
    event_ts: '2026-01-01T00:00:00.000Z',
    source: 'siem',
    description: id,
    position: { x: 0, y: 0 },
    origin: 'analyst',
    ...overrides,
  }
}

function edge(id: string, source_id: string, target_id: string): Edge {
  return {
    id,
    source_id,
    target_id,
    kind: 'related',
    origin: 'analyst',
    status: 'confirmed',
    confidence: 1,
  }
}

const base = {
  entities: [entity('n-host'), entity('n-other')],
  alerts: [alert('n-evt'), alert('n-noise')],
  edges: [edge('e-in', 'n-host', 'n-evt'), edge('e-out', 'n-host', 'n-other')],
  events: [],
  filters,
  selection: null,
  hoverEventId: null,
}

const visibility = {
  visibleNodeIds: new Set(['n-host', 'n-evt']),
  highlightedNodeIds: new Set(['n-host', 'n-evt']),
  activeNodeIds: new Set(['n-host', 'n-evt']),
  writable: true,
}

describe('buildVisibleGraph hypothesis layers', () => {
  it('dims outsiders and keeps them on the canvas', () => {
    const { nodes, edges } = buildVisibleGraph({
      ...base,
      graphVisibility: { ...visibility, visibleNodeIds: null },
    })
    expect(nodes.map((n) => n.id).sort()).toEqual(['n-evt', 'n-host', 'n-noise', 'n-other'])
    expect(nodes.find((n) => n.id === 'n-host')?.data.dimmed).toBe(false)
    expect(nodes.find((n) => n.id === 'n-other')?.data.dimmed).toBe(true)
    expect(nodes.find((n) => n.id === 'n-noise')?.data.dimmed).toBe(true)
    expect(edges.map((e) => e.id).sort()).toEqual(['e-in', 'e-out'])
    expect(edges.find((e) => e.id === 'e-out')?.style?.opacity).toBe(0.15)
  })

  it('hides outsiders when visibleNodeIds is set', () => {
    const { nodes, edges } = buildVisibleGraph({
      ...base,
      graphVisibility: { ...visibility, highlightedNodeIds: null },
    })
    expect(nodes.map((n) => n.id).sort()).toEqual(['n-evt', 'n-host'])
    expect(nodes.every((n) => n.data.dimmed === false)).toBe(true)
    expect(edges.map((e) => e.id)).toEqual(['e-in'])
  })
})
