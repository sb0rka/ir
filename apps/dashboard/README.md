# SOC Analyst Workspace — Prototype v1

Clickable frontend prototype of the Sb0rka IR analyst workspace. Built from `docs/analyst-path.md` and `docs/frontend/ui.md`. All data is mocked — no API.

## Run

```bash
cd frontend_ideas/v1
npm install
npm run dev
```

## Walkthrough

1. **Очередь** — filter with chip bar (field → value), expand correlation «Компрометация WS-1042», start investigation.
2. **Граф + таймлайн** — inspect process chain; brush the timeline.
3. **Насыщение контекста** — runs a mock AI task (~2.5s) that adds proposed nodes/edges (SRV-DC01 lateral movement). Accept/reject on the graph.
4. **Детали** — entity actions (reputation, sandbox, decode) return simulated results.
5. **Дочернее расследование** — select entities → create child tab with parent link.

## Stack

Vite · React · TypeScript · Tailwind CSS v4 · Zustand · React Flow via `src/components/graph/` (`InvestigationGraph` + Timeline)
