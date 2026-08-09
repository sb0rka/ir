![Sb0rka Incident Response](docs/imgs/ir-background-op.png)

Incident Response — Единая картина инцидента и AI-автоматизация расследований.

Русский | [English](README_EN.md)

---

[Сайт Sb0rka IR](https://ir.sb0rka.ru) | [Документация](https://docs.sb0rka.com/ru)

## Состав репозитория

- `api/`: OpenAPI-контракты по сервисам
- `apps/gateway/`: Gateway внешних инструментов безопасности
- `apps/investigations/`: Go API сервис `ir-api` и утилита `irctl`
- `db/migrations/`: схема `inv` и справочники
- `packages/contract/`: сгенерированный Go-контракт
- `packages/contract-ts/`: сгенерированный TypeScript-контракт и клиент

## Команды

Требуются [Task](https://taskfile.dev/) и POSIX-утилиты (`sh`, `rm`, `perl`).

```bash
task spec       # Собрать и проверить OpenAPI
task gen        # Обновить Go/TypeScript-контракты и заглушки
task build      # Собрать bin/ir-api и bin/irctl
task check      # Генерация, сборка и go vet
task dev        # Postgres, миграции и ir-api
task dev-down
```

Сгенерированные файлы коммитятся, но вручную не редактируются. Источник правды
для API — файлы в `api/<service>/`.

## Локальный запуск

```bash
docker compose -f docker-compose.dev.yml up -d --build

curl http://localhost:8090/healthz
curl http://localhost:8090/readyz
```

Для защищённых ручек нужен access-токен платформы с audience `api.local`,
заголовок `X-Project-ID` и роль в этом проекте в `inv.role_bindings`. Роли
выдаются через `irctl`; доступа по умолчанию нет.

Состояние покрытия требований и реализации — в [api/investigations/COVERAGE.md](api/investigations/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
