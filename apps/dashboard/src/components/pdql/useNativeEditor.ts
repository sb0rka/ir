import { useRef, useState } from 'react'
import { formatDql, parseDql } from '../../lib/dql/dql'
import {
  addFieldToAst,
  bumpFieldFreq,
  groupCountColumn,
  loadFieldFreq,
  moveFilterNode,
  parse,
  removeFilterNode,
  removeGroup as removeGroupFromQuery,
  reorderFilterNodes,
  serialize,
  setFilterJoiner,
  setGroupAggregate as setGroupAggregateOnQuery,
  toggleFilterGroupNegated,
  ungroupFilter,
  updateCondition as updateConditionInTree,
  wrapFilterAdjacent,
  type ActiveSection,
  type QueryAst,
} from '../../lib/pdql'
import type { EventFieldDef, ParseError } from '../../lib/pdql'
import type { BuilderApi } from './queryBuilder'

function moveItem<T>(items: T[], index: number, delta: number): T[] {
  const next = index + delta
  if (next < 0 || next >= items.length) return items
  const copy = items.slice()
  const [item] = copy.splice(index, 1)
  copy.splice(next, 0, item)
  return copy
}

function draftText(ast: QueryAst, mode: 'pdql' | 'dql'): string | null {
  if (mode === 'pdql') return serialize(ast)
  return formatDql(ast.filter, ast.joiners)
}

export function useNativeEditor(
  mode: 'pdql' | 'dql',
  fields: EventFieldDef[],
  initial: QueryAst,
): BuilderApi & { init: (ast: QueryAst, iso?: string) => void; snapshot: () => QueryAst } {
  const [query, setQuery] = useState(initial)
  const [activeSection, setActiveSection] = useState<ActiveSection>('filter')
  const [pdqlDraft, setPdqlDraft] = useState(draftText(initial, mode) ?? '')
  const [parseError, setParseError] = useState<ParseError | null>(null)
  const [fieldFreq, setFieldFreq] = useState(loadFieldFreq)
  const [defaultDatetimeIso, setDefaultDatetimeIso] = useState(new Date(0).toISOString())
  const queryRef = useRef(query)
  const draftRef = useRef(pdqlDraft)
  const sectionRef = useRef(activeSection)
  const fieldsRef = useRef(fields)
  queryRef.current = query
  draftRef.current = pdqlDraft
  sectionRef.current = activeSection
  fieldsRef.current = fields

  const publish = (next: QueryAst) => {
    queryRef.current = next
    setQuery(next)
    const text = draftText(next, mode)
    if (text == null) {
      setParseError({ message: 'Условие нельзя записать в DQL без смены смысла', position: 0 })
      return
    }
    draftRef.current = text
    setPdqlDraft(text)
    setParseError(null)
  }

  const api: BuilderApi & { init: (ast: QueryAst, iso?: string) => void; snapshot: () => QueryAst } = {
    syntaxLabel: mode === 'pdql' ? 'PDQL' : 'DQL',
    fields,
    fieldsLoading: false,
    fieldsError: null,
    fieldFreq,
    query,
    activeSection,
    pdqlDraft,
    parseError,
    defaultDatetimeIso,
    loadFields: async () => {},
    setActiveSection: (section) => {
      sectionRef.current = section
      setActiveSection(section)
    },
    addField: (name, section, parentId) => {
      const target = section ?? sectionRef.current
      sectionRef.current = target
      setActiveSection(target)
      setFieldFreq((freq) => bumpFieldFreq(freq, name))
      publish(addFieldToAst(queryRef.current, name, target, fieldsRef.current, parentId ?? null))
    },
    removeCondition: (id) => publish(removeFilterNode(queryRef.current, id)),
    updateCondition: (id, patch) => publish(updateConditionInTree(queryRef.current, id, patch)),
    setJoiner: (parentId, index, joiner) => publish(setFilterJoiner(queryRef.current, parentId, index, joiner)),
    wrapAdjacent: (parentId, joinerIndex) => publish(wrapFilterAdjacent(queryRef.current, parentId, joinerIndex)),
    ungroup: (groupId) => publish(ungroupFilter(queryRef.current, groupId)),
    toggleGroupNegated: (groupId) => publish(toggleFilterGroupNegated(queryRef.current, groupId)),
    removeColumn: (id) => publish({ ...queryRef.current, columns: queryRef.current.columns.filter((item) => item.id !== id) }),
    setColumnSort: (id, sort) => {
      const columns = queryRef.current.columns.map((item) => (item.id === id ? { ...item, sort } : item))
      if (sort) {
        const used = new Set(columns.filter((item) => item.sort && item.id !== id).map((item) => item.sort?.priority ?? 0))
        if (used.has(sort.priority)) {
          let priority = 1
          for (const column of columns) {
            if (!column.sort) continue
            column.sort = { ...column.sort, priority }
            priority += 1
          }
        }
      }
      publish({ ...queryRef.current, columns })
    },
    setColumnAggregate: (id, aggregate) => {
      publish({
        ...queryRef.current,
        columns: queryRef.current.columns.map((item) => (item.id === id ? { ...item, aggregate } : item)),
      })
    },
    setGroupAggregate: (aggregate) => publish(setGroupAggregateOnQuery(queryRef.current, aggregate)),
    setGroupSort: (sort) => {
      let current = queryRef.current
      if (!groupCountColumn(current)) current = setGroupAggregateOnQuery(current, 'count')
      const target = groupCountColumn(current)
      if (!target) return
      const columns = current.columns.map((item) => (item.id === target.id ? { ...item, sort } : item))
      publish({ ...current, columns })
    },
    removeGroup: (id) => publish(removeGroupFromQuery(queryRef.current, id)),
    moveCondition: (parentId, index, delta) => publish(moveFilterNode(queryRef.current, parentId, index, delta)),
    moveColumn: (index, delta) => {
      const columns = moveItem(queryRef.current.columns, index, delta)
      if (columns !== queryRef.current.columns) publish({ ...queryRef.current, columns })
    },
    moveGroup: (index, delta) => {
      const groups = moveItem(queryRef.current.groups, index, delta)
      if (groups !== queryRef.current.groups) publish({ ...queryRef.current, groups })
    },
    reorder: (section, from, to, parentId) => {
      const current = queryRef.current
      if (section === 'filter') {
        publish(reorderFilterNodes(current, parentId ?? null, from, to))
        return
      }
      if (section === 'columns') {
        const columns = current.columns.slice()
        const [item] = columns.splice(from, 1)
        if (item) columns.splice(to, 0, item)
        publish({ ...current, columns })
        return
      }
      const groups = current.groups.slice()
      const [item] = groups.splice(from, 1)
      if (item) groups.splice(to, 0, item)
      publish({ ...current, groups })
    },
    setPdqlDraft: (text) => {
      draftRef.current = text
      setPdqlDraft(text)
    },
    applyPdql: () => {
      if (mode === 'pdql') {
        const result = parse(draftRef.current)
        if (!result.ok) {
          setParseError(result.error)
          return false
        }
        publish(result.ast)
        return true
      }
      const result = parseDql(draftRef.current)
      if (!result.ok) {
        setParseError(result.error)
        return false
      }
      publish({ ...queryRef.current, filter: result.filter, joiners: result.joiners })
      return true
    },
    init: (ast, iso) => {
      if (iso) setDefaultDatetimeIso(iso)
      publish(ast)
      setActiveSection('filter')
    },
    snapshot: () => queryRef.current,
  }
  return api
}
