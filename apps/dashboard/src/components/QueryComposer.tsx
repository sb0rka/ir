import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { useEffect, useState } from 'react'
import { Braces, Check, History, Loader2, Pencil, Play, Plus, Search } from 'lucide-react'
import {
  addFieldToPdql,
  parseQueuePdql,
  pdqlToChips,
  serialize,
  serializeWithoutChip,
  type ActiveSection,
  type ParseError,
  type ParseResult,
} from '../lib/pdql'
import { filterFingerprint } from '../lib/queryFingerprint'
import { clsx } from '../lib/utils'
import { emptyContextQueue, useAppStore } from '../store/appStore'
import { usePdqlStore } from '../store/pdqlStore'
import {
  DEFAULT_QUEUE_SOURCE,
  QUEUE_SOURCE_OPTIONS,
  type QueryHistoryEntry,
  type QueueSource,
} from '../types'
import { PdqlBuilderModal } from './pdql/PdqlBuilderModal'
import { FieldSearchList } from './pdql/FieldSearchList'
import {
  TimeIntervalButton,
  demoDayInterval,
  intervalButtonLabel,
  type TimeInterval,
} from './time-interval'
import { Button, Chip } from './ui'

const DEMO_DAY_LABEL = '23.10.2025 весь день'
const SECTION_LABELS: { id: ActiveSection; label: string }[] = [
  { id: 'filter', label: 'Фильтр' },
  { id: 'columns', label: 'Поля' },
  { id: 'groups', label: 'Группы' },
]

function formatParseError(error: ParseError): string {
  return error.position > 0 ? `${error.message} (позиция ${error.position})` : error.message
}

function parseErrorText(result: ParseResult): string | null {
  if (result.ok === false) return formatParseError(result.error)
  return null
}

function queueSourceLabel(source: QueueSource | undefined): string | undefined {
  return QUEUE_SOURCE_OPTIONS.find((option) => option.id === source)?.label
}

function QueueSourceToggle({
  value,
  onChange,
}: {
  value: QueueSource
  onChange: (value: QueueSource) => void
}) {
  return (
    <div
      className="inline-flex min-h-9 overflow-hidden rounded border border-border bg-surface-0"
      role="group"
      aria-label="Тип сущности"
    >
      {QUEUE_SOURCE_OPTIONS.map((option) => (
        <button
          key={option.id}
          type="button"
          onClick={() => onChange(option.id)}
          className={clsx(
            'px-2.5 py-1.5 text-xs',
            value === option.id ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:text-fg',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function QueryComposer({
  pdql,
  timeInterval,
  executedFingerprint,
  history,
  extra,
  executing,
  queueSource,
  onPdqlChange,
  onTimeChange,
  onQueueSourceChange,
  onExecute,
  onApplyHistory,
}: {
  pdql: string
  timeInterval: TimeInterval
  executedFingerprint: string | null
  history: QueryHistoryEntry[]
  extra?: React.ReactNode
  executing?: boolean
  queueSource?: QueueSource
  onPdqlChange: (pdql: string) => void
  onTimeChange: (interval: TimeInterval) => void
  onQueueSourceChange?: (source: QueueSource) => void
  onExecute: () => void
  onApplyHistory: (entry: QueryHistoryEntry) => void
}) {
  const loadFields = usePdqlStore((s) => s.loadFields)
  const fields = usePdqlStore((s) => s.fields)
  const fieldFreq = usePdqlStore((s) => s.fieldFreq)
  const fieldsError = usePdqlStore((s) => s.fieldsError)
  const [builderOpen, setBuilderOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [addSection, setAddSection] = useState<ActiveSection>('filter')
  const [addQuery, setAddQuery] = useState('')
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(pdql)
  const [editError, setEditError] = useState<string | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  useEffect(() => {
    void loadFields()
  }, [loadFields])

  useEffect(() => {
    if (!editing) setDraft(pdql)
  }, [editing, pdql])

  const parsed = parseQueuePdql(pdql)
  const chips = parsed.ok ? pdqlToChips(parsed.ast) : []
  const filters = chips.filter((chip) => chip.kind === 'filter')
  const columns = chips.filter((chip) => chip.kind === 'column')
  const groups = chips.filter((chip) => chip.kind === 'group')
  const stale = filterFingerprint(pdql, timeInterval, queueSource) !== executedFingerprint
  const parseError = parseErrorText(parsed)

  const removeChip = (id: string) => {
    if (!parsed.ok) return
    onPdqlChange(serializeWithoutChip(parsed.ast, id))
  }

  const addField = (name: string) => {
    onPdqlChange(addFieldToPdql(pdql, name, addSection, fields))
    setAddOpen(false)
    setAddQuery('')
  }

  const exitEdit = () => {
    const result = parseQueuePdql(draft)
    if (result.ok === false) {
      setEditError(formatParseError(result.error))
      return
    }
    onPdqlChange(serialize(result.ast))
    setEditError(null)
    setEditing(false)
  }

  return (
    <div className="border-b border-border bg-surface-1 px-4 py-3">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <TimeIntervalButton
          value={timeInterval}
          onChange={onTimeChange}
          onExecute={(interval) => {
            onTimeChange(interval)
            onExecute()
          }}
        />
        {queueSource && onQueueSourceChange && (
          <QueueSourceToggle value={queueSource} onChange={onQueueSourceChange} />
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {filters.map((chip) => (
          <Chip key={chip.id} onRemove={() => removeChip(chip.id)}>
            {chip.label}
          </Chip>
        ))}
        {columns.map((chip) => (
          <Chip key={chip.id} onRemove={() => removeChip(chip.id)}>
            <span className="text-fg-dim">select</span> {chip.label}
          </Chip>
        ))}
        {groups.map((chip) => (
          <Chip key={chip.id} onRemove={() => removeChip(chip.id)}>
            {chip.label}
          </Chip>
        ))}
        <div className="relative">
          <Button
            size="sm"
            variant="ghost"
            title="Добавить поле"
            onClick={() => setAddOpen((open) => !open)}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          {addOpen && (
            <>
              <div className="fixed inset-0 z-20" onClick={() => setAddOpen(false)} />
              <div className="absolute left-0 top-full z-30 mt-1 w-80 overflow-hidden rounded border border-border bg-surface-2 shadow-xl">
                <div className="grid grid-cols-3 gap-1 border-b border-border p-1">
                  {SECTION_LABELS.map((section) => (
                    <button
                      key={section.id}
                      type="button"
                      onClick={() => setAddSection(section.id)}
                      className={clsx(
                        'rounded px-1.5 py-1 text-[11px]',
                        addSection === section.id
                          ? 'bg-surface-3 text-fg'
                          : 'text-fg-muted hover:text-fg',
                      )}
                    >
                      {section.label}
                    </button>
                  ))}
                </div>
                <label className="flex items-center gap-1.5 border-b border-border px-2 py-1.5">
                  <Search className="h-3.5 w-3.5 text-fg-dim" />
                  <input
                    autoFocus
                    value={addQuery}
                    onChange={(e) => setAddQuery(e.target.value)}
                    placeholder="Найти поле"
                    className="w-full bg-transparent text-xs text-fg outline-none placeholder:text-fg-dim"
                  />
                </label>
                <div className="max-h-72 overflow-auto">
                  {fieldsError && (
                    <div className="px-3 py-2 text-[11px] text-critical">{fieldsError}</div>
                  )}
                  <DndContext sensors={sensors}>
                    <FieldSearchList
                      idPrefix="composer"
                      fields={fields}
                      freq={fieldFreq}
                      query={addQuery}
                      onChoose={addField}
                      onActivate={addField}
                    />
                  </DndContext>
                </div>
              </div>
            </>
          )}
        </div>
        <div className="relative">
          <Button
            size="sm"
            variant="ghost"
            title="История"
            onClick={() => setHistoryOpen((open) => !open)}
          >
            <History className="h-3.5 w-3.5" />
          </Button>
          {historyOpen && (
            <>
              <div className="fixed inset-0 z-20" onClick={() => setHistoryOpen(false)} />
              <div className="absolute right-0 top-full z-30 mt-1 w-80 overflow-hidden rounded border border-border bg-surface-2 shadow-xl">
                <button
                  type="button"
                  className="flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left hover:bg-surface-3"
                  onClick={() => {
                    onTimeChange(demoDayInterval())
                    setHistoryOpen(false)
                  }}
                >
                  <span className="text-xs text-fg">{DEMO_DAY_LABEL}</span>
                  <span className="font-mono text-[11px] text-fg-dim">пресет времени</span>
                </button>
                {history.length > 0 && (
                  <div className="border-t border-border px-3 py-1 text-[10px] uppercase tracking-wider text-fg-dim">
                    Недавние
                  </div>
                )}
                {history.map((entry) => (
                  <button
                    key={filterFingerprint(entry.pdql, entry.timeInterval, entry.queueSource)}
                    type="button"
                    className="flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left hover:bg-surface-3"
                    onClick={() => {
                      onApplyHistory(entry)
                      setHistoryOpen(false)
                    }}
                  >
                    <span className="w-full truncate font-mono text-[11px] text-fg">
                      {entry.pdql || '—'}
                    </span>
                    <span className="font-mono text-[11px] text-fg-dim">
                      {intervalButtonLabel(entry.timeInterval)}
                      {queueSourceLabel(entry.queueSource)
                        ? ` · ${queueSourceLabel(entry.queueSource)}`
                        : ''}
                    </span>
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
        {extra}
      </div>

      <div className="mt-2 flex items-center gap-2">
        {editing ? (
          <input
            autoFocus
            value={draft}
            spellCheck={false}
            onChange={(e) => {
              setDraft(e.target.value)
              setEditError(null)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') exitEdit()
              if (e.key === 'Escape') {
                setDraft(pdql)
                setEditError(null)
                setEditing(false)
              }
            }}
            className="min-w-0 flex-1 rounded border border-border bg-surface-0 px-2 py-1.5 font-mono text-xs text-fg outline-none focus:border-fg/40"
          />
        ) : (
          <div
            className="min-w-0 flex-1 truncate rounded border border-transparent px-2 py-1.5 font-mono text-xs text-fg-muted"
            title={pdql}
          >
            {pdql || 'select(time) | sort(time desc)'}
          </div>
        )}
        <Button
          size="sm"
          variant="ghost"
          title={editing ? 'Выйти из редактирования' : 'Редактировать PDQL'}
          onClick={() => {
            if (editing) {
              exitEdit()
              return
            }
            setDraft(pdql)
            setEditError(null)
            setEditing(true)
          }}
        >
          {editing ? <Check className="h-3.5 w-3.5" /> : <Pencil className="h-3.5 w-3.5" />}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          title="Конструктор PDQL"
          onClick={() => setBuilderOpen(true)}
        >
          <Braces className="h-3.5 w-3.5" />
        </Button>
        <Button
          size="sm"
          variant={stale ? 'primary' : 'default'}
          disabled={executing}
          title={stale ? 'Показываемый результат не соответствует фильтру' : 'Выполнить поиск'}
          onClick={onExecute}
        >
          {executing ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Play className="h-3.5 w-3.5" />
          )}
          {stale ? 'Выполнить · фильтр изменён' : 'Выполнить'}
        </Button>
      </div>
      {(editError || parseError) && (
        <div className="mt-1 text-[11px] text-critical">{editError ?? parseError}</div>
      )}

      <PdqlBuilderModal
        open={builderOpen}
        initialPdql={pdql}
        onClose={() => setBuilderOpen(false)}
        onApply={(text) => {
          onPdqlChange(text)
          setBuilderOpen(false)
        }}
        onExecute={(text) => {
          onPdqlChange(text)
          setBuilderOpen(false)
          onExecute()
        }}
      />
    </div>
  )
}

export function GlobalQueryComposer() {
  const pdql = useAppStore((s) => s.queuePdql)
  const timeInterval = useAppStore((s) => s.timeInterval)
  const queueSource = useAppStore((s) => s.queueSource)
  const executedFingerprint = useAppStore((s) => s.executedFingerprint)
  const history = useAppStore((s) => s.queryHistory)
  const executing = useAppStore((s) => s.queueLoading)
  const setQueuePdql = useAppStore((s) => s.setQueuePdql)
  const setTimeInterval = useAppStore((s) => s.setTimeInterval)
  const setQueueSource = useAppStore((s) => s.setQueueSource)
  const applyQueueHistory = useAppStore((s) => s.applyQueueHistory)
  const loadQueue = useAppStore((s) => s.loadQueue)

  return (
    <QueryComposer
      pdql={pdql}
      timeInterval={timeInterval}
      queueSource={queueSource}
      executedFingerprint={executedFingerprint}
      history={history}
      executing={executing}
      onPdqlChange={setQueuePdql}
      onTimeChange={setTimeInterval}
      onQueueSourceChange={setQueueSource}
      onApplyHistory={applyQueueHistory}
      onExecute={() => void loadQueue()}
    />
  )
}

export function ContextQueryComposer({
  investigationId,
  extra,
}: {
  investigationId: string
  extra?: React.ReactNode
}) {
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const executeContextQuery = useAppStore((s) => s.executeContextQuery)

  return (
    <QueryComposer
      pdql={queue.pdql}
      timeInterval={queue.timeInterval}
      queueSource={queue.queueSource}
      executedFingerprint={queue.executedFingerprint}
      history={queue.queryHistory}
      executing={queue.loading}
      extra={extra}
      onPdqlChange={(pdql) => setContextQueue(investigationId, { pdql })}
      onTimeChange={(timeInterval) => setContextQueue(investigationId, { timeInterval })}
      onQueueSourceChange={(queueSource) => setContextQueue(investigationId, { queueSource })}
      onApplyHistory={(entry) =>
        setContextQueue(investigationId, {
          pdql: entry.pdql,
          timeInterval: entry.timeInterval,
          queueSource: entry.queueSource ?? DEFAULT_QUEUE_SOURCE,
        })
      }
      onExecute={() => {
        void executeContextQuery(investigationId)
      }}
    />
  )
}
