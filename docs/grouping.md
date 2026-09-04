# Группы сущностей и событий

Актуально на 2026-09-04. Детальный дизайн: [blueprints/041-entity-event-grouping.md](blueprints/041-entity-event-grouping.md).

Backend, HTTP/MCP API и хранение реализованы в этом PR. Grouped-view Dashboard
описан ниже как контракт для клиента; клиентские изменения в PR не входят.

## Что знает frontend

Frontend работает только с API-моделью групп. Таблицы members, lineage,
operation links и алгоритм resolution остаются внутри backend.

| Поле `GraphProjection` | Отображение |
| --- | --- |
| `nodes` | атомы или virtual `entity_group` / `event_group` nodes |
| `edges` | агрегированные связи; `×N` = число raw edges |
| `groups` | состав, роли, статусы, confidence и view-scoped assertions |
| `annotations` | sequence/correlation без collapse |
| `diagnostics` | причина безопасного отказа от collapse |
| `raw_nodes`, `raw_edges` | lossless раскрытие и provenance |

Целевой Dashboard имеет два режима:

- **Факты** — прежний raw graph, фильтры, hypothesis layers и timeline.
- **Группы** — server projection текущего investigation (`include_subtree=false`),
  virtual nodes и панель состава. Raw-фильтры и timeline скрыты, чтобы не
  искажать уже агрегированную проекцию.

Клик по virtual node должен выбирать группу в панели. Панель read-only: review,
merge/split требуют полного root detail, которого в projection намеренно нет.
Hypothesis и subtree projection поддержаны API; переключатели остаются задачей
клиента.

## Как рисовать

| Kind | Canvas |
| --- | --- |
| `entity/resolved_entity` | confirmed subject/identifiers → один «Один объект» node |
| `event/same_event` | confirmed primary/duplicates → один «Одно событие» node |
| `event/composite` | confirmed parent/parts → один «Составное событие» node |
| `event/sequence` | атомы остаются; ordered annotation в панели |
| `event/correlation` | атомы остаются; annotation в панели |

Только `confirmed` membership может скрыть атом. `proposed` и `rejected`
видны в `groups`, но не участвуют в collapse. Same-event применяется раньше
composite. Неоднозначные overlaps и identifier ownership оставляют атомы raw и
добавляют diagnostic. Virtual IDs (`entity-group:{uuid}`, `event-group:{uuid}`)
нельзя передавать в atomic node/edge CRUD.

Позиция virtual node — centroid его видимых raw members; raw nodes сохраняют
свои координаты. Edge агрегируется только при одинаковых direction, endpoints,
relation и status; origins, member/evidence IDs и confidence min/max не теряются.

## Scope и семантика

Группа принадлежит `(project_id, root_investigation_id)`. Root и descendants по
`parent_id` разделяют решения; независимые roots изолированы даже при одинаковых
entity/event/source IDs. Это аналитическая граница, не отдельный ACL.

Проекция сначала выбирает investigation, subtree или hypothesis, затем применяет
группы только к evidence этого view. Группа не импортирует скрытые факты из
соседней ветви. Полный title, decision reason и memberships доступны только по
root detail endpoint.

Основные инварианты:

- confirmed subject — максимум в одной active entity group дерева;
- confirmed event — максимум в одной active same-event group;
- same-event имеет один non-rejected primary;
- composite имеет максимум один non-rejected parent;
- sequence ordinals уникальны среди non-rejected members;
- IP identifier collapse требует time-bounded assertion на каждое наблюдение;
- разные PT NAD HostID одного source instance нельзя подтвердить как один subject.

`event_groups` и `event_group_members` полностью задействованы: same-event,
composite, sequence и correlation используют их так же, как entity resolution
использует typed entity tables. Lineage хранит merge/split историю backend и во
frontend не передаётся.

## API

```text
GET /api/v1/investigations/{id}/graph                         # raw, совместим
GET /api/v1/investigations/{id}/graph/projection              # grouped view
GET /api/v1/investigations/{id}/hypotheses/{hypothesis_id}/graph/projection
```

Пример запроса dashboard:

```http
GET /api/v1/investigations/11111111-1111-4111-8111-111111111111/graph/projection?include_subtree=false&statuses=proposed&statuses=confirmed&statuses=rejected
Authorization: Bearer <token>
X-Project-ID: project-demo
```

Сокращённый ответ (assertions и raw objects показаны по одному):

```json
{
  "investigation_id": "11111111-1111-4111-8111-111111111111",
  "root_investigation_id": "11111111-1111-4111-8111-111111111111",
  "include_subtree": false,
  "groups": [{
    "id": "22222222-2222-4222-8222-222222222222",
    "family": "entity",
    "kind": "resolved_entity",
    "version": 3,
    "members": [{
      "id": "33333333-3333-4333-8333-333333333333",
      "node_ids": ["44444444-4444-4444-8444-444444444444"],
      "role": "subject",
      "status": "confirmed",
      "version": 2,
      "confidence": 1,
      "assertions": [{
        "investigation_id": "11111111-1111-4111-8111-111111111111",
        "origin": "source",
        "origin_ref": "pt-nad:device:42",
        "method": "pt-nad-host-id",
        "method_version": "v1",
        "evidence_event_ids": ["55555555-5555-4555-8555-555555555555"],
        "reason": "stable HostID"
      }]
    }]
  }],
  "nodes": [{
    "id": "entity-group:22222222-2222-4222-8222-222222222222",
    "node_type": "entity_group",
    "group_id": "22222222-2222-4222-8222-222222222222",
    "group_kind": "resolved_entity",
    "member_node_ids": ["44444444-4444-4444-8444-444444444444"]
  }],
  "edges": [],
  "annotations": [],
  "diagnostics": [],
  "raw_nodes": [{
    "id": "44444444-4444-4444-8444-444444444444",
    "investigation_id": "11111111-1111-4111-8111-111111111111",
    "node_type": "entity",
    "entity_id": "66666666-6666-4666-8666-666666666666",
    "label": "host-42",
    "type_code": "host",
    "origin": "source",
    "som_issue_ids": []
  }],
  "raw_edges": []
}
```

Для каждой family (`entity-groups`, `event-groups`):

```text
GET  /api/v1/investigations/{root_id}/{family}/{group_id}
GET  /api/v1/investigations/{root_id}/{family}/{group_id}/history
POST /api/v1/investigations/{root_id}/{family}/{group_id}/review
POST /api/v1/investigations/{root_id}/{family}/{group_id}/merge
POST /api/v1/investigations/{root_id}/{family}/{group_id}/split
```

Mutations используют group/member `version`, обязательные `reason` и уникальный
`operation_id`. Повтор того же payload идемпотентен; stale/different payload →
`409`, invalid domain state → `422`, child ID или чужой scope → `404`. Перед
merge/split клиент обязан перечитать полный root detail и явно распределить все
members; автоматический retry решения после `409` запрещён.

MCP `get_investigation_graph` принимает `projection: raw | grouped` (default raw)
и использует тот же backend projector. Group mutation MCP tools не добавлены.

## Создание групп

Группы строятся только из выбранного Gateway context или явных agent proposals:

- PT NAD: device anchor только из HostID + source instance; IP/hostname/MAC — atoms;
- timed `has_identifier` → entity assertion; без времени raw relation сохраняется;
- source session / `subevent_of` → composite; finding → correlation;
- proximity/time bucket не создаёт same-event или sequence;
- agent proposal ссылается на local node refs своего batch и всегда создаёт
  `proposed` memberships.

Context/proposals, nodes и edges пишутся одной транзакцией. Refresh добавляет
assertions, но сохраняет review; superseded group не воскресает после merge/split.

## Хранение и проверки

Schema: [010-initial_schema.sql](../db/migrations/investigations/010-initial_schema.sql).
Typed entity/event tables, lineage и append-only `group_operations` разделены.
Mutation/import блокирует root; projection читается одним repeatable-read snapshot.
Baseline migration изменена по pre-production policy; backfill старых импортов нет.

```powershell
task check
task dashboard-check

$env:INVESTIGATIONS_TEST_DATABASE_URI='<isolated postgres URI with search_path=inv>'
Push-Location apps/investigations
go test -count=1 ./internal/store/psql ./internal/server
Pop-Location
```

CI поднимает disposable PostgreSQL через `db/migrate.sh` и отдельно запускает DB,
HTTP/JWT/MCP runtime tests. `task dashboard-check` проверяет текущие lint/build,
но grouped UI и его browser E2E появятся вместе с клиентской реализацией. Это не
live PT NAD/SOM run и не authenticated browser E2E полного стенда.
