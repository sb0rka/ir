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
Model Context Protocol. Для ручного доступа он защищён теми же `Authorization`
и `X-Project-ID`, что и REST API; для запуска через SOM используется подписанный
Sb0rka Auth `agent+jwt`, ограниченный одним project и investigation.
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
OpenCode и не записывая JWT в SQLite. Поэтому параллельные investigation
не могут подменить MCP-конфигурацию друг друга. Для этого flow требуется
`SOM_EXECUTOR=OPENCODE`; human OAuth token агенту не передаётся, agent JWT не
принимается REST API и не пересылается в Gateway, Platform API или Secrets.
Для Gateway tools `ir-api` через confidential client обменивает agent JWT в Auth
на обычный пяти минутный access JWT. Этот JWT существует только в памяти на
время server-to-server вызова; Gateway и Platform Secrets продолжают работать
по общему пользовательскому access-JWT pattern.

MCP является частью бинарника `ir-api`, поэтому его версия развёртывается
атомарно вместе с Investigation API. Подписанный JWT не хранится в `ir-api`,
поэтому его принимает любая реплика с тем же публичным ключом и issuer.

Состояние покрытия требований и реализации — в [api/investigations/COVERAGE.md](api/investigations/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
