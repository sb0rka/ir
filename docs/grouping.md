# Группы сущностей и событий

Backend implementation, 2026-09-03. Основание: [техдизайн и исследование](blueprints/041-entity-event-grouping.md).
Dashboard в эту работу не входит; старый raw API совместим с текущим клиентом.

## Граница данных

Группа принадлежит `(project_id, root_investigation_id)`: корневому расследованию
и всем descendants по `parent_id`. Самостоятельный кейс — дерево из одного узла.
Соседние child investigations одного корня используют общие решения. Независимые
roots не разделяют группы, статусы, историю, merge/split или ключи повторов,
даже если atomic entity/event и source IDs одинаковы.

Это граница аналитических решений, не новый ACL. В текущем IR все валидные
JWT-пользователи имеют одинаковые права, scope задаёт `X-Project-ID`.
Project-level canonical facts остаются общими; grouping не переписывает их
аналитическими выводами. Reparent/copy/share групп между деревьями не реализован.

Проекция сначала выбирает investigation (или явное subtree/hypothesis), затем
применяет решения своего дерева только к доступным в этом view фактам.
Общий group ID не добавляет evidence из соседней ветви. Для effective membership
нужно сохранившееся attachment и assertion с evidence в текущем view.
Soft delete скрывает соответствующий view, но оставляет историю решений.

## Семантика

| Family / kind | Роли | Поведение |
| --- | --- | --- |
| entity / resolved_entity | subject, identifier | Предполагаемый один объект; subject совпадает с type_code группы |
| event / same_event | primary, duplicate | Один occurrence; один non-rejected primary, confirmed duplicates требуют confirmed primary |
| event / composite | parent, part | Составное событие; не более одного non-rejected parent |
| event / sequence | step + ordinal | Упорядоченные шаги; уникальный non-negative ordinal среди non-rejected members |
| event / correlation | evidence | Общий контекст, не identity |

Confirmed subject принадлежит не более чем одной active entity group в дереве.
Confirmed event — не более чем одной active same_event group. Identifier может
иметь несколько owners; это не транзитивное равенство устройств.

Сохраняются отдельные assertions с originating investigation, origin/ref,
method/version, evidence event IDs, validity interval и reason. Два наблюдения
не растягиваются через ненаблюдавшийся промежуток. Для confirmed IP identifier
обязательны обе временные границы. Несколько разных PT NAD HostID одного
source instance нельзя подтвердить как один subject group.

## Импорт

Источник grouping — уже выбранный Gateway context, не фоновое сканирование проекта.

- PT NAD device anchor создаётся только при HostID **и** source instance.
  Значение — namespaced stable UUID; IP, MAC и hostname остаются отдельными atoms.
- `has_identifier` с observation time формирует confirmed point-in-time assertion.
  Без времени grouping пропускается с warning; raw relation сохраняется.
- Source session и явные `subevent_of` дают composite. Пути raw events и source
  object используют один ключ одного PT NAD parent occurrence.
- Finding даёт correlation, не same_event. Sequence/same_event автоматически
  из соседства или time bucket не выводятся.
- Source refresh добавляет assertions, но сохраняет reviewed status, role,
  order, confidence и reason. Superseded group не воскресает: после merge
  используется survivor; после split — известные successors. Новые неоднозначные
  members остаются raw с warning.

Оба `agent-results` endpoint и MCP `add_investigation_agent_results` принимают
необязательные `entity_group_proposals` / `event_group_proposals`. Каждый proposal:

- имеет UUID `proposal_id`, kind, title, why, members и evidence_event_refs;
- использует **локальные node refs того же batch**, а не произвольные tree IDs;
- для entity family указывает type_code; роли зависят от kind;
- сохраняет SOM issue IDs как provenance и всегда создаёт `proposed` memberships.

Nodes, edges, source context и proposals пишутся одной транзакцией. Ошибка в
одном proposal откатывает весь batch. Hypothesis route использует те же правила
и ограничивает explicit graph membership выбранной active hypothesis.

## HTTP и MCP

Source contract: [groups.yaml](../api/investigations/paths/groups.yaml),
[AgentResultBatch](../api/investigations/paths/investigations.yaml).
Go/TS контракты генерируются `task gen`; TS импортируется из `@ir/contract/domains/groups`.

Read views:

```text
GET /api/v1/investigations/{investigation_id}/graph
GET /api/v1/investigations/{investigation_id}/graph/projection
GET /api/v1/investigations/{investigation_id}/hypotheses/{hypothesis_id}/graph/projection
```

Первый route не меняется. Новые routes возвращают `GraphProjection`:

- `root_investigation_id` — вычисленный scope; `include_subtree` по умолчанию false;
- `nodes`, `edges` — collapsed view с virtual string IDs;
- `raw_nodes`, `raw_edges` — все atoms/edges выбранного view для раскрытия,
  включая связи, ставшие internal self-loops после collapse;
- `groups` — discovery видимых active groups: versions, **все статусы** members,
  node IDs, роли, confidence и только view-supported assertions;
- `annotations` — confirmed sequence/correlation; sequence сохраняет ordinal order;
- `diagnostics` — безопасные коды неоднозначности и только видимые node IDs.

Group-wide title и decision_reason намеренно отсутствуют в projection: они
могут описывать members из других ветвей. Root detail возвращает их явно.
Collapsed node содержит group_id/kind и member_node_ids; provenance/статусы —
в `groups`, originating investigation — в raw objects.

Only confirmed memberships могут скрывать atoms. Сначала same_event, затем
composite. Неполное пересечение composite с same_event или конкурирующие
composites не выбирают победителя по порядку rows. Identifier сворачивается
только при одном owner на каждом видимом event/evidence timestamp. Если время
неизвестно или owners меняются, он остаётся raw.

Edges агрегируются по direction, endpoints, relation и status. Origins,
member_edge_ids и evidence_event_ids явно перечислены и детерминированно
отсортированы. Confidence min/max считаются по известным значениям; отсутствующий
confidence остаётся отсутствующим в raw edge, а не превращается в 0.
Projection IDs нельзя отправлять в atomic GraphNode/GraphEdge CRUD.

MCP `get_investigation_graph` принимает `projection: raw | grouped`, default raw.
Grouped использует тот же код, что HTTP; никакого implicit include_subtree.
Новых MCP review/merge/split tools нет.

Для каждой family (`entity-groups`, `event-groups`):

```text
GET  /api/v1/investigations/{root_id}/{family}/{group_id}
GET  /api/v1/investigations/{root_id}/{family}/{group_id}/history?limit=50&cursor=...
POST /api/v1/investigations/{root_id}/{family}/{group_id}/review
POST /api/v1/investigations/{root_id}/{family}/{group_id}/merge
POST /api/v1/investigations/{root_id}/{family}/{group_id}/split
```

Здесь `root_id` обязан быть настоящим root. Child ID или чужая группа — `404`,
как missing. View не является разрешением на root detail; текущая проверка
прав — существующий JWT/project механизм, не per-case capability.

Review принимает `operation_id`, group `version`, `reason` и members с
`id`, `version`, status `confirmed|rejected`. Проверяется итоговая целая группа.
Merge: path group — survivor; sources задаются с expected versions; members
должны явно распределить **все** memberships survivor и sources с ролями/order.
Conflicting statuses одного atom требуют отдельного review перед merge.
Split задаёт две или более named partitions с полным покрытием members.
Только identifier можно явно назначить нескольким partitions; subjects/events — нет.

Идентичный `operation_id` и payload возвращает сохранённый результат. Изменённый
payload или stale version — `409`. Payload proposal ID также immutable в scope
дерева. Invalid domain state — `422`. Whole-tree mutation требует JWT actor;
при `AUTH_DISABLED=true` без actor review/merge/split отвечают `401`.

## Хранение и конкурентность

[010-initial_schema.sql](../db/migrations/investigations/010-initial_schema.sql):
entity_resolution_groups/members и event_groups/members — отдельные typed
таблицы. FK ограничивают project/root и тип atomic ID. Source keys имеют
method/version prefix и encoded identity tuple; human titles не участвуют в identity.

Entity/event lineage раздельный. `group_operations` и typed operation links
сохраняют actor, hash, reason, before/after и versions. UPDATE/DELETE audit
запрещён триггером; физическое удаление всего root разрешает каскадную очистку.
Soft delete ничего не удаляет из audit. History cursor привязан к
project/root/family/group и использует keyset pagination.

Каждая group mutation/import блокирует root investigation в транзакции.
Это обеспечивает exclusive confirmed memberships и optimistic concurrency
без дополнительного lock service. Read projection — единый repeatable-read
snapshot; attachments загружаются пакетно, группы выбираются только для atoms view.

Изменена baseline migration согласно pre-production политике репозитория.
Автоматического backfill исторических импортов нет: source grouping возникает
при следующем context import. Не запускайте wipe рабочей БД ради тестирования.

## Frontend handoff

1. Оставить raw mode и добавить отдельный grouped fetch по новым endpoints.
2. Discovery/review badges брать из `groups`, а не только collapsed nodes:
   proposed/rejected остаются видимыми и не меняют raw evidence.
3. Collapse/expand использовать server mapping, не connected components по IP.
4. Group card может показывать view-scoped assertions; полный состав и
   review/merge/split требуют явного перехода к root scope, полученному из ответа.
5. Перед merge/split получить полный root detail; projection может содержать
   только часть memberships. При `409` перечитать состояние, не повторять
   автоматически решение с новой version.
6. При смене investigation/hypothesis/project очистить view state. Cache key
   должен включать project, root, investigation, hypothesis и graph filters.

Ни один файл `apps/dashboard/**` в этой реализации не изменяется.

## Проверки

CI создаёт disposable PostgreSQL 18, применяет migrations 007–010 и 901, затем
`task check` запускает также DB и HTTP/JWT/MCP runtime tests. Локально используйте
отдельную пустую test database с теми же migrations и search_path=inv:

```powershell
$env:INVESTIGATIONS_TEST_DATABASE_URI='postgres://postgres@127.0.0.1:55439/ir_grouping?sslmode=disable&search_path=inv'
task check
task dashboard-check
```

Указанный URI — пример для отдельного временного сервера, не команда его создания.
Без переменной integration tests явно skip. Unit tests покрывают transitions,
role invariants, temporal gaps, ambiguous owners, overlaps и deterministic
aggregation. DB tests покрывают дерево A/A1/A11/A2 и independent B с общими atoms,
изолированные group IDs, views/hypotheses, soft delete, source object/raw retries,
review, audit, merge/split refresh, concurrency и atomic proposal rollback.
`TestGroupingHTTPAndMCPRuntime` проверяет настоящий HTTP listener, JWT, DB и MCP
на synthetic evidence. Это не live PT NAD, SOM agent run или browser/UI E2E.
