# IR authentication and permissions

[Русский](auth.md)

Dashboard calls only ir-api. The backend completes OIDC Authorization Code with PKCE S256, nonce and a one-use browser handoff. Auth issues user `at+jwt` tokens for `sb0rka-ir` with client ID `sb0rka-ir-web`. IR never signs tokens or receives signing keys.

Access tokens remain in browser memory. A 256-bit local refresh handle uses a host-only HttpOnly SameSite=Lax cookie: `__Host-ir_refresh` with Secure in production, `ir_refresh` in explicit HTTP development mode. POST Origin must match `IR_WEB_URL`; web and API must share a site for this cookie policy.

`ir_login_sessions` stores Auth sub/sid, a hashed handle, profile and AEAD-encrypted Auth tokens. Auth refresh credentials never reach the browser. Session expiry is the earlier of seven days and Auth's absolute deadline. Refresh preserves sid/auth_time and uses a database reservation, HTTP outside transactions, generation/operation checks and a stable Auth idempotency key. Each browser refresh rotates its local handle; the previous handle returns the same successor for 30 seconds, then a detected replay revokes that session. `ir_login_requests` stores encrypted PKCE/nonce context and single-use callbacks/handoffs. Ready handoffs expire within 60 seconds and require the initiating browser's verifier.

Logout revokes the local session and queues Auth revocation atomically. A short attempt handles the current session; a durable worker retries failures without making logout wait on another user's backlog. Logout preserves consent.

## Explicit integration consent

API access (`projects.read`, `resources.read`, `secrets.read`, `secrets.reveal`, `secrets.create`, `secrets.write`, audience `sb0rka-api`) and Gateway access (`ir.gateway.use`, audience `sb0rka-ir-gateway`) are separate grants. Login creates neither. Grants are user/client/audience-bound and survive logins. Existing grants require explicit consent for the new configuration scopes. Disconnect revokes one integration across user sessions and preserves login and the other integration.

Every user project-scoped REST/MCP request validates the JWT and local/Auth session, the consented project and current user RBAC through Platform API. `X-Project-ID` selects a project; it never grants access. Missing/foreign projects return 404. Delegated-token caches include session, audience, grant/version, scopes and projects; resource servers independently check live authorization.

Dashboard reaches Gateway through the fixed-upstream, allowlisted `/api/v1/gateway/…` proxy. It replaces Authorization, `X-IR-Config-Token` and project context and strips browser cookies and Origin. Internal Gateway clients use the same exchange for user requests. Gateway checks its dedicated delegated-token audience and authenticated, uncached live introspection. The second JWT has API audience and exactly `secrets.read` and `secrets.reveal`; its user/session/client/actor must match the Gateway token. Platform API checks its grant and current reveal permission on every user Gateway request.

## Project agent access

Project configuration lets a user create or rotate a 64-character `s0_ir_agent_…` key. IR writes it as the ordinary text secret `IR/AGENT_ACCESS` in Platform Secrets. Issuance requires the user session, project consent and read/reveal/create/write permissions. The issuance response shows the key once; subsequent access to its value uses the Platform Secrets interface. IR stores only the active SHA-256 digest, secret UUID/version and masked prefix/postfix. Rotation appends a Platform secret version and atomically replaces the active digest; older secret versions no longer authorize requests.

Agents send `Authorization: Bearer` and `X-Project-ID`. Domain REST and `/mcp` verify the active digest and project against the IR database on each request. Invalid, rotated or foreign-project keys receive `401`; database outages receive `503`. Key management, configuration and OIDC remain user-only. The management endpoints are `GET/POST /api/v1/agent-access` and `POST /api/v1/agent-access/rotate`; rotation requires `expected_version` and returns `409` for a stale version.

IR forwards the same agent Bearer to Gateway without `X-IR-Config-Token`. Gateway independently verifies it and decrypts the project source configuration. Agent requests make no user-session, refresh/exchange, introspection or Platform reveal calls and survive user logout and process restarts. Platform Secrets remains the source of truth; saving or reloading through IR synchronizes its AES-256-GCM copy in the IR database. Direct Platform edits require a reload through IR. Plaintext is confined to one request.

IR API and Gateway share `IR_GATEWAY_CONFIGURATION_KEY`, independent of the session/OIDC keys, and use the existing IR database. In SoM, set `SB0RKA_IR_AGENT_TOKEN` in the executor environment and add the “Sb0rka IR” MCP card. After rotation, update the environment and restart the executor/MCP connection.

## Deployment and contracts

The source contract is `api/investigations/paths/authentication.yaml`; `task gen` generates Go/TS types. HTTP authentication handles cookies/redirects separately from the business Server; both compose into `transport.API`.

Configure the shared Auth client manifest. Copy `SB0RKA_IR_WEB_CLIENT_SECRET` and `SB0RKA_IR_GATEWAY_CLIENT_SECRET` from Auth into `apps/investigations/.env` and `apps/gateway/.env`. Set distinct `IR_SESSION_ENCRYPTION_KEY`, `IR_OIDC_CONTEXT_ENCRYPTION_KEY` and `IR_GATEWAY_CONFIGURATION_KEY` with `head -c 32 /dev/urandom | base64 -w0`. Supply the last key and the existing IR `DATABASE_URI` to Gateway as well. Copy `apps/investigations/.env.sample` and `apps/gateway/.env.sample` to `.env` beside them; Compose reads those files. Set the canonical issuer, exact callback URLs under `IR_PUBLIC_BASE_URL`, the web URL and explicit CORS origins. Native development uses loopback or private-IP HTTP and the explicit dev flag. HTTP is also accepted for `host.docker.internal` and a single-label Compose service name. Production uses HTTPS. Dashboard development port is 3400, preview is 3300, and Auth Console uses 5173.

For a new installation, apply the Auth schema `010-initial_auth_schema.sql` and client seed `011-initial_auth_clients.sql`. Apply the targeted policy seed `012-ir_secret_permissions.sql` and IR migrations `011-auth_sessions.sql` and `012-agent-access.sql`. Refresh-handle rotation is included in `011-auth_sessions.sql`. Existing investigations and SOM identities/sessions/grants are preserved; existing OIDC sessions remain compatible. `go.work` consumes the sibling shared core checkout; Docker builds use its dedicated `core` context.

Configuration settings edit the project's single versioned `IR/SOURCE_CONFIGURATION` JSON secret, tagged `source:ir`. The document is an array of sources with unique names and string credential fields. IR uses ordinary Secrets routes with generic delegated scopes and existing user RBAC. sb0rka validates JSON syntax and stores the original text; the source schema and cookie interpretation belong to IR. Gateway resolves known cookie fields from one snapshot per request, with no plaintext cache between requests. Legacy SOM routes are not registered; IR receives no SOM bootstrap or workload privileges. IR → SOM remains outside this stage.

Run IR Go builds, vet and race tests, repeatable contract generation, frontend lint/typecheck/build and the HTTP/browser flow in `vuln_check`. Follow the separate testing rules of the sibling sb0rka repository. Acceptance covers JSON versions/concurrency/validation, project/RBAC isolation, local refresh concurrency/replay, separate consent/revocation, matching Gateway tokens and Auth/API failures. See [configuration.md](configuration.md) for configuration and VPN-free startup, and [the implementation report](../vuln_check/IMPLEMENTATION_REPORT.md) for verified results and limits.
