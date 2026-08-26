import { useEffect, useMemo, useRef, useState } from 'react'
import { issueTemplates, useAppStore } from '../store/appStore'
import { Button, Chip, Panel } from './ui'
import { clsx, formatTime, statusLabel } from '../lib/utils'
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleDashed,
  Loader2,
  MessageSquare,
  Play,
  Plus,
  Square,
  X,
  XCircle,
} from 'lucide-react'
import { TreeItem } from './TreeItem'
import { buildEntityTree, type ProposedTreeNode } from '../lib/tree-bundler'
import type { components } from '@ir/contract'

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

type SomIssue = components['schemas']['SomIssue']

interface SomIssueTreeNode {
  issue: SomIssue
  children: SomIssueTreeNode[]
}

function SomIssueTreeItem({
  node,
  investigationId,
  depth = 0
}: {
  node: SomIssueTreeNode
  investigationId: string
  depth?: number
}) {
  const item = node.issue
  const [expanded, setExpanded] = useState(false)
  const [commentDraft, setCommentDraft] = useState('')
  const issues = useAppStore((s) => s.issues)
  const runEnrichment = useAppStore((s) => s.runEnrichment)
  const cancelIssue = useAppStore((s) => s.cancelIssue)
  const addComment = useAppStore((s) => s.addIssueComment)
  const entities = useAppStore((s) => s.entities)

  const issue = issues[item.id]
  const busy = issue?.status === 'running'
  const hasChildren = node.children.length > 0
  const hasDetails = Boolean(
    item.description?.trim() || issue?.description || issue
  )
  const canExpand = hasChildren || hasDetails

  return (
    <div
      className={clsx(
        'relative',
        depth > 0 && 'ml-3 border-l border-border/70 pl-3 mt-1.5'
      )}
    >
      <div className="rounded border border-border bg-surface-0 overflow-hidden shadow-xs">
        <div className="flex items-start gap-2 p-2.5">
          {canExpand ? (
            <button
              type="button"
              onClick={() => setExpanded(!expanded)}
              className="mt-0.5 text-fg-dim hover:text-fg p-0.5"
            >
              {expanded ? (
                <ChevronDown className="h-3.5 w-3.5" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5" />
              )}
            </button>
          ) : (
            <span className="w-4.5" />
          )}

          <div
            className={clsx('min-w-0 flex-1', canExpand && 'cursor-pointer')}
            onClick={canExpand ? () => setExpanded(!expanded) : undefined}
          >
            <div className="flex items-center gap-2">
              {issue && <StatusIcon status={issue.status} />}
              <span className="text-xs font-semibold text-fg truncate">
                {item.title}
              </span>
              {issue && (
                <Chip
                  tone={issue.status === 'completed' ? 'confirmed' : 'default'}
                >
                  {statusLabel[issue.status]}
                </Chip>
              )}
            </div>

            <div className="mt-0.5 flex items-center gap-2 text-[10px] text-fg-dim font-mono">
              <span>{item.simple_id}</span>
              {hasChildren && <span>· подзадач: {node.children.length}</span>}
              {issue?.createdAt && <span>· {formatTime(issue.createdAt)}</span>}
            </div>

            {issue && issue.entityIds.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {issue.entityIds.map((id) => (
                  <span
                    key={id}
                    className="rounded border border-border bg-surface-2 px-1 font-mono text-[9px] text-fg-muted"
                  >
                    {entities[id]?.label ?? id}
                  </span>
                ))}
              </div>
            )}

            {issue?.resultSummary && (
              <div className="mt-1 text-[11px] text-fg-dim">
                {issue.resultSummary}
              </div>
            )}
          </div>
        </div>

        <div className="flex gap-1 border-t border-border/60 bg-surface-1/40 px-2.5 py-1.5">
          <Button
            size="sm"
            className="flex-1 text-xs"
            disabled={busy}
            onClick={() => void runEnrichment(investigationId, item.id)}
          >
            {busy ? (
              <>
                <Loader2 className="h-3 w-3 animate-spin text-proposed" />{' '}
                Выполняется…
              </>
            ) : (
              <>
                <Play className="h-3 w-3" /> Запустить
              </>
            )}
          </Button>
          {busy && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => cancelIssue(item.id)}
            >
              <Square className="h-3 w-3" />
            </Button>
          )}
        </div>

        {expanded && hasDetails && (
          <div className="space-y-2 border-t border-border/80 bg-surface-2/30 p-2.5 text-xs">
            {(item.description?.trim() || issue?.description) && (
              <p className="whitespace-pre-wrap text-fg-muted">
                {item.description?.trim() || issue?.description}
              </p>
            )}

            {issue && (
              <div className="space-y-1.5 pt-1 border-t border-border/60">
                <div className="flex items-center gap-1 text-[10px] uppercase font-semibold tracking-wider text-fg-dim">
                  <MessageSquare className="h-3 w-3" />
                  Комментарии ({issue.comments.length})
                </div>
                {issue.comments.length > 0 && (
                  <div className="space-y-1.5">
                    {issue.comments.map((c) => (
                      <div
                        key={c.id}
                        className="rounded bg-surface-2 p-1.5 text-xs"
                      >
                        <div className="text-[10px] text-fg-dim">
                          {c.author} · {formatTime(c.time)}
                        </div>
                        <div className="mt-0.5 text-fg-muted">{c.text}</div>
                      </div>
                    ))}
                  </div>
                )}
                <form
                  className="flex gap-1 pt-1"
                  onSubmit={(e) => {
                    e.preventDefault()
                    if (!commentDraft.trim()) return
                    addComment(issue.id, commentDraft.trim())
                    setCommentDraft('')
                  }}
                >
                  <input
                    className="flex-1 rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none focus:border-fg/30 placeholder:text-fg-dim"
                    placeholder="Комментарий / обоснование…"
                    value={commentDraft}
                    onChange={(e) => setCommentDraft(e.target.value)}
                  />
                  <Button
                    size="sm"
                    type="submit"
                    disabled={!commentDraft.trim()}
                  >
                    →
                  </Button>
                </form>
              </div>
            )}
          </div>
        )}
      </div>

      {hasChildren && expanded && (
        <div className="space-y-1.5 mt-1.5">
          {node.children.map((child) => (
            <SomIssueTreeItem
              key={child.issue.id}
              node={child}
              investigationId={investigationId}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function buildSomIssueTree(issues: SomIssue[]): SomIssueTreeNode[] {
  const nodeMap = new Map<string, SomIssueTreeNode>()

  for (const issue of issues) {
    nodeMap.set(issue.id, { issue, children: [] })
  }

  const roots: SomIssueTreeNode[] = []

  for (const issue of issues) {
    const node = nodeMap.get(issue.id)!
    if (issue.parent_issue_id && nodeMap.has(issue.parent_issue_id)) {
      nodeMap.get(issue.parent_issue_id)!.children.push(node)
    } else {
      roots.push(node)
    }
  }

  return roots
}

/** Agent-proposed graph edges waiting for analyst accept/reject. */
function ProposedLinksSection({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const edgeReviews = useAppStore((s) => s.edgeReviews)
  const setReview = useAppStore((s) => s.setReview)
  const graphNodes = useAppStore((s) => s.graphNodes)
  const graphEdges = useAppStore((s) => s.graphEdges)

  if (!inv) return null

  const proposedEdges = inv.edgeIds
    .map((id) => graphEdges[id])
    .filter(Boolean)
    .filter((e) => (edgeReviews[e.id] ?? e.review) === 'proposed')

  if (proposedEdges.length === 0) return null

  const tree = buildEntityTree(proposedEdges, graphNodes)

  const handleAcceptBranch = (treeNode: ProposedTreeNode) => {
    const collectEdges = (node: ProposedTreeNode): string[] => [
      ...node.edges.map((e) => e.id),
      ...node.children.flatMap(collectEdges)
    ]
    const edgeIds = collectEdges(treeNode)
    edgeIds.forEach((id) => setReview('edge', id, 'confirmed', investigationId))
  }

  return (
    <div className="rounded border border-proposed/30 bg-surface-0 p-2">
      <div className="flex items-center justify-between border-b border-border pb-1.5 mb-2 px-1">
        <span className="text-[10px] uppercase font-semibold text-fg-dim">
          Дерево предложенных связей ({proposedEdges.length})
        </span>
      </div>

      <div className="max-h-80 overflow-y-auto space-y-1">
        {tree.map((rootNode) => (
          <TreeItem
            key={rootNode.id}
            item={rootNode}
            onAcceptEdge={(id) =>
              setReview('edge', id, 'confirmed', investigationId)
            }
            onRejectEdge={(id) =>
              setReview('edge', id, 'rejected', investigationId)
            }
            onAcceptBranch={handleAcceptBranch}
          />
        ))}
      </div>
    </div>
  )
}

export function AgentPanel({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const open = useAppStore((s) => s.agentPanelOpen)
  const setOpen = useAppStore((s) => s.setAgentPanelOpen)
  const catalog = useAppStore((s) => s.somCatalog)
  const loadSomCatalog = useAppStore((s) => s.loadSomCatalog)
  const createIssue = useAppStore((s) => s.createIssue)

  const [showCreate, setShowCreate] = useState(false)
  const [width, setWidth] = useState(() => {
    try {
      const saved = localStorage.getItem('ir.agentPanel.width')
      if (saved) {
        const parsed = Number(saved)
        if (parsed >= 280 && parsed <= 900) return parsed
      }
    } catch {
      /* ignore */
    }
    return 384 // 24rem (w-96)
  })

  const isDraggingRef = useRef(false)
  const dragStartRef = useRef({ startX: 0, startWidth: 384 })

  const handlePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    isDraggingRef.current = true
    dragStartRef.current = { startX: e.clientX, startWidth: width }
  }

  const handlePointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!isDraggingRef.current) return
    const delta = e.clientX - dragStartRef.current.startX
    const maxW = Math.min(900, window.innerWidth - 360)
    const nextWidth = Math.min(
      Math.max(280, dragStartRef.current.startWidth + delta),
      Math.max(280, maxW)
    )
    setWidth(nextWidth)
  }

  const handlePointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!isDraggingRef.current) return
    isDraggingRef.current = false
    try {
      e.currentTarget.releasePointerCapture(e.pointerId)
    } catch {
      /* ignore */
    }
    try {
      localStorage.setItem('ir.agentPanel.width', String(width))
    } catch {
      /* ignore */
    }
  }

  useEffect(() => {
    if (open) void loadSomCatalog()
  }, [open, loadSomCatalog])

  if (!inv || !open) return null

  const issueTree = useMemo(() => {
    return buildSomIssueTree(catalog?.issues ?? [])
  }, [catalog?.issues])

  return (
    <div
      className="relative z-10 flex h-full shrink-0 flex-col"
      style={{ width }}
    >
      <Panel
        title="ИИ-агент"
        side="left"
        className="w-full flex-1 min-w-0"
        actions={
          <div className="flex items-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setShowCreate((v) => !v)}
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
            <button type="button" onClick={() => setOpen(false)}>
              <X className="h-3.5 w-3.5 text-fg-dim" />
            </button>
          </div>
        }
      >
        <div className="space-y-3 p-3">
          <ProposedLinksSection investigationId={investigationId} />

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
                          : inv.entityIds.slice(0, 1)
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

          {issueTree.map((rootNode) => (
            <SomIssueTreeItem
              key={rootNode.issue.id}
              node={rootNode}
              investigationId={investigationId}
            />
          ))}

          {issueTree.length === 0 && (
            <div className="rounded border border-border p-4 text-center text-xs text-fg-dim">
              Нет задач на выбранной доске SOM
            </div>
          )}
        </div>
      </Panel>

      <div
        className="group absolute top-0 -right-1 z-20 h-full w-2 cursor-col-resize select-none touch-none flex items-center justify-center"
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        title="Потяните, чтобы изменить ширину"
      >
        <div className="h-8 w-1 rounded-full bg-border-strong/60 transition-colors group-hover:bg-proposed group-active:bg-proposed" />
      </div>
    </div>
  )
}
