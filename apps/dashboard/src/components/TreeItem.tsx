import { useState } from "react";
import { ChevronDown, ChevronRight, Check, X } from "lucide-react";
import type { ProposedTreeNode } from "../lib/tree-bundler";

export function TreeItem({
  item,
  onAcceptEdge,
  onRejectEdge,
  onAcceptBranch,
}: {
  item: ProposedTreeNode;
  onAcceptEdge: (edgeId: string) => void;
  onRejectEdge: (edgeId: string) => void;
  onAcceptBranch: (item: ProposedTreeNode) => void;
}) {
  const [expanded, setExpanded] = useState(true);
  const hasChildren = item.children.length > 0 || item.edges.length > 0;

  return (
    <div className="relative text-xs">
      <div className="group flex items-center justify-between rounded px-2 py-1.5 hover:bg-surface-2">
        <div className="flex items-center gap-1.5 min-w-0">
          {hasChildren ? (
            <button
              onClick={() => setExpanded(!expanded)}
              className="text-fg-dim hover:text-fg p-0.5"
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
          <span className="font-mono font-medium truncate text-fg">
            {item.node.label}
          </span>
          <span className="text-[10px] text-fg-dim font-mono uppercase">
            ({item.node.kind})
          </span>
        </div>

        {hasChildren && (
          <button
            onClick={() => onAcceptBranch(item)}
            className="hidden group-hover:inline-flex items-center gap-1 text-[10px] text-confirmed hover:underline"
            title="Принять всю ветку"
          >
            <Check className="h-3 w-3" /> Всю ветку
          </button>
        )}
      </div>

      {expanded && (
        <div className="ml-3 border-l border-border/80 pl-3 space-y-1.5 mt-1">
          {item.edges.map((edge) => (
            <div
              key={edge.id}
              className="rounded bg-surface-2/40 p-1.5 border border-border/40"
            >
              <div className="flex items-center justify-between gap-1">
                <span className="text-[11px] text-proposed">
                  ──{edge.relation}──►
                </span>
                <div className="flex gap-1">
                  <button
                    onClick={() => onAcceptEdge(edge.id)}
                    className="hover:text-confirmed p-0.5"
                  >
                    <Check className="h-3 w-3 text-confirmed" />
                  </button>
                  <button
                    onClick={() => onRejectEdge(edge.id)}
                    className="hover:text-critical p-0.5"
                  >
                    <X className="h-3 w-3 text-critical" />
                  </button>
                </div>
              </div>
              {edge.rationale && (
                <p className="mt-0.5 text-[10px] text-fg-dim leading-tight">
                  {edge.rationale}
                </p>
              )}
            </div>
          ))}

          {item.children.map((child) => (
            <TreeItem
              key={child.id}
              item={child}
              onAcceptEdge={onAcceptEdge}
              onRejectEdge={onRejectEdge}
              onAcceptBranch={onAcceptBranch}
            />
          ))}
        </div>
      )}
    </div>
  );
}
