# Вход и полномочия IR

[English](auth_en.md)

Dashboard обращается только к `ir-api`. Backend завершает OIDC Authorization Code flow с PKCE S256, nonce и одноразовым browser handoff. Auth выдаёт пользовательский `at+jwt` с `client_id=sb0rka-ir-web`, `aud=sb0rka-ir`; IR не подписывает JWT и не получает signing keys.

## Сессия

Access JWT хранится только в памяти браузера. Локальный refresh handle на 256 бит передаётся в host-only HttpOnly cookie: `__Host-ir_refresh` с Secure в production, `ir_refresh` в явном HTTP dev-режиме; SameSite=Lax. Origin POST-запросов должен точно совпадать с `IR_WEB_URL`. Web и API должны находиться на одном site для этой cookie policy.

`ir_login_sessions` связывает локальную сессию с Auth `sub/sid`, хранит hash handle, профиль и AEAD-зашифрованные Auth tokens. Auth refresh token браузеру не выдаётся. TTL — минимум семи дней и абсолютного срока Auth-сессии. `sid/auth_time` не меняются при refresh.

Refresh резервирует операцию короткой транзакцией, затем вызывает Auth вне SQL-транзакции и сохраняет результат через проверку generation/operation и состояния сессии. Lease координирует разные экземпляры; Idempotency-Key определяется прежним Auth refresh token. Повтор операции не ротирует его второй раз.

Каждый browser refresh ротирует локальный handle. Предыдущий handle в течение 30 секунд возвращает тот же новый handle; обнаруженный повтор после этого окна отзывает соответствующую сессию. Абсолютный срок Auth-сессии не продлевается.

`ir_login_requests` хранит hash state, зашифрованный context с PKCE/nonce, одноразовое потребление callback и handoff. Login callback живёт максимум десять минут; готовый handoff — до 60 секунд и требует verifier исходного браузера. Cookie и токены не передаются через URL; URL содержит только одноразовый handoff.

Logout сразу отзывает локальную сессию и транзакционно ставит Auth revocation в `ir_auth_revocations`. Отзыв текущей Auth-сессии имеет короткий backchannel timeout; durable worker повторяет неудачные операции. Очередь другого пользователя не блокирует logout. Consent при logout сохраняется.

## Consent и проекты

Вход сам по себе не создаёт grant. Экран подключений предлагает независимые разрешения:

| Интеграция | Аудитория | Scopes |
|---|---|---|
| Platform API | `sb0rka-api` | `projects.read`, `resources.read`, `secrets.read`, `secrets.reveal`, `secrets.create`, `secrets.write` |
| IR Gateway | `sb0rka-ir-gateway` | `ir.gateway.use` |

Auth показывает проекты текущего пользователя; callback привязан к инициирующей локальной сессии и PKCE. Grant принадлежит пользователю, клиенту и аудитории и сохраняется между login-сессиями. Disconnect отзывает соответствующий grant во всех сессиях, не завершает вход и не затрагивает другую интеграцию.

Для каждого пользовательского project-scoped REST/MCP запроса IR проверяет access JWT, активную локальную/Auth-сессию и выбранный проект. `X-Project-ID` не предоставляет доступ: нужен API consent, проект в grant и успешная проверка текущего пользовательского RBAC через API. Отсутствующий/чужой проект возвращает `404`.

Delegated JWT кешируется по локальной сессии, аудитории, grant/version, scopes и проектам. Текущее состояние grant читается из Auth. API и Gateway проверяют отзыв независимо, поэтому кеш не продлевает полномочия.

Пользовательский Gateway вызывается через `/api/v1/gateway/…` с фиксированным upstream и allowlist документированных операций. Backend заменяет Authorization, `X-IR-Config-Token` и project context, удаляет browser cookies и Origin. Внутренние REST/MCP Gateway clients используют тот же обмен для пользовательских запросов. Gateway проверяет отдельный delegated JWT и выполняет аудиториально ограниченную introspection на каждом запросе. Второй JWT имеет аудиторию API и ровно `secrets.read` и `secrets.reveal`; пользователь, сессия, клиент и actor должны совпадать в обоих JWT. Platform API проверяет его grant и действующее право reveal при каждом пользовательском Gateway-запросе.

## Доступ агента

В конфигурации проекта пользователь создаёт ключ или ротирует его. IR записывает 64-символьный `s0_ir_agent_…` в текстовый секрет `IR/AGENT_ACCESS` через обычные Platform Secrets routes. Генерация требует пользовательской сессии, project consent и прав read/reveal/create/write. Исходный ключ возвращается один раз; позднее он доступен только в интерфейсе Platform Secrets. IR хранит активный SHA-256, UUID/версию секрета и маску. Ротация создаёт новую Platform-версию и атомарно заменяет активный хеш; прежние версии не авторизуют запросы.

Агент передаёт ключ в `Authorization: Bearer` и проект в `X-Project-ID`. Доменные REST и `/mcp` проверяют хеш и проект в БД IR на каждом запросе. Запрос с ключом другого проекта или старым ключом получает `401`, недоступность БД — `503`. Управление ключами, конфигурацией и OIDC требует пользовательского JWT.

IR передаёт тот же Bearer в Gateway, без `X-IR-Config-Token`. Gateway независимо проверяет активный хеш и расшифровывает проектную копию конфигурации источников. Агентский путь не использует пользовательскую сессию, refresh/exchange, introspection или Platform reveal и работает после logout и перезапуска процессов.

Platform Secrets остаётся основным хранилищем конфигурации. AES-256-GCM-копия в БД IR обновляется при сохранении или «Перечитать»; прямые изменения в Platform нужно перечитать через IR. Plaintext ограничен одним запросом. `IR_GATEWAY_CONFIGURATION_KEY` одинаков в IR API и Gateway и отличается от ключей сессии/OIDC. В SoM ключ вручную задаётся как `SB0RKA_IR_AGENT_TOKEN` в env исполнителя, карточка MCP «Sb0rka IR» ссылается на эту переменную. После ротации нужно обновить env и перезапустить исполнитель/MCP-подключение.

## Интерфейсы

Источник HTTP-контракта — `api/investigations/paths/authentication.yaml`; Go/TS типы генерируются `task gen`. Authentication HTTP transport обслуживает cookies и redirects отдельно от business Server; оба интерфейса собираются в `transport.API`.

- `/api/v1/auth/oidc/{init,callback,redeem}`, `/api/v1/auth/{refresh,logout,me}`;
- `/api/v1/integrations/{platform|gateway}/{connect,callback,status,disconnect}`;
- `/api/v1/platform/projects`, `/api/v1/configuration`, `/api/v1/gateway/…`.
- `GET/POST /api/v1/agent-access`, `POST /api/v1/agent-access/rotate` с `expected_version`; устаревшая версия возвращает `409`.

Swagger доступен на `/swagger`, полная спецификация — `/openapi.json`. Невалидные/истёкшие удостоверения дают `401`, отсутствие consent — `403`, недоступность необходимых ключей/авторизации — `503`.

## Конфигурация и запуск

1. В соседнем `sb0rka` подготовить Auth manifest; `bash deploy/dev/generate-client-secrets.sh` записывает отсутствующие dev credentials в `apps/auth/.env`. Не менять `sb0rka-som-*` IDs/principals. Те же значения `SB0RKA_IR_WEB_CLIENT_SECRET` и `SB0RKA_IR_GATEWAY_CLIENT_SECRET` скопировать в `apps/investigations/.env` и `apps/gateway/.env`.
2. В `apps/investigations/.env` задать разные `IR_SESSION_ENCRYPTION_KEY`, `IR_OIDC_CONTEXT_ENCRYPTION_KEY` и `IR_GATEWAY_CONFIGURATION_KEY`: `head -c 32 /dev/urandom | base64 -w0`. Последний ключ передать также Gateway вместе с `DATABASE_URI` существующей БД IR. Signing keys здесь не нужны.
3. Настроить `apps/investigations/.env` и `apps/gateway/.env` по их `.env.sample`: один canonical `SB0RKA_AUTH_BASE_URL`, exact registered callback URLs относительно `IR_PUBLIC_BASE_URL`, `IR_WEB_URL` и явный CORS origin. Compose читает эти файлы из каталогов приложений. Native dev использует loopback или private-IP origins и `AUTH_ALLOW_INSECURE_HTTP_ISSUER=true`. HTTP также принимается для `host.docker.internal` и одноуровневого имени сервиса Compose. Production — HTTPS. Dev dashboard слушает `3400`, preview — `3300`, Auth Console — `5173`.
4. Для новой установки подготовить Auth schema `010-initial_auth_schema.sql` и client seed `011-initial_auth_clients.sql`. Применить адресный policy seed `012-ir_secret_permissions.sql`, в IR — миграции `011-auth_sessions.sql` и `012-agent-access.sql` в схеме `inv`. Ротация refresh handles входит в `011-auth_sessions.sql`. Бизнес-данные и существующие OIDC-сессии сохраняются.
5. Собирать с актуальным shared core: `go.work` включает соседний `../sb0rka/packages/core`; Dockerfiles используют отдельный build context `core`. Установка готового публичного пакета старой версии вместо текущего core не поддерживает новый контракт.

JSON-конфигурация `IR/SOURCE_CONFIGURATION`, новые scopes и передача credentials Gateway описаны в [configuration.md](configuration.md). Legacy SoM routes не регистрируются; SecretResolver нового Gateway читает только проектный JSON. IR → SoM bootstrap и workload scopes в этот этап не входят.

## Проверки

Текущий этап: сборка, `go vet`, целевые Go-тесты IR с `-race`, frontend-проверки и HTTP/browser smoke из `vuln_check`. Правила проверок соседнего sb0rka сохраняются. Для контрактов: `task gen` и проверка повторной генерации; для frontend — lint/typecheck/build. Сценарии: оба OIDC-клиента, callback replay, stable sid, refresh concurrency/replay, отдельные consent, project/RBAC denial, grant/client revocation и Gateway introspection outage.
