# Покрытие требований Техлаб

Текущий объём контракта: 24 пути, 37 операций, 6 доменов.

Важно: контракт шире реализованного вертикального среза. Незаявленные
update/delete/review операции сохраняют `501 not_implemented`.

| № | Требование | Контракт | Реализация |
|---|---|---|---|
| 5.1 | Единый рабочий стол инцидента | Investigation, graph, timeline, entity cards | Read-срез готов; tree/update 501 |
| 5.2 | Источники Gateway | Исходные идентификаторы и нормализованные данные | Готово для выбранного контекста |
| 5.3 | Автосвязывание и ручная корректировка | analyst `mentions`/Gateway relations; agent proposals | Импорт готов; review 501 |
| 5.4 | Граф и таймлайн | `getGraph`, `listNodes`, `listGraphEdges`, `listEvents` | Готово, включая filters/cursors |
| 5.5 | Историчность и окружение сущности | `getEntityCard` | Готово |
| 5.6 | Журнал находок | Дерево под-расследований с verdict | 501 |
| 5.7 | Обогащение артефактов | Вне v0.1 | — |
| 5.8 | Поиск и фильтрация | Поиск внутри кейса; сквозной триаж в Gateway | Готово в заявленном срезе |
| 5.9 | Подсказки следующих шагов | Не описано, опционально | — |
| 5.10 | ИИ-ассистент | SOM facade и явный `agent-results` batch | Готово; review 501 |
| 5.11 | Пакет реагирования | Вне v0.1 | — |
| 5.12 | Отчёт и экспорт | Вне v0.1 | — |
| 5.13 | Роли и права | JWT и deny-by-default; RBAC по операциям не задан | Частично |
| 5.14 | Журнал действий | Вне v0.1 | — |

## Нефункциональные требования

- **Аутентификация:** Ed25519 JWT платформы, audience `api.local`.
- **Авторизация:** субъект без `role_bindings` получает `403`. Разграничение
  L1/L2/lead/admin по операциям ещё нужно реализовать.
- **Project isolation:** обязательный `X-Project-ID` проверяется по `role_bindings`;
  `project_id` хранится на общих для проекта сущностях и проверяется составными FK. Узлы и рёбра
  получают project scope через `investigation_id` и не дублируют `project_id`.
- **Воспроизводимость:** событие хранит пару `source_code + source_event_id`,
  сущность — отдельные ссылки на исходные инструменты; нормализованные поля,
  provenance/source URL сохранены, agent edge требует `why` и evidence.
- **Граница источников:** UI и агент читают Gateway напрямую. `ir-api` не
  выполняет Gateway search и не хранит полный поток или vendor raw payload.

## Вне v0.1

- внутренний MCP-контур агента;
- review/accept/reject предложенных рёбер и обогащение;
- отчёты и пакет реагирования;
- аудит, метрики и API администрирования;
- сквозной триаж и поиск вне расследования;
- проверка существования внешних SOM UUID при записи; facade и запуск issue
  остаются тонкой pass-through интеграцией.

## Проверка

```bash
task spec
task check
# PostgreSQL integration tests после task db:wipe && task db:up && task db:migrate:
cd apps/investigations
INVESTIGATIONS_TEST_DATABASE_URI=... go test ./internal/transport ./internal/store/psql
```
