# SOC Analyst Workspace

Frontend of the Sb0rka IR analyst workspace. Queue, investigation graph/timeline, and the agent panel talk to live Gateway (`:8091`) and ir-api (`:8090`).

## Run

```bash
cp .env.sample .env   # already gitignored
npm install
npm run dev
```

`VITE_AUTH_BASE_URL`, `VITE_PLATFORM_API_BASE_URL`, `VITE_IR_URL` and
`VITE_GATEWAY_URL` are bare origins. Sign in with a Sb0rka user; the access JWT
is refreshed through the Auth refresh cookie. Project-scoped SOM/PT credentials
are managed as versioned Sb0rka Secrets in the Configuration window. SOM
workspace/board selectors live in project-scoped `sessionStorage`; SOM variant
and model settings live in project-scoped `localStorage`.

## Walkthrough

1. **Очередь** — Gateway `POST /events/search`. Default window is 30 days so the `impacket_smbexec` demo (23 July) is visible. Start an investigation from a row.
2. **Граф + таймлайн** — `GET /investigations/{id}/graph` and `/events`. Node positions are computed locally and stored in `localStorage`.
3. **Насыщение контекста** — `POST /som/issues/{id}/run`, then poll for agent-proposed edges. Requires `DEMO_SOM_ACCESS_TOKEN` in the selected project.
4. **Ревью связей** — `POST /investigations/{id}/review`. ir-api currently returns 501; the UI shows that instead of faking success.
5. **Детали** — reputation (`/entities/lookup`), sandbox (`/artifact-analyses`), related (`/entities/{id}`).

## Stack

Vite · React · TypeScript · Tailwind CSS v4 · Zustand · openapi-fetch · `@ir/contract` · React Flow via `src/components/graph/`
