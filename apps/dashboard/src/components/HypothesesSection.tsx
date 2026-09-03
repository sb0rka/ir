import { useMemo, useState, type ReactNode } from 'react'
import type { Hypothesis } from '../api/hypotheses'
import {
  EMPTY_LAYER_IDS,
  hypothesisOriginLabel,
  hypothesisStatusLabel,
  INVESTIGATION_LAYER_ID,
  investigationLayerNodeIds,
  layerItemIds,
} from '../lib/hypotheses'
import { useAppStore } from '../store/appStore'
import { Button, Chip } from './ui'
import { clsx } from '../lib/utils'
import { ChevronDown, ChevronRight, Eye, EyeOff, Focus, Highlighter, Plus } from 'lucide-react'

type StatusFilter = 'all' | 'open' | 'resolved'

export function HypothesesSection({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const hypotheses = useAppStore((s) => s.hypotheses)
  const membership = useAppStore((s) => s.hypothesisMembership)
  const activeId = useAppStore((s) => s.activeHypothesisId[investigationId] ?? null)
  const storedVisibleIds = useAppStore((s) => s.visibleHypothesisIds[investigationId])
  const storedHighlightedIds = useAppStore((s) => s.highlightedHypothesisIds[investigationId])
  const draftOpen = useAppStore((s) => s.hypothesisDraftOpen)
  const setDraftOpen = useAppStore((s) => s.setHypothesisDraftOpen)
  const setActive = useAppStore((s) => s.setActiveHypothesis)
  const createHypothesis = useAppStore((s) => s.createHypothesis)
  const patchHypothesis = useAppStore((s) => s.patchHypothesis)
  const deleteHypothesis = useAppStore((s) => s.deleteHypothesis)
  const toggleVisible = useAppStore((s) => s.toggleHypothesisVisible)
  const toggleHighlight = useAppStore((s) => s.toggleHypothesisHighlight)

  const [filter, setFilter] = useState<StatusFilter>('all')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [statement, setStatement] = useState('')
  const [description, setDescription] = useState('')
  const [includeSelection, setIncludeSelection] = useState(true)
  const [resolveId, setResolveId] = useState<string | null>(null)
  const [reason, setReason] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const items = useMemo(() => {
    const ids = inv?.hypothesisIds ?? EMPTY_LAYER_IDS
    return ids
      .map((id) => hypotheses[id])
      .filter(Boolean)
      .filter((item) => {
        if (filter === 'resolved') return item.status === 'resolved'
        if (filter === 'open') return item.status !== 'resolved'
        return true
      })
  }, [filter, hypotheses, inv?.hypothesisIds])

  const investigationNodeCount = useMemo(() => {
    if (!inv) return 0
    return investigationLayerNodeIds(inv.nodeIds, inv.hypothesisIds, hypotheses, membership).length
  }, [hypotheses, inv, membership])

  const visibleIds = useMemo(
    () => storedVisibleIds ?? layerItemIds(inv?.hypothesisIds ?? EMPTY_LAYER_IDS),
    [inv?.hypothesisIds, storedVisibleIds],
  )
  const highlightedIds = storedHighlightedIds ?? EMPTY_LAYER_IDS

  if (!inv) return null

  const selectedCount = inv.selectedEntityIds.length
  const investigationSelected = activeId == null

  return (
    <div className="space-y-3 p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex rounded border border-border p-0.5">
          {(
            [
              ['all', 'все'],
              ['open', 'в работе'],
              ['resolved', 'закрытые'],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              className={clsx(
                'rounded px-1.5 py-0.5 text-[10px]',
                filter === id ? 'bg-surface-3 text-fg' : 'text-fg-muted',
              )}
              onClick={() => setFilter(id)}
            >
              {label}
            </button>
          ))}
        </div>
        <Button size="sm" variant="ghost" onClick={() => setDraftOpen(!draftOpen)}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>

      {draftOpen && (
        <form
          className="space-y-2 rounded border border-border bg-surface-2 p-2"
          onSubmit={(e) => {
            e.preventDefault()
            void createHypothesis(investigationId, {
              statement,
              description: description || undefined,
              includeSelection: includeSelection && selectedCount > 0,
            }).then((created) => {
              if (!created) return
              setStatement('')
              setDescription('')
            })
          }}
        >
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">Новая гипотеза</div>
          <input
            className="w-full rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30"
            placeholder="Формулировка версии событий"
            maxLength={255}
            value={statement}
            onChange={(e) => setStatement(e.target.value)}
            required
          />
          <textarea
            className="w-full resize-none rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30"
            placeholder="Контекст и шаги проверки"
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
          {selectedCount > 0 && (
            <label className="flex items-center gap-2 text-xs text-fg-muted">
              <input
                type="checkbox"
                checked={includeSelection}
                onChange={(e) => setIncludeSelection(e.target.checked)}
              />
              Включить {selectedCount} выбранных узлов и связи между ними
            </label>
          )}
          <div className="flex justify-end gap-1">
            <Button size="sm" variant="ghost" type="button" onClick={() => setDraftOpen(false)}>
              Отмена
            </Button>
            <Button size="sm" type="submit" disabled={!statement.trim()}>
              Создать
            </Button>
          </div>
        </form>
      )}

      <LayerCard
        selected={investigationSelected}
        visible={visibleIds.includes(INVESTIGATION_LAYER_ID)}
        highlighted={highlightedIds.includes(INVESTIGATION_LAYER_ID)}
        isSolo={visibleIds.length === 1 && visibleIds[0] === INVESTIGATION_LAYER_ID}
        onSelect={() => void setActive(investigationId, null)}
        onToggleVisible={(solo) =>
          toggleVisible(investigationId, INVESTIGATION_LAYER_ID, solo)
        }
        onToggleHighlight={(solo) =>
          toggleHighlight(investigationId, INVESTIGATION_LAYER_ID, solo)
        }
        onToggleSolo={() => toggleVisible(investigationId, INVESTIGATION_LAYER_ID, true)}
        title={inv.title}
        meta={
          <>
            <Chip tone="default">расследование</Chip>
            <NodeCount count={investigationNodeCount} />
          </>
        }
      />

      {items.map((item) => (
        <HypothesisCard
          key={item.id}
          item={item}
          active={activeId === item.id}
          expanded={expandedId === item.id}
          nodeCount={membership[item.id]?.nodeIds.length}
          visible={visibleIds.includes(item.id)}
          highlighted={highlightedIds.includes(item.id)}
          isSolo={visibleIds.length === 1 && visibleIds[0] === item.id}
          resolving={resolveId === item.id}
          deleting={deleteId === item.id}
          reason={reason}
          onReason={setReason}
          onSelect={() => void setActive(investigationId, item.id)}
          onToggleExpand={() => setExpandedId((id) => (id === item.id ? null : item.id))}
          onToggleVisible={(solo) => toggleVisible(investigationId, item.id, solo)}
          onToggleHighlight={(solo) => toggleHighlight(investigationId, item.id, solo)}
          onToggleSolo={() => toggleVisible(investigationId, item.id, true)}
          onActivate={() => void patchHypothesis(investigationId, item.id, { status: 'active' })}
          onReopen={() => void patchHypothesis(investigationId, item.id, { status: 'active' })}
          onAskResolve={() => {
            setResolveId(item.id)
            setReason(item.reason ?? '')
          }}
          onCancelResolve={() => {
            setResolveId(null)
            setReason('')
          }}
          onResolve={() => {
            void patchHypothesis(investigationId, item.id, {
              status: 'resolved',
              reason: reason.trim(),
            }).then((updated) => {
              if (updated) {
                setResolveId(null)
                setReason('')
              }
            })
          }}
          onAskDelete={() => setDeleteId(item.id)}
          onCancelDelete={() => setDeleteId(null)}
          onDelete={() => {
            void deleteHypothesis(investigationId, item.id)
            setDeleteId(null)
          }}
        />
      ))}

      {items.length === 0 && (
        <div className="rounded border border-border p-4 text-center text-xs text-fg-dim">
          Гипотез нет
          <div className="mt-2">
            <Button size="sm" onClick={() => setDraftOpen(true)}>
              Создать гипотезу
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

function NodeCount({ count }: { count: number }) {
  return <span className="rounded bg-surface-2 px-1.5 py-0.5">узлов: {count}</span>
}

function LayerCard({
  selected,
  visible,
  highlighted,
  isSolo,
  onSelect,
  onToggleVisible,
  onToggleHighlight,
  onToggleSolo,
  title,
  meta,
}: {
  selected: boolean
  visible: boolean
  highlighted: boolean
  isSolo: boolean
  onSelect: () => void
  onToggleVisible: (solo: boolean) => void
  onToggleHighlight: (solo: boolean) => void
  onToggleSolo: () => void
  title: string
  meta: ReactNode
}) {
  return (
    <div
      className={clsx(
        'overflow-hidden rounded border shadow-xs',
        selected
          ? 'border-accent bg-accent/10 ring-1 ring-accent/50'
          : 'border-border bg-surface-0',
      )}
    >
      <div className="flex items-start gap-1.5 p-3">
        <span className="mt-0.5 p-0.5" aria-hidden>
          <span className="block h-3.5 w-3.5" />
        </span>
        <button type="button" className="min-w-0 flex-1 text-left" onClick={onSelect}>
          <span className="block break-words text-xs font-semibold text-fg">{title}</span>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-fg-dim">
            {meta}
          </div>
        </button>
        <LayerToggles
          visible={visible}
          highlighted={highlighted}
          isSolo={isSolo}
          onToggleVisible={onToggleVisible}
          onToggleHighlight={onToggleHighlight}
          onToggleSolo={onToggleSolo}
        />
      </div>
    </div>
  )
}

function LayerToggles({
  visible,
  highlighted,
  isSolo,
  onToggleVisible,
  onToggleHighlight,
  onToggleSolo,
}: {
  visible: boolean
  highlighted: boolean
  isSolo?: boolean
  onToggleVisible: (solo: boolean) => void
  onToggleHighlight: (solo: boolean) => void
  onToggleSolo?: () => void
}) {
  return (
    <div className="mt-0.5 flex shrink-0 items-center gap-0.5">
      <button
        type="button"
        className={clsx(
          'rounded p-0.5',
          visible ? 'bg-accent/15 text-accent' : 'text-fg-dim hover:text-fg',
        )}
        title={
          visible
            ? 'Скрыть узлы. Alt+клик — только этот слой'
            : 'Показать узлы. Alt+клик — только этот слой'
        }
        onClick={(e) => {
          e.stopPropagation()
          onToggleVisible(e.altKey || e.metaKey)
        }}
      >
        {visible ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
      </button>
      {onToggleSolo && (
        <button
          type="button"
          className={clsx(
            'rounded p-0.5',
            isSolo ? 'bg-accent/20 text-accent ring-1 ring-accent/40' : 'text-fg-dim hover:text-fg',
          )}
          title={isSolo ? 'Отключить соло (показать все слои)' : 'Показать только этот слой (соло)'}
          onClick={(e) => {
            e.stopPropagation()
            onToggleSolo()
          }}
        >
          <Focus className="h-3.5 w-3.5" />
        </button>
      )}
      <button
        type="button"
        className={clsx(
          'rounded p-0.5',
          highlighted ? 'bg-accent/15 text-accent' : 'text-fg-dim hover:text-fg',
        )}
        title={
          highlighted
            ? 'Снять подсветку. Alt+клик — подсветить только этот слой'
            : 'Подсветить. Alt+клик — подсветить только этот слой'
        }
        onClick={(e) => {
          e.stopPropagation()
          onToggleHighlight(e.altKey || e.metaKey)
        }}
      >
        <Highlighter className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

function HypothesisCard({
  item,
  active,
  expanded,
  nodeCount,
  visible,
  highlighted,
  isSolo,
  resolving,
  deleting,
  reason,
  onReason,
  onSelect,
  onToggleExpand,
  onToggleVisible,
  onToggleHighlight,
  onToggleSolo,
  onActivate,
  onReopen,
  onAskResolve,
  onCancelResolve,
  onResolve,
  onAskDelete,
  onCancelDelete,
  onDelete,
}: {
  item: Hypothesis
  active: boolean
  expanded: boolean
  nodeCount?: number
  visible: boolean
  highlighted: boolean
  isSolo: boolean
  resolving: boolean
  deleting: boolean
  reason: string
  onReason: (value: string) => void
  onSelect: () => void
  onToggleExpand: () => void
  onToggleVisible: (solo: boolean) => void
  onToggleHighlight: (solo: boolean) => void
  onToggleSolo: () => void
  onActivate: () => void
  onReopen: () => void
  onAskResolve: () => void
  onCancelResolve: () => void
  onResolve: () => void
  onAskDelete: () => void
  onCancelDelete: () => void
  onDelete: () => void
}) {
  const tone =
    item.status === 'resolved' ? 'confirmed' : item.status === 'active' ? 'proposed' : 'default'

  return (
    <div
      className={clsx(
        'overflow-hidden rounded border shadow-xs',
        active
          ? 'border-accent bg-accent/10 ring-1 ring-accent/50'
          : 'border-border bg-surface-0',
      )}
    >
      <div className="flex items-start gap-1.5 p-3">
        <button
          type="button"
          className="mt-0.5 p-0.5 text-fg-dim hover:text-fg"
          onClick={onToggleExpand}
        >
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
        </button>
        <button type="button" className="min-w-0 flex-1 text-left" onClick={onSelect}>
          <span className="block break-words text-xs font-semibold text-fg">{item.statement}</span>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-fg-dim">
            <Chip tone={tone}>{hypothesisStatusLabel(item.status)}</Chip>
            <span>{hypothesisOriginLabel(item.origin)}</span>
            {nodeCount != null && <NodeCount count={nodeCount} />}
          </div>
          {item.status === 'resolved' && item.reason && !expanded && (
            <div className="mt-1 line-clamp-2 text-xs text-fg-muted">{item.reason}</div>
          )}
        </button>
        <LayerToggles
          visible={visible}
          highlighted={highlighted}
          isSolo={isSolo}
          onToggleVisible={onToggleVisible}
          onToggleHighlight={onToggleHighlight}
          onToggleSolo={onToggleSolo}
        />
      </div>

      {expanded && (
        <div className="space-y-2 border-t border-border/80 bg-surface-2/30 p-3 text-xs">
          {item.description && (
            <p className="whitespace-pre-wrap text-fg-muted">{item.description}</p>
          )}
          {item.reason && <p className="text-fg-muted">Закрыта: {item.reason}</p>}

          {resolving ? (
            <div className="space-y-1">
              <textarea
                className="w-full resize-none rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30"
                rows={2}
                placeholder="Обоснование закрытия"
                value={reason}
                onChange={(e) => onReason(e.target.value)}
              />
              <div className="flex gap-1">
                <Button size="sm" disabled={!reason.trim()} onClick={onResolve}>
                  Закрыть
                </Button>
                <Button size="sm" variant="ghost" onClick={onCancelResolve}>
                  Отмена
                </Button>
              </div>
            </div>
          ) : deleting ? (
            <div className="space-y-1">
              <p className="text-fg-muted">Граф расследования не изменится. Удалить гипотезу?</p>
              <div className="flex gap-1">
                <Button size="sm" variant="danger" onClick={onDelete}>
                  Удалить
                </Button>
                <Button size="sm" variant="ghost" onClick={onCancelDelete}>
                  Отмена
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex flex-wrap gap-1">
              {item.status === 'proposed' && (
                <Button size="sm" onClick={onActivate}>
                  Активировать
                </Button>
              )}
              {item.status !== 'resolved' && (
                <Button size="sm" variant="ghost" onClick={onAskResolve}>
                  Закрыть…
                </Button>
              )}
              {item.status === 'resolved' && (
                <Button size="sm" onClick={onReopen}>
                  Вернуть в работу
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={onAskDelete}>
                Удалить
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
