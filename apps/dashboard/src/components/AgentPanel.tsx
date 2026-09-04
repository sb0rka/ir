import { useEffect, useMemo, useState } from 'react'
import { useAppStore } from '../store/appStore'
import { Button, Chip } from './ui'
import { clsx, formatTime, statusLabel } from '../lib/utils'
import {
  CheckCircle2,
  CircleDashed,
  Loader2,
  MessageSquare,
  Play,
  Square,
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

function containsIssue(node: SomIssueTreeNode, issueId: string): boolean {
  if (node.issue.id === issueId) return true
  return node.children.some((child) => containsIssue(child, issueId))
}

function SomIssueTreeItem({
  node,
  investigationId,
  expandedId,
  confirmIssueId,
  onToggleExpand,
  onConfirmIssue,
  depth = 0,
}: {
  node: SomIssueTreeNode
  investigationId: string
  expandedId: string | null
  confirmIssueId: string | null
  onToggleExpand: (issueId: string) => void
  onConfirmIssue: (issueId: string | null) => void
  depth?: number
}) {
  const item = node.issue
  const [commentDraft, setCommentDraft] = useState('')
  const issues = useAppStore((s) => s.issues)
  const runEnrichment = useAppStore((s) => s.runEnrichment)
  const cancelIssue = useAppStore((s) => s.cancelIssue)
  const addComment = useAppStore((s) => s.addIssueComment)

  const issue = issues[item.id]
  const busy = issue?.status === 'running'
  const hasChildren = node.children.length > 0
  const selected = expandedId === item.id
  const awaitingConfirm = confirmIssueId === item.id
  const cta = selected || awaitingConfirm
  const onActivePath = expandedId != null && containsIssue(node, expandedId)
  const showChildren = hasChildren && onActivePath
  const description = item.description?.trim() || issue?.description
  const showDescription = onActivePath && Boolean(description)
  const showComments = selected && Boolean(issue)

  const handleRunClick = () => {
    if (busy) return
    if (selected) {
      void runEnrichment(investigationId, item.id)
      return
    }
    if (awaitingConfirm) {
      onConfirmIssue(null)
      void runEnrichment(investigationId, item.id)
      return
    }
    onConfirmIssue(item.id)
  }

  return (
    <div
      className={clsx(
        'relative',
        depth > 0 && 'ml-3 mt-1.5 border-l border-border/70 pl-3',
      )}
    >
      <div
        className={clsx(
          'overflow-hidden rounded border shadow-xs',
          onActivePath
            ? 'border-accent bg-accent/10 ring-1 ring-accent/50'
            : 'border-border bg-surface-0',
        )}
      >
        <button
          type="button"
          className="w-full cursor-pointer p-2.5 text-left"
          onClick={() => onToggleExpand(item.id)}
        >
          <div className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-xs font-semibold text-fg">
              {item.title}
            </span>
            {issue && (
              <Chip tone={issue.status === 'completed' ? 'confirmed' : 'default'}>
                <StatusIcon status={issue.status} />
                {statusLabel[issue.status]}
              </Chip>
            )}
          </div>

          <div className="mt-0.5 flex items-center gap-2 font-mono text-xs text-fg-dim">
            <span>{item.simple_id}</span>
            {hasChildren && <span>· подзадач: {node.children.length}</span>}
            {issue?.createdAt && (
              <span className="ml-auto">· {formatTime(issue.createdAt)}</span>
            )}
          </div>

          {issue?.resultSummary && (
            <div className="mt-1 text-xs text-fg-dim">{issue.resultSummary}</div>
          )}
        </button>

        <div className="flex gap-1 border-t border-border/60 px-2.5 py-1.5">
          <div
            className="min-w-0 flex-1"
            data-som-confirm={awaitingConfirm ? item.id : undefined}
          >
            <Button
              size="sm"
              className="w-full"
              variant={cta ? 'primary' : 'default'}
              disabled={busy}
              onClick={handleRunClick}
            >
              {busy ? (
                <>
                  <Loader2 className="h-3 w-3 animate-spin text-proposed" />
                  Выполняется…
                </>
              ) : awaitingConfirm ? (
                'Подтвердить?'
              ) : (
                <>
                  <Play className="h-3 w-3" />
                  Запустить
                </>
              )}
            </Button>
          </div>
          {busy && (
            <Button
              size="sm"
              title="Остановить"
              aria-label="Остановить"
              onClick={() => cancelIssue(item.id)}
            >
              <Square className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>

        {(showDescription || showComments) && (
          <div className="space-y-2 border-t border-border/80 bg-surface-2/30 p-2.5 text-xs">
            {showDescription && (
              <p className="max-h-64 overflow-y-auto whitespace-pre-wrap text-fg-muted">
                {description}
              </p>
            )}

            {showComments && issue && (
              <div className="space-y-1.5 border-t border-border/60 pt-1">
                <div className="flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wider text-fg-dim">
                  <MessageSquare className="h-3 w-3" />
                  Комментарии ({issue.comments.length})
                </div>
                {issue.comments.length > 0 && (
                  <div className="max-h-48 space-y-1.5 overflow-y-auto">
                    {issue.comments.map((c) => (
                      <div key={c.id} className="rounded bg-surface-2 p-1.5 text-xs">
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
                    className="flex-1 rounded border border-border bg-surface-0 px-2 py-1 text-xs outline-none placeholder:text-fg-dim focus:border-fg/30"
                    placeholder="Комментарий / обоснование…"
                    value={commentDraft}
                    onChange={(e) => setCommentDraft(e.target.value)}
                  />
                  <Button size="sm" type="submit" disabled={!commentDraft.trim()}>
                    →
                  </Button>
                </form>
              </div>
            )}
          </div>
        )}
      </div>

      {showChildren && (
        <div className="mt-1.5 space-y-1.5">
          {node.children.map((child) => (
            <SomIssueTreeItem
              key={child.issue.id}
              node={child}
              investigationId={investigationId}
              expandedId={expandedId}
              confirmIssueId={confirmIssueId}
              onToggleExpand={onToggleExpand}
              onConfirmIssue={onConfirmIssue}
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

export function AgentSection({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const catalog = useAppStore((s) => s.somCatalog)
  const loadSomCatalog = useAppStore((s) => s.loadSomCatalog)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [confirmIssueId, setConfirmIssueId] = useState<string | null>(null)

  useEffect(() => {
    void loadSomCatalog()
  }, [loadSomCatalog])

  useEffect(() => {
    if (!confirmIssueId) return
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target
      if (
        target instanceof Element &&
        target.closest(`[data-som-confirm="${confirmIssueId}"]`)
      ) {
        return
      }
      setConfirmIssueId(null)
    }
    const timer = window.setTimeout(() => {
      document.addEventListener('pointerdown', onPointerDown)
    }, 0)
    return () => {
      window.clearTimeout(timer)
      document.removeEventListener('pointerdown', onPointerDown)
    }
  }, [confirmIssueId])

  const issueTree = useMemo(() => {
    return buildSomIssueTree(catalog?.issues ?? [])
  }, [catalog?.issues])

  if (!inv) return null

  return (
    <div className="space-y-3 p-3">
      <ProposedLinksSection investigationId={investigationId} />

      {issueTree.map((rootNode) => (
        <SomIssueTreeItem
          key={rootNode.issue.id}
          node={rootNode}
          investigationId={investigationId}
          expandedId={expandedId}
          confirmIssueId={confirmIssueId}
          onConfirmIssue={setConfirmIssueId}
          onToggleExpand={(issueId) => {
            setConfirmIssueId(null)
            setExpandedId((current) => (current === issueId ? null : issueId))
          }}
        />
      ))}

      {issueTree.length === 0 && (
        <div className="rounded border border-border p-4 text-center text-xs text-fg-dim">
          Нет задач на выбранной доске SOM
        </div>
      )}
    </div>
  )
}
