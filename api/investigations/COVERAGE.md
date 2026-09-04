# Покрытие требований Техлаб

Контракт включает домен `groups`: scoped graph projection и entity/event
detail, review, merge, split, history. Источник точного состава операций —
`paths/*.yaml`.

Важно: контракт шире реализованного вертикального среза. Group operations
реализованы; остальные незаявленные операции могут сохранять `501 not_implemented`.

| № | Требование | Контракт | Реализация |
|---|---|---|---|
| 5.1 | Единый рабочий стол инцидента | Investigation, graph, timeline, entity cards | Read-срез готов; tree/update 501 |
| 5.2 | Источники Gateway | Findings/sessions как first-class objects; events/entities как drill-down | Готово для выбранного полного или partial контекста |
| 5.3 | Автосвязывание и ручная корректировка | analyst `mentions`/Gateway relations; agent proposals; tree-scoped entity/event groups | Импорт, рёбра и группы с HTTP review/merge/split/history готовы; UI groups отдельно |
| 5.4 | Граф и таймлайн | `getGraph`, `listNodes`, `listGraphEdges`, `listEvents` | Готово, включая filters/cursors |
| 5.5 | Историчность и окружение сущности | `getEntityCard` | Готово |
| 5.6 | Журнал находок | Hypotheses без иерархии, с status/reason и graph projection; child investigations — отдельные cases | Hypothesis CRUD/memberships готовы; child tree/update — 501 |
| 5.7 | Обогащение артефактов | Вне v0.1 | — |
| 5.8 | Поиск и фильтрация | Поиск внутри кейса; сквозной триаж в Gateway | Готово в заявленном срезе |
| 5.9 | Подсказки следующих шагов | Не описано, опционально | — |
| 5.10 | ИИ-ассистент | SOM facade и явные `agent-results` batches, включая active hypothesis scope | Готово, включая review предложений |
| 5.11 | Пакет реагирования | Вне v0.1 | — |
| 5.12 | Отчёт и экспорт | Вне v0.1 | — |
| 5.13 | Доступ | JWT и обязательный project scope; внутри IR права одинаковы | Готово для демо |
| 5.14 | Журнал действий | Append-only group operations | Готов для групп; общего аудита приложения нет |

## Нефункциональные требования

- **Аутентификация:** Ed25519 JWT платформы с issuer/audience/kid/typ из окружения.
- **Авторизация:** все аутентифицированные пользователи имеют одинаковые права в IR.
- **Project isolation:** обязательный `X-Project-ID` задаёт scope запроса;
  `project_id` хранится на общих для проекта сущностях и hypotheses и
  проверяется составными FK. Узлы, рёбра и memberships получают
  project scope через `investigation_id`.
- **Group isolation:** решения группировки общие только для root и descendants;
  independent roots того же project изолированы. Atomic records остаются
  project-level. Projection ограничивает не только nodes/edges, но и assertions
  текущим investigation/subtree/hypothesis. Это не новый per-case ACL.
- **Воспроизводимость:** finding/session хранит stable ref без времени в identity,
  но с обязательным replay window; событие хранит `source_code + source_event_id`,
  сущность — отдельные ссылки на исходные инструменты. Normalized snapshot,
  безопасный provenance и partial errors сохраняются; agent edge требует `why`
  и evidence.
- **Граница источников:** UI ищет данные через Gateway REST, агент — через
  read-only `gateway_*` tools того же MCP. MCP также читает investigation
  context и записывает выбранные результаты; `ir-api` не хранит полный поток
  или vendor raw payload.

## Вне v0.1

- обогащение артефактов;
- отчёты и пакет реагирования;
- общий аудит приложения, метрики и API администрирования;
- сквозной триаж и поиск вне расследования;
- проверка существования внешних SOM UUID при записи.

## Проверка

```bash
task spec
task check
# PostgreSQL integration tests на отдельной disposable базе с migrations 007–010 и 901:
cd apps/investigations
INVESTIGATIONS_TEST_DATABASE_URI=... go test ./... -count=1
```

CI поднимает отдельный PostgreSQL 18, применяет миграции и запускает в том числе
`TestGroupingHTTPAndMCPRuntime` (реальные HTTP/JWT/MCP + DB, synthetic evidence).
Live PT NAD и UI E2E этим тестом не покрываются. Инструкции — [grouping.md](../../docs/grouping.md).
