import { useMemo, useState } from 'react'
import type { Hypothesis } from '../api/hypotheses'
import {
  hypothesisOriginLabel,
  hypothesisStatusLabel,
} from '../lib/hypotheses'
import { useAppStore } from '../store/appStore'
import { Button, Chip } from './ui'
import { clsx } from '../lib/utils'
import { ChevronDown, ChevronRight, Plus } from 'lucide-react'

type StatusFilter = 'all' | 'open' | 'resolved'

export function HypothesesSection({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const hypotheses = useAppStore((s) => s.hypotheses)
  const membership = useAppStore((s) => s.hypothesisMembership)
  const activeId = useAppStore((s) => s.activeHypothesisId[investigationId] ?? null)
  const draftOpen = useAppStore((s) => s.hypothesisDraftOpen)
  const setDraftOpen = useAppStore((s) => s.setHypothesisDraftOpen)
  const setActive = useAppStore((s) => s.setActiveHypothesis)
  const createHypothesis = useAppStore((s) => s.createHypothesis)
  const patchHypothesis = useAppStore((s) => s.patchHypothesis)
  const deleteHypothesis = useAppStore((s) => s.deleteHypothesis)

  const [filter, setFilter] = useState<StatusFilter>('all')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [statement, setStatement] = useState('')
  const [description, setDescription] = useState('')
  const [includeSelection, setIncludeSelection] = useState(true)
  const [resolveId, setResolveId] = useState<string | null>(null)
  const [reason, setReason] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const items = useMemo(() => {
    const ids = inv?.hypothesisIds ?? []
    return ids
      .map((id) => hypotheses[id])
      .filter(Boolean)
      .filter((item) => {
        if (filter === 'resolved') return item.status === 'resolved'
        if (filter === 'open') return item.status !== 'resolved'
        return true
      })
  }, [filter, hypotheses, inv?.hypothesisIds])

  if (!inv) return null

  const selectedCount = inv.selectedEntityIds.length

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

      {items.map((item) => (
        <HypothesisCard
          key={item.id}
          item={item}
          active={activeId === item.id}
          expanded={expandedId === item.id}
          nodeCount={membership[item.id]?.nodeIds.length}
          resolving={resolveId === item.id}
          deleting={deleteId === item.id}
          reason={reason}
          onReason={setReason}
          onToggleActive={() => void setActive(investigationId, item.id)}
          onToggleExpand={() => setExpandedId((id) => (id === item.id ? null : item.id))}
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
          onSaveStatement={(next) =>
            void patchHypothesis(investigationId, item.id, { statement: next })
          }
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

function HypothesisCard({
  item,
  active,
  expanded,
  nodeCount,
  resolving,
  deleting,
  reason,
  onReason,
  onToggleActive,
  onToggleExpand,
  onActivate,
  onReopen,
  onAskResolve,
  onCancelResolve,
  onResolve,
  onAskDelete,
  onCancelDelete,
  onDelete,
  onSaveStatement,
}: {
  item: Hypothesis
  active: boolean
  expanded: boolean
  nodeCount?: number
  resolving: boolean
  deleting: boolean
  reason: string
  onReason: (value: string) => void
  onToggleActive: () => void
  onToggleExpand: () => void
  onActivate: () => void
  onReopen: () => void
  onAskResolve: () => void
  onCancelResolve: () => void
  onResolve: () => void
  onAskDelete: () => void
  onCancelDelete: () => void
  onDelete: () => void
  onSaveStatement: (statement: string) => void
}) {
  const [draft, setDraft] = useState(item.statement)
  const tone =
    item.status === 'resolved' ? 'confirmed' : item.status === 'active' ? 'proposed' : 'default'

  return (
    <div
      className={clsx(
        'rounded border bg-surface-0 overflow-hidden shadow-xs',
        active ? 'border-accent' : 'border-border',
      )}
    >
      <div className="flex items-start gap-2 p-2.5">
        <button
          type="button"
          className="mt-0.5 text-fg-dim hover:text-fg p-0.5"
          onClick={onToggleExpand}
        >
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
        </button>
        <button type="button" className="min-w-0 flex-1 text-left" onClick={onToggleActive}>
          <div className="flex items-center gap-2">
            <span
              className={clsx(
                'h-1.5 w-1.5 shrink-0 rounded-full',
                item.status === 'active' && 'bg-proposed',
                item.status === 'proposed' && 'bg-fg-dim',
                item.status === 'resolved' && 'bg-confirmed',
              )}
            />
            <span className="line-clamp-2 text-xs font-semibold text-fg">{item.statement}</span>
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-fg-dim">
            <Chip tone={tone}>{hypothesisStatusLabel(item.status)}</Chip>
            <span>{hypothesisOriginLabel(item.origin)}</span>
            {nodeCount != null && <span>· узлов: {nodeCount}</span>}
          </div>
          {item.status === 'resolved' && item.reason && !expanded && (
            <div className="mt-1 line-clamp-2 text-xs text-fg-muted">{item.reason}</div>
          )}
        </button>
      </div>

      {expanded && (
        <div className="space-y-2 border-t border-border/80 bg-surface-2/30 p-2.5 text-xs">
          <label className="block space-y-1">
            <span className="text-[10px] uppercase tracking-wider text-fg-dim">Формулировка</span>
            <textarea
              className="w-full resize-none rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30"
              rows={2}
              maxLength={255}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onBlur={() => {
                const next = draft.trim()
                if (next && next !== item.statement) onSaveStatement(next)
                else setDraft(item.statement)
              }}
            />
          </label>
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
