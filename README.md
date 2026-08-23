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

Состояние покрытия требований и реализации — в [api/investigations/COVERAGE.md](api/investigations/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
