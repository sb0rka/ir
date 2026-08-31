Incident Response — Единая картина инцидента и AI-автоматизация расследований.

Русский | [English](README_EN.md)

---

[Сайт Sb0rka IR](https://ir.sb0rka.ru) | [Документация](https://docs.sb0rka.com/ru)

## Состав репозитория

- `api/`: OpenAPI-контракты по сервисам
- `apps/gateway/`: Gateway внешних инструментов безопасности
- `apps/investigations/`: Go API сервис `ir-api`
- `db/migrations/`: схема `inv` и справочники
- `packages/contract/`: сгенерированный Go-контракт
- `packages/contract-ts/`: сгенерированный TypeScript-контракт и клиент
- `packages/common/`: общий клиент project-scoped секретов Sb0rka

## Команды

Требуются [Task](https://taskfile.dev/) и POSIX-утилиты (`sh`, `rm`, `perl`).

```bash
task spec       # Собрать и проверить OpenAPI
task gen        # Обновить Go/TypeScript-контракты и заглушки
task build      # Собрать bin/ir-api и bin/gateway
task check      # Генерация, сборка и go vet
task db:up      # Локальный Postgres
task db:migrate # Миграции
task apps:up    # ir-api и gateway
task apps:down
```

Сгенерированные файлы коммитятся, но вручную не редактируются. Источник правды
для API — файлы в `api/<service>/`.

## Локальный запуск

```bash
# Запуск
task db:up
task db:migrate
task apps:up

# Проверка
curl http://localhost:8090/ping
curl http://localhost:8091/ping

# Логи
task apps:logs

# Остановка
task apps:down
task db:down
```

## Investigation MCP и SOM

`ir-api` публикует Streamable HTTP endpoint `POST /mcp` на официальном Go SDK
Model Context Protocol. Он поддерживает обычный пользовательский `access+jwt` +
`X-Project-ID` и delegated `agent+jwt`.
Endpoint предоставляет инструменты:

- `get_investigation_graph` — прочитать граф;
- `list_investigation_events` — прочитать страницу таймлайна;
- `add_investigation_agent_results` — атомарно добавить локальные узлы и
  evidence-backed связи, включая выбранные Gateway records;
- `search_gateway_events` — искать события в разрешённых project sources;
- `lookup_gateway_entity` — обогащать observable через Gateway.

Запись использует canonical `agent-results`: узлы получают `origin=agent`, а
связи создаются в статусе `proposed` и требуют решения аналитика.

При `POST /api/v1/som/issues/{issue_id}/run` `ir-api` передаёт конфигурацию
`investigation` в том же запросе, которым создаётся конкретный environment SOM.
Daemon валидирует только удалённые HTTP(S) MCP-серверы и добавляет их в
`OPENCODE_CONFIG_CONTENT` первого процесса агента, не меняя глобальный профиль
OpenCode и не записывая конфигурацию environment в SQLite. Поэтому параллельные
investigation не могут подменить MCP-конфигурацию друг друга. Для этого flow
требуется `SOM_EXECUTOR=OPENCODE`.

Временный demo-path использует `ACCESS_KEY` из Environment variables профиля
OpenCode. Это обычный короткоживущий пользовательский `access+jwt`. IR передаёт
в remote MCP config только ссылку `Bearer {env:ACCESS_KEY}` и подписанный
текущим запросом `X-Project-ID`; OpenCode подставляет значение при запуске.
Тот же token доступен агенту для прямого Gateway REST на
`http://gateway:8091/api/v1`. Перед демо token нужно обновить вручную; он даёт
агенту те же права, что и пользователю, поэтому не используйте admin JWT.

Delegated `agent+jwt` и server-side exchange остаются поддержанным следующим
этапом, но для demo-запуска через `ACCESS_KEY` не требуются.

MCP является частью бинарника `ir-api`, поэтому его версия развёртывается
атомарно вместе с Investigation API. Подписанный JWT не хранится в `ir-api`,
поэтому его принимает любая реплика с тем же публичным ключом и issuer.

Состояние покрытия требований и реализации — в [api/investigations/COVERAGE.md](api/investigations/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
