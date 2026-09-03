import { useMemo } from 'react'
import {
  EMPTY_LAYER_IDS,
  graphLayerNodeSets,
  layerItemIds,
} from '../../lib/hypotheses'
import { useAppStore } from '../../store/appStore'
import type { GraphVisibility } from './graph-adapters'

export function useHypothesisGraphView(
  investigationId: string | undefined,
  allNodeIds: string[],
): GraphVisibility {
  const visibleItemIds = useAppStore((s) =>
    investigationId ? s.visibleHypothesisIds[investigationId] : undefined,
  )
  const highlightedItemIds = useAppStore((s) =>
    investigationId ? s.highlightedHypothesisIds[investigationId] : undefined,
  )
  const hypothesisIds = useAppStore((s) =>
    investigationId ? s.investigations[investigationId]?.hypothesisIds : undefined,
  )
  const hypotheses = useAppStore((s) => s.hypotheses)
  const membership = useAppStore((s) => s.hypothesisMembership)
  const activeHypothesisId = useAppStore((s) =>
    investigationId ? (s.activeHypothesisId[investigationId] ?? null) : null,
  )

  return useMemo(() => {
    const ids = hypothesisIds ?? EMPTY_LAYER_IDS
    const layers = graphLayerNodeSets({
      visibleItemIds,
      highlightedItemIds: highlightedItemIds ?? EMPTY_LAYER_IDS,
      allItemIds: layerItemIds(ids),
      allNodeIds,
      hypothesisIds: ids,
      hypotheses,
      membership,
    })
    return {
      ...layers,
      activeNodeIds: new Set(
        activeHypothesisId ? (membership[activeHypothesisId]?.nodeIds ?? EMPTY_LAYER_IDS) : EMPTY_LAYER_IDS,
      ),
      writable: Boolean(
        activeHypothesisId && hypotheses[activeHypothesisId]?.status !== 'resolved',
      ),
    }
  }, [
    visibleItemIds,
    highlightedItemIds,
    hypothesisIds,
    allNodeIds,
    hypotheses,
    membership,
    activeHypothesisId,
  ])
}
