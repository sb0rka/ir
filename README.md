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

`ir-api` публикует project-scoped Streamable HTTP endpoint `POST /mcp`.
Он защищён теми же `Authorization` и `X-Project-ID`, что и REST API, и
предоставляет инструменты:

- `get_investigation_graph` — прочитать граф;
- `list_investigation_events` — прочитать страницу таймлайна;
- `add_investigation_agent_results` — атомарно добавить выбранные записи,
  узлы и evidence-backed связи.

Запись использует canonical `agent-results`: узлы получают `origin=agent`, а
связи создаются в статусе `proposed` и требуют решения аналитика.

При `POST /api/v1/som/issues/{issue_id}/run` сервис читает текущую MCP-карту
daemon через relay, сохраняет существующие серверы и добавляет
`investigation` с точным URL `${IR_PUBLIC_BASE_URL}/mcp` и текущим
`X-Project-ID`. Для этого flow требуется `SOM_EXECUTOR=OPENCODE`: текущий
Codex adapter daemon намеренно не передаёт remote HTTP MCP servers. `ir-api`
создаёт случайный capability token с TTL 4 часа, ограниченный одним project и
investigation; human OAuth token агенту не передаётся, REST этим token недоступен.
Обновление глобальной OpenCode MCP-карты и запуск
environment сериализованы внутри одного `ir-api`; несколько реплик `ir-api` на
один daemon-хост потребуют environment-scoped MCP в SOM и общий capability store.

Состояние покрытия требований и реализации — в [api/investigations/COVERAGE.md](api/investigations/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
