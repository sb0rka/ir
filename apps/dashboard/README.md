# SOC Analyst Workspace

Frontend of the Sb0rka IR analyst workspace. Queue, investigation graph/timeline, and the agent panel talk to live Gateway (`:8091`) and ir-api (`:8090`).

## Run

```bash
cp .env.sample .env   # already gitignored
# fill VITE_SOM_TOKEN if you need the Agent panel
npm install
npm run dev
```

`VITE_IR_URL` and `VITE_GATEWAY_URL` are bare origins. The client appends `/api/v1` for ir-api; Gateway paths already include it.

SOM JWT lasts about an hour. Vite inlines `VITE_SOM_TOKEN` at startup — paste a fresh token into the header field to override it without rebuilding.

## Walkthrough

1. **Очередь** — Gateway `POST /events/search`. Default window is 30 days so the `impacket_smbexec` demo (23 July) is visible. Start an investigation from a row.
2. **Граф + таймлайн** — `GET /investigations/{id}/graph` and `/events`. Node positions are computed locally and stored in `localStorage`.
3. **Насыщение контекста** — `POST /som/issues/{id}/run`, then poll for agent-proposed edges. Needs a valid SOM token.
4. **Ревью связей** — `POST /investigations/{id}/review`. ir-api currently returns 501; the UI shows that instead of faking success.
5. **Детали** — reputation (`/entities/lookup`), sandbox (`/artifact-analyses`), related (`/entities/{id}`).

## Stack

Vite · React · TypeScript · Tailwind CSS v4 · Zustand · openapi-fetch · `@ir/contract` · React Flow via `src/components/graph/`
