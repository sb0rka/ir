import { createContext, useContext, type ReactNode } from 'react'
import type { ActiveSection, Column, Condition, EventFieldDef, LogicalJoiner, ParseError, QueryAst } from '../../lib/pdql'
import type { AggregateFn } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'

export interface BuilderApi {
  syntaxLabel: string
  fields: EventFieldDef[]
  fieldsLoading: boolean
  fieldsError: string | null
  fieldFreq: Record<string, number>
  query: QueryAst
  activeSection: ActiveSection
  pdqlDraft: string
  parseError: ParseError | null
  defaultDatetimeIso: string
  loadFields: () => Promise<void>
  setActiveSection: (section: ActiveSection) => void
  addField: (name: string, section?: ActiveSection, parentId?: string | null) => void
  removeCondition: (id: string) => void
  updateCondition: (id: string, patch: Partial<Condition>) => void
  setJoiner: (parentId: string | null, index: number, joiner: LogicalJoiner) => void
  wrapAdjacent: (parentId: string | null, joinerIndex: number) => void
  ungroup: (groupId: string) => void
  toggleGroupNegated: (groupId: string) => void
  removeColumn: (id: string) => void
  setColumnSort: (id: string, sort: Column['sort'] | undefined) => void
  setColumnAggregate: (id: string, aggregate: AggregateFn | undefined) => void
  setGroupAggregate: (aggregate: AggregateFn) => void
  setGroupSort: (sort: Column['sort'] | undefined) => void
  removeGroup: (id: string) => void
  moveCondition: (parentId: string | null, index: number, delta: number) => void
  moveColumn: (index: number, delta: number) => void
  moveGroup: (index: number, delta: number) => void
  reorder: (section: ActiveSection, from: number, to: number, parentId?: string | null) => void
  setPdqlDraft: (text: string) => void
  applyPdql: () => boolean
}

const QueryBuilderContext = createContext<BuilderApi | null>(null)

export function QueryBuilderProvider({ value, children }: { value: BuilderApi; children: ReactNode }) {
  return <QueryBuilderContext.Provider value={value}>{children}</QueryBuilderContext.Provider>
}

/** Incident builder uses the PDQL store. Event tabs pass their own editor. */
export function useBuilder(): BuilderApi {
  const override = useContext(QueryBuilderContext)
  const store = usePdqlStore()
  if (override) return override
  return { ...store, syntaxLabel: 'PDQL' }
}
