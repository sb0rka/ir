# Покрытие требований Техлаб

Текущий объём контракта: 39 путей, 57 операций, 9 доменов.

Важно: контракт шире реализованного вертикального среза. Незаявленные операции
вне edge CRUD/review сохраняют `501 not_implemented`.

| № | Требование | Контракт | Реализация |
|---|---|---|---|
| 5.1 | Единый рабочий стол инцидента | Investigation, graph, timeline, entity cards | Read-срез готов; tree/update 501 |
| 5.2 | Источники Gateway | Findings/sessions как first-class objects; events/entities как drill-down | Готово для выбранного полного или partial контекста |
| 5.3 | Автосвязывание и ручная корректировка | analyst `mentions`/Gateway relations; agent proposals | Импорт, ручные рёбра и review готовы |
| 5.4 | Граф и таймлайн | `getGraph`, `listNodes`, `listGraphEdges`, `listEvents` | Готово, включая filters/cursors |
| 5.5 | Историчность и окружение сущности | `getEntityCard` | Готово |
| 5.6 | Журнал находок | Плоские hypotheses с status/reason и graph projection; child investigations — отдельные cases | Hypothesis CRUD/memberships готовы; child tree/update — 501 |
| 5.7 | Обогащение артефактов | Вне v0.1 | — |
| 5.8 | Поиск и фильтрация | Поиск внутри кейса; сквозной триаж в Gateway | Готово в заявленном срезе |
| 5.9 | Подсказки следующих шагов | Не описано, опционально | — |
| 5.10 | ИИ-ассистент | SOM facade и явные `agent-results` batches, включая active hypothesis scope | Готово, включая review предложений |
| 5.11 | Пакет реагирования | Вне v0.1 | — |
| 5.12 | Отчёт и экспорт | Вне v0.1 | — |
| 5.13 | Доступ | JWT и обязательный project scope; внутри IR права одинаковы | Готово для демо |
| 5.14 | Журнал действий | Вне v0.1 | — |

## Нефункциональные требования

- **Аутентификация:** Ed25519 JWT платформы с issuer/audience/kid/typ из окружения.
- **Авторизация:** все аутентифицированные пользователи имеют одинаковые права в IR.
- **Project isolation:** обязательный `X-Project-ID` задаёт scope запроса;
  `project_id` хранится на общих для проекта сущностях и hypotheses и
  проверяется составными FK. Узлы, рёбра и memberships получают
  project scope через `investigation_id`.
- **Воспроизводимость:** finding/session хранит stable ref без времени в identity,
  но с обязательным replay window; событие хранит `source_code + source_event_id`,
  сущность — отдельные ссылки на исходные инструменты. Normalized snapshot,
  безопасный provenance и partial errors сохраняются; agent edge требует `why`
  и evidence.
- **Граница источников:** UI и агент читают Gateway напрямую. `ir-api` не
  выполняет Gateway search и не хранит полный поток или vendor raw payload.

## Вне v0.1

- внутренний MCP-контур агента;
- обогащение артефактов;
- отчёты и пакет реагирования;
- аудит, метрики и API администрирования;
- сквозной триаж и поиск вне расследования;
- проверка существования внешних SOM UUID при записи.

## Проверка

```bash
task spec
task check
# PostgreSQL integration tests после task db:wipe && task db:up && task db:migrate:
cd apps/investigations
INVESTIGATIONS_TEST_DATABASE_URI=... go test ./internal/transport ./internal/store/psql
```
