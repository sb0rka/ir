# IR API

API сервиса расследований Sb0rka. Репозиторий содержит OpenAPI-контракт,
сгенерированные Go/TypeScript-типы, SQL-схему и каркас Go-сервиса.

Сейчас реализована инфраструктура сервиса: конфигурация, проверка JWT,
project scope, health checks, Swagger и миграции. Все 32 доменные операции пока
возвращают `501 not_implemented`.

## Структура

```text
api/                       OpenAPI-фрагменты и общие компоненты
apps/investigations/       Go-сервис `ir-api` и утилита `irctl`
db/migrations/             схема `inv` и справочники
packages/contract/         сгенерированный Go-контракт
packages/contract-ts/      сгенерированный TypeScript-контракт и клиент
```

## Команды

Требуются [Task](https://taskfile.dev/) и POSIX-утилиты (`sh`, `rm`, `perl`).
Генерация из текущего Taskfile не запускается в чистом PowerShell.

```bash
task spec       # собрать и проверить OpenAPI
task gen        # обновить Go/TypeScript-контракты и заглушки
task build      # собрать bin/ir-api и bin/irctl
task check      # генерация, сборка и go vet
task dev        # Postgres, миграции и ir-api
task dev-down
```

Сгенерированные файлы коммитятся, но вручную не редактируются. Источник правды
для API — файлы в `api/`.

## Локальный запуск

```bash
docker compose -f docker-compose.dev.yml up -d --build
curl http://localhost:8090/healthz
curl http://localhost:8090/readyz
```

Для защищённых ручек нужен access-токен платформы с audience `api.local`,
заголовок `X-Project-ID` и роль в этом проекте в `inv.role_bindings`. Роли
выдаются через `irctl`; доступа по умолчанию нет.

Состояние покрытия требований и реализации — в [api/COVERAGE.md](api/COVERAGE.md).
Правила разработки — в [AGENTS.md](AGENTS.md).
