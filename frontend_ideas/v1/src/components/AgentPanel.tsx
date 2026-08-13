import { useState } from 'react'
import { entities, issueTemplates, useAppStore } from '../store/appStore'
import { Button, Chip, Panel } from './ui'
import { clsx, formatTime, statusLabel } from '../lib/utils'
import {
  CheckCircle2,
  CircleDashed,
  Loader2,
  MessageSquare,
  Plus,
  Square,
  X,
  XCircle,
} from 'lucide-react'

function StatusIcon({ status }: { status: string }) {
  if (status === 'running')
    return <Loader2 className="h-3.5 w-3.5 animate-spin text-proposed" />
  if (status === 'completed')
    return <CheckCircle2 className="h-3.5 w-3.5 text-confirmed" />
  if (status === 'error') return <XCircle className="h-3.5 w-3.5 text-critical" />
  if (status === 'cancelled')
    return <CircleDashed className="h-3.5 w-3.5 text-fg-dim" />
  return null
}

export function AgentPanel({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const issues = useAppStore((s) => s.issues)
  const open = useAppStore((s) => s.agentPanelOpen)
  const setOpen = useAppStore((s) => s.setAgentPanelOpen)
  const runEnrichment = useAppStore((s) => s.runEnrichment)
  const createIssue = useAppStore((s) => s.createIssue)
  const cancelIssue = useAppStore((s) => s.cancelIssue)
  const addComment = useAppStore((s) => s.addIssueComment)

  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [commentDraft, setCommentDraft] = useState('')
  const [showCreate, setShowCreate] = useState(false)

  if (!inv || !open) return null

  const list = inv.issueIds
    .map((id) => issues[id])
    .filter(Boolean)
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))

  const roots = list.filter((i) => !i.parentId)
  const subsOf = (id: string) => list.filter((i) => i.parentId === id)

  return (
    <Panel
      title="ИИ-агент · задачи"
      side="left"
      className="w-96 shrink-0"
      actions={
        <div className="flex items-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => setShowCreate((v) => !v)}>
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <button type="button" onClick={() => setOpen(false)}>
            <X className="h-3.5 w-3.5 text-fg-dim" />
          </button>
        </div>
      }
    >
      <div className="space-y-3 p-3">
        <Button className="w-full" onClick={() => runEnrichment(investigationId)}>
          Запустить насыщение контекста
        </Button>

        {showCreate && (
          <div className="rounded border border-border bg-surface-2 p-2">
            <div className="mb-2 text-[10px] uppercase tracking-wider text-fg-dim">
              Новый issue
            </div>
            <div className="space-y-1">
              {issueTemplates.map((tpl) => (
                <button
                  key={tpl.id}
                  type="button"
                  className="block w-full rounded px-2 py-1.5 text-left text-xs hover:bg-surface-3"
                  onClick={() => {
                    createIssue(
                      investigationId,
                      tpl.id,
                      inv.selectedEntityIds.slice(0, 2).length
                        ? inv.selectedEntityIds.slice(0, 2)
                        : inv.entityIds.slice(0, 1),
                    )
                    setShowCreate(false)
                  }}
                >
                  <div className="font-medium text-fg">{tpl.title}</div>
                  <div className="text-fg-dim">{tpl.description}</div>
                </button>
              ))}
            </div>
          </div>
        )}

        {roots.length === 0 && (
          <div className="rounded border border-dashed border-border px-3 py-6 text-center">
            <div className="text-sm text-fg-muted">Задач пока нет</div>
            <div className="mt-1 text-xs text-fg-dim">
              Запустите насыщение контекста — агент найдёт связанные события и
              предложит их на ревью. Issue по сущности создаётся из панели
              «Детали».
            </div>
          </div>
        )}

        {roots.map((issue) => {
          const expanded = expandedId === issue.id
          const subs = subsOf(issue.id)
          return (
            <div
              key={issue.id}
              className="rounded border border-border bg-surface-0"
            >
              <button
                type="button"
                className="flex w-full items-start gap-2 p-2.5 text-left"
                onClick={() => setExpandedId(expanded ? null : issue.id)}
              >
                <StatusIcon status={issue.status} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{issue.title}</span>
                    <Chip>{statusLabel[issue.status]}</Chip>
                  </div>
                  <div className="mt-0.5 text-[11px] text-fg-dim">
                    {formatTime(issue.createdAt)} · {issue.template}
                  </div>
                  {issue.entityIds.length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {issue.entityIds.map((id) => (
                        <span
                          key={id}
                          className="rounded border border-border px-1 font-mono text-[10px] text-fg-muted"
                        >
                          {entities[id]?.label ?? id}
                        </span>
                      ))}
                    </div>
                  )}
                  {(issue.eventsFound > 0 ||
                    issue.edgesFound > 0 ||
                    issue.findingsFound > 0) && (
                    <div className="mt-1 text-[11px] text-fg-muted">
                      +{issue.eventsFound} соб. · +{issue.edgesFound} связей · +
                      {issue.findingsFound} находок
                    </div>
                  )}
                  {issue.resultSummary && (
                    <div className="mt-1 text-[11px] text-fg-dim">{issue.resultSummary}</div>
                  )}
                </div>
              </button>

              {issue.status === 'running' && (
                <div className="border-t border-border px-2.5 py-1.5">
                  <Button size="sm" variant="ghost" onClick={() => cancelIssue(issue.id)}>
                    <Square className="h-3 w-3" /> Остановить
                  </Button>
                </div>
              )}

              {expanded && (
                <div className="space-y-2 border-t border-border p-2.5">
                  <p className="text-xs text-fg-muted">{issue.description}</p>

                  <div className="flex gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() =>
                        createIssue(
                          investigationId,
                          'tpl-reputation',
                          issue.entityIds,
                          issue.id,
                        )
                      }
                    >
                      + Sub-issue
                    </Button>
                  </div>

                  {subs.map((sub) => (
                    <div
                      key={sub.id}
                      className={clsx(
                        'ml-2 rounded border border-border/80 bg-surface-2 p-2 text-xs',
                      )}
                    >
                      <div className="flex items-center gap-1.5">
                        <StatusIcon status={sub.status} />
                        <span>{sub.title}</span>
                        <Chip>{statusLabel[sub.status]}</Chip>
                      </div>
                      {sub.resultSummary && (
                        <div className="mt-1 text-fg-dim">{sub.resultSummary}</div>
                      )}
                    </div>
                  ))}

                  <div>
                    <div className="mb-1 flex items-center gap-1 text-[10px] uppercase tracking-wider text-fg-dim">
                      <MessageSquare className="h-3 w-3" />
                      Комментарии ({issue.comments.length})
                    </div>
                    <div className="space-y-1.5">
                      {issue.comments.map((c) => (
                        <div key={c.id} className="rounded bg-surface-2 p-1.5 text-xs">
                          <div className="text-fg-dim">
                            {c.author} · {formatTime(c.time)}
                          </div>
                          <div className="text-fg-muted">{c.text}</div>
                        </div>
                      ))}
                    </div>
                    <form
                      className="mt-2 flex gap-1"
                      onSubmit={(e) => {
                        e.preventDefault()
                        if (!commentDraft.trim()) return
                        addComment(issue.id, commentDraft.trim())
                        setCommentDraft('')
                      }}
                    >
                      <input
                        className="flex-1 rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30"
                        placeholder="Комментарий / обоснование…"
                        value={commentDraft}
                        onChange={(e) => setCommentDraft(e.target.value)}
                      />
                      <Button size="sm" type="submit">
                        →
                      </Button>
                    </form>
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </Panel>
  )
}
