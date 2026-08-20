# Investigation graph — integration

Self-contained Graph + Timeline UI used by `pt-workspace`. Copy this whole folder into another app.

## Contents

| Path | Role |
|---|---|
| `InvestigationGraph.tsx` | Toolbar + canvas + drawer + timeline |
| `GraphCanvas.tsx` / `Timeline.tsx` / `GraphToolbar.tsx` / `GraphDetailsDrawer.tsx` | UI pieces |
| `nodes/` | React Flow node renderers |
| `graph-adapters.ts` | Visible-graph / timeline filtering |
| `types.ts` / `constants.ts` / `time.ts` | Domain model + helpers |
| `graph.css` | Theme tokens + React Flow overrides |

## Dependencies

```bash
npm i @xyflow/react lucide-react react react-dom
```

Also needs **Tailwind CSS** (utility classes in the components) and React 18+.

## Wire-up

1. Import styles once in the host app:

```ts
import './components/graph/graph.css'
```

2. Provide investigation state. Components currently call:

```ts
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
```

Point that import at your store (or a thin adapter). Required surface:

- `activeInvestigation` — see `GraphInvestigation` in `types.ts`
- `selection`, `hoverEventId`
- `select`, `setHoverEvent`, `setTimeRange`
- `toggleEntityType`, `toggleSeverity`, `toggleEdgeOrigin`, `resetGraphFilters`
- `expandRelated`, `collapseRelated`, `canExpand`, `isExpanded`
- `updateNodePosition`, `arrangeNodes`

3. Render:

```tsx
import { InvestigationGraph } from './components/graph'

<div className="h-full">
  <InvestigationGraph />
</div>
```

Or compose pieces yourself (`GraphToolbar` + `GraphCanvas` + `GraphDetailsDrawer` + `Timeline`) — see `ContextExpand.tsx` in this mock.

## Data shape

Feed `activeInvestigation` with `entities`, `alerts`, `edges`, `events`, time window, and `filters`. Positions live on each entity/alert (`position: { x, y }`). Each entity, alert, and edge has `origin: 'agent' | 'analyst'`; the toolbar origin chips hide both nodes and links. Expand/collapse is host-owned (this UI only calls the callbacks).
