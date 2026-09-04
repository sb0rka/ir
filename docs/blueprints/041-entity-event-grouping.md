# Technical Design: группировка сущностей и событий

Статус: **Область согласована; backend реализован, редакция 2026-09-03**

Область реализации: `ir` backend — Gateway normalization, evidence store,
HTTP contracts, investigation graph projection и Investigation MCP.
Dashboard не меняем; для frontend готовим описание контракта и UX handoff.

Baseline: `origin/main` на момент создания документа, commit `b6ca1c1`

Реализация и точный frontend handoff: [Группы расследования](../grouping.md).
Этот blueprint сохраняет обоснование и концептуальную модель; точные SQL/DTO
задаются миграцией и OpenAPI, а не псевдосхемами ниже.

Уточнения реализации:

- Method/version, originating investigation, интервалы и evidence хранятся в
  массиве `assertions` каждого membership; нет дублирующих group-level полей.
- `group_key` включает namespace метода/версии; unique — project/root/key
  отдельно в каждой typed group table. Source identity tuple кодируется
  Base64URL, поскольку PostgreSQL text/JSONB не принимает NUL-разделители.
- Одна блокировка root investigation сериализует group mutations; отдельный
  набор group locks не нужен. Проекция использует repeatable-read snapshot.
- Виртуальные nodes минимальны; `groups` в projection содержит видимые
  memberships, статусы, confidence и assertions. Полные title/review reasons
  не подмешиваются из соседних ветвей; их возвращает explicit root detail.
- У IR сейчас JWT + project scope, без per-investigation ACL/capability токенов.
  Tree isolation относится к аналитическим решениям, не к новым правам доступа.

## Краткое решение

IR сохраняет исходные нормализованные сущности и события как атомарные факты и
не объединяет их физически. Поверх них вводятся две разные модели:

1. **Entity resolution group** — несколько наблюдений описывают один объект
   реального мира, например один endpoint в NAD, EDR и SIEM.
2. **Event group** — события относятся к одному логическому целому: являются
   дублями одного occurrence, частями составного события либо входят в
   последовательность/корреляцию.

Группы принадлежат **одному дереву расследований**: корневому расследованию
и всем его дочерним расследованиям на любой глубине. Ключ области —
`(project_id, root_investigation_id)`. Дочерние и соседние ветви одного корня
используют общие groups и membership decisions; независимые корни — никогда.
Самостоятельное расследование без детей образует дерево из одного узла.

Совпадение atomic entity/event, source ID, IP, hostname, SOM workspace или
ссылки между кейсами не расширяет область. Между независимыми деревьями нет
общих групп, наследования review, автоматического reuse или copy/import groups.
Это заменяет прежнее предложение project-wide groups.

Расследования продолжают ссылаться на атомарные evidence rows. Старый raw graph
API сохраняется; новый отдельный endpoint строит grouped projection.
HTTP и MCP используют одну серверную семантику.

Не вводится одна универсальная таблица `groups`: entity resolution и event
grouping имеют разные инварианты, роли участников и правила подтверждения.

## Контекст и проблема

В разных источниках один endpoint может быть представлен как:

- source-native device/agent ID;
- hostname;
- hardware UUID или serial number;
- один или несколько MAC;
- меняющийся IP;
- набор связанных account/user/process observations.

Одновременно одна исходная запись может раскрываться в несколько granular
events: network session, HTTP request, authentication, SMB operation, file
transfer. Похожие записи также могут быть независимыми событиями, дублями
одного события в разных источниках или элементами более длинной корреляции.

Если использовать для всех случаев обычное «склеивание», ложное совпадение
теряет provenance и распространяется транзитивно по графу. Если выполнять
сворачивание только в Dashboard, API, MCP и аналитик получают разные картины.

## Текущее состояние `main`

### Атомарные evidence objects

- `events` уникальны по `(project_id, source_code, source_event_id)` и хранят
  normalized snapshot с provenance.
- `entities` уникальны по `(project_id, type_code, canonical_key)`.
- `entity_sources` сопоставляет source-native identity существующей entity.
- `event_entity_relations` и `entity_relations` сохраняют source facts.
- `findings` и `network_sessions` являются first-class coarse source objects;
  их granular context связан через `finding_events`,
  `network_session_events` и соседние таблицы.

Таким образом, текущий `events` — это нормализованное наблюдение источника, а не
гарантированно уникальное событие реального мира.

### Investigation graph

`graph_nodes` ссылаются на membership атомарного event/entity в конкретном
расследовании. `edges` принадлежат расследованию и имеют origin, confidence,
why и review status.

Source-owned `subevent_of` передаётся Gateway через
`parent_source_event_id`/`relation_type`, после чего Investigations создаёт
event-to-event edge. Это полезная декомпозиция, но ещё не общая модель event
groups.

### Ограничения текущей модели

- `canonical_key` принимает решение об identity до того, как можно сохранить
  неоднозначность и evidence этого решения.
- У entity resolution нет validity interval, method version, confidence и
  review lifecycle в области дерева расследований.
- Нет first-class различия между `same_event`, `composite`, `sequence` и
  `correlation`.
- Graph edge выражает отношение внутри одного investigation, но не заменяет
  resolution/grouping, общие для корня и его дочерних расследований.
- Клиентское сворачивание не может быть общей истиной для Dashboard, MCP и
  будущих consumers.

## Термины

| Термин | Значение | Пример |
| --- | --- | --- |
| Atomic entity | Сохранённое наблюдение/identity из одного или нескольких источников | `host:edr-agent-42`, `ip:10.0.0.5` |
| Resolved entity group | Один предполагаемый объект реального мира | endpoint WS-1042 |
| Identifier | Признак объекта, но не обязательно сам объект | IP, MAC, hostname |
| Atomic event | Одна нормализованная source event row | NAD HTTP record |
| `same_event` | Несколько rows описывают один occurrence | SIEM и EDR видят один process start |
| `composite` | Одно логическое событие состоит из частей | NAD session с HTTP/file/auth parts |
| `sequence` | Упорядоченные самостоятельные события | login → SMB → execution |
| `correlation` | Связанный набор без утверждения identity или composition | 20 failed logins за 10 минут |
| Projection | Представление raw evidence с раскрытыми или свёрнутыми группами | grouped investigation graph |
| Investigation tree scope | Корень и все его descendants по `parent_id` в одном проекте | кейс инцидента и зависимые расследования |

Принадлежность одному investigation означает только общий контекст. Она не
является доказательством `same_entity` или `same_event`.

## Область расследования и запрет распространения между деревьями

`project_id` остаётся внешней границей данных, но недостаточен для группировки.
В каждой операции сервер проверяет investigation в текущем проекте и проходит
по `parent_id` до корня. `root_investigation_id` вычисляется сервером, а не
принимается как доверенное поле proposal/import body.

- Один корень, его дети, внуки и соседние дочерние ветви имеют одну область
  группировок. Hypothesis остаётся view внутри своего investigation, не новым
  владельцем группы.
- `review`, `merge` и `split` меняют группу для всего этого дерева. Это явно
  указано в API responses и будущем UI; отдельные local overrides не вводятся.
- Два независимых корня одного проекта могут содержать те же atomic rows,
  но имеют разные group IDs, решения, историю и idempotency namespaces.
- `include_subtree` расширяет только выбранное представление на descendants
  запрошенного investigation. Он не выбирает область хранения и не добавляет
  предков, соседние ветви или независимые деревья в response.
- Cross-tree group lookup/mutation отвечает `404`, как для отсутствующей
  записи, даже если оба дерева принадлежат одному проекту. Ошибки не раскрывают
  название чужой группы, участников, историю, counts или факт конфликта с ней.
- Совпадение источника или atomic ID не является membership дерева. Import и
  candidate selection используют только явно импортируемый context и evidence,
  привязанный к активным investigations текущего дерева; обход всех source
  relations/coarse-object members проекта запрещён.

Membership атомарных фактов в расследованиях остаётся отдельным от group
membership. Группа не прикрепляет evidence к расследованию автоматически.
После soft delete дочернего кейса evidence, оставшееся только в удалённом кейсе,
исключается из effective projection; история решений сохраняется. Удаление
корня делает группы его дерева недоступными для обычных reads и mutations.

Перенос investigation под другой корень, объединение независимых деревьев и
перенос групп между ними не входят в этот PR. Текущий update contract не меняет
`parent_id`; будущая такая операция обязана отдельно решить судьбу групп и не
может молча менять их scope. При создании ребёнок сразу использует scope корня.

### Что эта граница не меняет

Существующие canonical `events`/`entities` и source observations остаются
project-level: их физическую изоляцию и текущий snapshot refresh этот PR не
перепроектирует. Поэтому tree-scoped groups — не обещание полной изоляции всех
существующих данных IR и не новый per-investigation RBAC.

Новые аналитические conclusions, review reasons, group provenance и aggregate
metadata хранятся только в tree-scoped таблицах, а не в общих entity/event
metadata. Group labels, confidence и explanations нельзя обогащать решениями
или observations из другого дерева. Отдельная проверка существующего
canonical storage потребуется, если нужна изоляция самих source snapshots.

## Цели

- Показывать один логический device вместо набора source-specific observations
  и identifiers.
- Позволять сворачивать составные и повторно зарегистрированные события без
  удаления атомарного evidence.
- Давать Dashboard, HTTP API и MCP одинаковую проекцию.
- Объяснять каждое объединение: какие signals использованы, кем и какой версией
  правила принято решение.
- Поддерживать proposed/confirmed/rejected и безопасный split/rollback.
- Сохранять project isolation, независимость деревьев расследований, provenance
  и investigation/hypothesis membership.
- Оставаться детерминированными и идемпотентными при повторном импорте.

## Не цели первого implementation PR

- Обучение ML-модели entity resolution.
- Автоматическое объединение по одному IP, hostname или MAC.
- Кросс-проектная identity.
- Глобальные группы проекта, обмен группами и перенос решений между
  независимыми деревьями, даже при совпадающих atomic facts.
- Удаление или замена `findings`, `network_sessions`, `entities`, `events` и
  существующего graph.
- Любые изменения `apps/dashboard/**`, включая analyst review/merge/split UI.
- Probabilistic scoring engine и field-level effective-profile fusion.
- Пересчёт всей исторической базы в фоне.
- Универсальный rule engine для всех будущих correlation-языков.

## Инварианты

1. **Atomic facts are immutable in meaning.** Resolution меняет проекцию, но не
   переписывает source identity, provenance или исходную принадлежность.
2. **Project scope is absolute.** Группа и каждый её участник имеют один
   `project_id`; любые lookup/mutation фильтруются по проверенному проекту.
3. **Different relation, different meaning.** `same_event`, `composite`,
   `sequence` и `correlation` не преобразуются друг в друга неявно.
4. **Identifiers are not equality.** IP/MAC/hostname могут быть evidence для
   device, но не становятся доказанным `same_entity` сами по себе.
5. **No silent transitivity.** Общий weak identifier не соединяет все
   затронутые entities через connected components.
6. **Ambiguity stays visible.** Если identifier подходит нескольким devices,
   grouped projection не выбирает владельца по порядку rows.
7. **Raw is always recoverable.** Любая grouped response раскрывается до
   исходных nodes, edges и evidence IDs.
8. **Agents propose.** Agent-created resolution/group membership начинается с
   `proposed`; confirmed source assertions и решения аналитика маркируются
   отдельно.
9. **Graph mutations target atomic objects.** Виртуальный group node не
   подменяет FK graph schema и не становится скрытым местом хранения evidence.
10. **Evidence survives edge aggregation.** Свёрнутое ребро содержит ссылки на
    все исходные edges и их evidence, а не только первый найденный edge.
11. **Tree scope is absolute.** Group, membership, operation, candidates и
    derived response имеют один `root_investigation_id`. Одинаковый atomic ID
    в двух деревьях не объединяет их группы и не создаёт cross-tree conflict.
12. **Group scope is not view scope.** Общее решение дерева не расширяет
    investigation/hypothesis view и не меняет существующие права доступа.
13. **No cross-tree analytical writes.** Review, merge, split и refresh одной
    области не изменяют группы, history или explanations другой области.

## Владение семантикой

| Компонент | Ответственность |
| --- | --- |
| Gateway adapter | Нормализует source-native IDs, explicit parent/child и identifier observations; не принимает межисточниковое решение сам |
| Investigations store | Хранит atomic evidence, группы, membership decisions и provenance |
| Grouping logic внутри Investigations | Применяет source policy и agent proposals только в текущем дереве; отдельного сервиса/worker нет |
| Projection logic внутри Investigations | Строит grouped graph и агрегирует edges без потери evidence; raw API не меняет |
| Dashboard, follow-up коллег | Использует подготовленный contract/handoff для collapse/expand и review actions; код вне этого PR |
| Investigation MCP | Читает ту же серверную проекцию; агент не получает отдельную модель мира |

## Модель entity resolution

### `entity_resolution_groups`

Одна строка представляет предполагаемую identity реального объекта внутри
одного дерева расследований, а не общепроектную identity.

Предлагаемые поля:

```text
id                  uuid
project_id          varchar(12)
root_investigation_id uuid
type_code           varchar(64)
group_key           varchar
origin              source | rule | analyst | agent
method              varchar
method_version      varchar
display_name        varchar nullable
state               active | superseded
version             integer
created_at          timestamptz
updated_at          timestamptz
```

`type_code` задаёт тип subject members группы, например `host` или `user`.
Для PT NAD используется `device`. Группа не копирует metadata из одного
«главного» источника. Field-level effective profile откладывается.
Source key уникален в `(project_id, root_investigation_id, type_code, method,
method_version, group_key)`, а не во всём проекте.

### `entity_resolution_members`

```text
id                  uuid
group_id            uuid
project_id          varchar(12)
root_investigation_id uuid
entity_id           uuid
role                subject | identifier
status              proposed | confirmed | rejected
confidence          real nullable
valid_from          timestamptz nullable
valid_to            timestamptz nullable
origin              source | rule | analyst | agent
origin_ref          varchar nullable
method              varchar
method_version      varchar
decision_reason     varchar nullable
evidence            jsonb
version             integer
created_at          timestamptz
updated_at          timestamptz
```

Роли различаются принципиально:

- `subject` означает, что source-specific entity считается наблюдением того же
  реального объекта. Её `type_code` должен совпадать с типом группы.
- `identifier` означает только принадлежность/наблюдение identifier в заданный
  период. Identifier может относиться к нескольким группам, например shared IP
  за NAT или повторно выданный адрес.

Для одного atomic entity допускается не более одного текущего confirmed
`subject` membership в active group **внутри одного дерева**. В другом дереве
эта entity может иметь независимое решение. Proposed candidates ограничению
не подчиняются. При merge выбирается survivor group; при split создаются
новые groups, atomic rows не перемещаются и не удаляются.

Disjoint temporal assertions сохраняются раздельно: нельзя объединять несколько
наблюдений в `[min(valid_from), max(valid_to)]`, приписывая identifier устройству
в ненаблюдавшийся промежуток. Group member evidence содержит привязку к
investigation/source assertion, чтобы проверять tree membership и provenance.

### Сила identity signals

| Класс | Примеры | Автоматическое действие |
| --- | --- | --- |
| Strong | source-native agent/device UID, hardware UUID, serial number | Может подтвердить subject membership при отсутствии противоречий |
| Combined | MAC + hostname + пересекающееся время + source context | Только versioned deterministic rule; обычно proposed |
| Weak | IP, hostname, username, display name, одиночный MAC | Только candidate/evidence, не auto-merge |
| Contradiction | Разные strong IDs одновременно, несовместимые tenants/source instances | Запрет merge и diagnostic reason |

MAC не считается глобальным стабильным ключом: он может рандомизироваться или
переиспользоваться. IP всегда temporal signal.

### Candidate и decision flow

```text
atomic entities из текущего дерева
    → blocking по source UID / hardware ID / bounded identifier+time
    → candidate pairs
    → deterministic score + contradiction checks
    → proposed membership
    → auto-confirm только allowlisted strong policy
    → analyst confirm/reject для серой зоны
```

Blocking ограничивает число сравнений, но не является доказательством merge.
Порог применяется к membership decision, а качество проверяется также на
итоговых группах, чтобы одна ошибочная связь не склеила два больших кластера.

Это направление последующего развития, не scoring engine первого PR. Сейчас
реализуются только explicit source assertions и agent proposals. Даже будущий
blocking обязан включать `project_id` и `root_investigation_id` до сравнения.

## Модель event grouping

### `event_groups`

```text
id                  uuid
project_id          varchar(12)
root_investigation_id uuid
kind                same_event | composite | sequence | correlation
group_key           varchar
title               varchar nullable
started_at          timestamptz nullable
ended_at            timestamptz nullable
state               active | superseded
origin              source | rule | analyst | agent
origin_ref          varchar nullable
method              varchar
method_version      varchar
metadata            jsonb
version             integer
created_at          timestamptz
updated_at          timestamptz
```

`group_key` обеспечивает идемпотентный upsert в пределах
`(project_id, root_investigation_id, kind, method, method_version, group_key)`.
Он не выдаётся за глобальную identity и не вычисляется только из изменяемого
title.

### `event_group_members`

```text
id                  uuid
group_id            uuid
project_id          varchar(12)
root_investigation_id uuid
event_id            uuid
role                primary | duplicate | parent | part | step | evidence
ordinal             integer nullable
status              proposed | confirmed | rejected
confidence          real nullable
origin              source | rule | analyst | agent
origin_ref          varchar nullable
method              varchar
method_version      varchar
decision_reason     varchar nullable
evidence            jsonb
version             integer
created_at          timestamptz
updated_at          timestamptz
```

Правила по kind:

| Kind | Допустимая семантика membership |
| --- | --- |
| `same_event` | Один `primary`, остальные `duplicate`; все описывают один occurrence |
| `composite` | Ноль или один `parent`, остальные `part`; части остаются самостоятельными atomic events |
| `sequence` | `step` с явным `ordinal`; events самостоятельны и только упорядочены |
| `correlation` | `evidence`; общий rule/context без identity или composition claim |

Event может входить в несколько sequence/correlation groups. Одновременно он
может состоять только в одной active confirmed `same_event` group в данном
дереве. Composite membership не означает duplicate и не дедуплицирует raw
таймлайн. Решение другого дерева не учитывается в этом ограничении.

### Интеграция с coarse source objects

`findings` и `network_sessions` не заменяются:

- явная session создаёт deterministic `composite`, finding — `correlation`
  group в дереве расследования, куда импортируется context;
- существующие `finding_events`/`network_session_events` являются источником
  подтверждённых memberships только после пересечения с импортируемым context
  или активным evidence membership текущего дерева;
- source object UUID и source identity входят в `origin_ref`/metadata;
- partial refresh не удаляет ранее подтверждённые members автоматически;
- отсутствие member в partial response не означает его удаление или stale
  state; source refresh не отменяет analyst rejection.

`subevent_of`, не покрытый coarse container, создаёт `composite`; покрытый не
создаёт вторую группу той же декомпозиции. Правило общее для providers.
Root/child повторный import переиспользует group в своём дереве. Импорт того
же source object в независимый корень создаёт независимую group.

PT NAD формирует device anchor только при наличии `HostID`: ключ содержит
source code, source instance и HostID. IP/MAC/hostname fallback identity нет.
Identifier assertions сохраняют время и provenance; отсутствие HostID
оставляет identifiers атомарными. Все ResolveContext/ResolveFinding/
ResolveSession paths получают regression coverage.

Refresh после merge направляется в survivor по lineage внутри дерева. После
split уже распределённые source members обновляют своих successors; новые
неоднозначные members остаются raw/ungrouped с безопасным warning. Superseded
group не воскресает при повторном импорте.

Существующий `subevent_of` остаётся raw graph relation. Event group добавляет
общую семантику collapse/expand; эти представления не должны расходиться.

## Серверная graph projection

### API shape

Существующий `/graph` и атомарный `GraphNode` с UUID не меняются. Новый OpenAPI
domain `groups` предоставляет отдельный endpoint и отдельный DTO:

```http
GET /api/v1/investigations/{id}/graph
GET /api/v1/investigations/{id}/graph/projection
GET /api/v1/investigations/{id}/hypotheses/{hypothesis_id}/graph/projection
```

`/graph` возвращает прежний raw response. `/graph/projection` возвращает
grouped view и вычисленный `root_investigation_id`; include_subtree и graph
filters сохраняют смысл текущего API. MCP `get_investigation_graph` получает
`projection: raw | grouped` с default `raw`; обе ветки используют ту же логику,
что HTTP. Dashboard в этом PR продолжает использовать старый raw API.

Grouped node содержит:

```json
{
  "id": "entity-group:<uuid>",
  "node_type": "entity_group",
  "group_kind": "resolved_entity",
  "group_id": "<uuid>",
  "member_node_ids": ["<uuid>", "<uuid>"]
}
```

Virtual ID стабилен для одного group, но не сохраняется в `graph_nodes`.
Разные деревья имеют разные group IDs, даже если atomic member IDs совпадают.
Response сохраняет originating investigation ID для каждого raw node/edge.
Confirmed active members могут сворачиваться; proposed/rejected не скрывают
атомарные узлы. Сначала применяется `same_event`, затем непротиворечивые
composite groups; при неоднозначном overlap узлы остаются раскрытыми с
diagnostic. Sequence/correlation возвращаются как annotations, не как
заменяющие все members nodes.

### Edge aggregation

После remap endpoint nodes сервер:

1. сохраняет direction и `relation_code`;
2. не объединяет edges разных status/origin без явной aggregate shape;
3. исключает generated self-loop из collapsed view, но возвращает его при
   expand;
4. возвращает `member_edge_ids`, `evidence_event_ids` (их длина — count),
   range известных confidence и origins; неизвестные confidence остаются null
   в raw edges и не становятся нулём;
5. сортирует members/edges детерминированно.

Если identifier имеет более одного eligible confirmed owner в relevant time,
он не сворачивается ни в один device. Response содержит ambiguity diagnostic.

### Investigation и hypothesis membership

Сначала выбирается group scope по корню запрошенного investigation. Затем
строится view: сам investigation или явно запрошенное subtree. Группа видна,
только если хотя бы один её atomic member представлен node в этом view.
Остальные members не добавляются в response автоматически, даже если находятся
в соседнем дочернем расследовании того же дерева.

В hypothesis projection группа появляется, если в hypothesis входит хотя бы
один member node. Expand возвращает только members/nodes/edges/evidence текущего
view. Для просмотра полного состава дерева нужен отдельный явный запрос в
контексте корня с соответствующей авторизацией; ответ гипотезы его не подменяет.

Фильтруются не только member IDs, но также evidence IDs, provenance,
explanations и aggregates. Наличие общей tree-scoped group не расширяет
запрошенный MCP view до остального дерева. Per-case capability/ACL в текущем
IR отсутствует; все JWT-пользователи имеют одинаковые права внутри project.

## Write и review policy

Первый PR включает source import, agent proposals, HTTP review, merge, split
и append-only audit. Произвольный generic CRUD и новый rule/scoring engine не
вводятся.

### AgentResultBatch и атомарный import

Существующий `AgentResultBatch` и MCP `add_investigation_agent_results`
расширяются optional массивами entity/event group proposals. Используются
существующие batch-local node refs, why, evidence refs и SOM issue IDs.
Все refs должны принадлежать investigation запроса или атомарно импортироваться
в него; общий root scope не разрешает агенту ссылаться на произвольный sibling
case. Группы и evidence записываются в той же транзакции `ImportContext`.
Результат возвращает group/membership IDs и `root_investigation_id`.
Agent может предлагать все event kinds, но не подтверждать их.

### HTTP actions

Group management явно адресуется через **корневое** расследование. `root_id`
в URL проверяется сервером: investigation существует в проекте, не удалено,
имеет `parent_id IS NULL` и разрешено caller context. Новые endpoints не
предоставляют project-wide поиск groups и не принимают только group ID:

```http
GET  /api/v1/investigations/{root_id}/entity-groups/{group_id}
GET  /api/v1/investigations/{root_id}/event-groups/{group_id}
POST /api/v1/investigations/{root_id}/entity-groups/{group_id}/review
POST /api/v1/investigations/{root_id}/event-groups/{group_id}/review
POST /api/v1/investigations/{root_id}/entity-groups/{target_id}/merge
POST /api/v1/investigations/{root_id}/event-groups/{target_id}/merge
POST /api/v1/investigations/{root_id}/entity-groups/{group_id}/split
POST /api/v1/investigations/{root_id}/event-groups/{group_id}/split
GET  /api/v1/investigations/{root_id}/entity-groups/{group_id}/history
GET  /api/v1/investigations/{root_id}/event-groups/{group_id}/history
```

Group detail/history в этом явном root context охватывают своё дерево;
projection дочернего кейса остаётся ограниченной его view. Review — отдельные
bulk endpoints для entity/event memberships одной группы, expected group/member
versions, reason и operation ID; весь batch применяется или откатывается.
History paginated. Новых MCP tools для review/merge/split в первом PR нет.

Политика статусов:

- explicit source parent/container и source-native stable device identity могут
  быть `confirmed` с `origin=source`;
- deterministic rule становится `confirmed` только для allowlisted strong
  method; иначе `proposed`;
- agent всегда создаёт `proposed`;
- analyst confirm/reject требует reason и optimistic group/member versions;
- аналитик может пересмотреть прежнее решение, сохраняя каждый переход в audit;
- изменение method version не переписывает старое решение молча.

### Merge, split и concurrency

- Merge принимает явный survivor, source group IDs и expected versions.
  Допустимы только один project/root и совместимые family/type/kind. Event
  roles, primary/parent и order задаются явно. Sources становятся superseded,
  memberships/history/provenance не удаляются.
- Split принимает полное явное распределение members по новым groups и
  roles/order. Исходная group становится superseded, создаётся lineage на
  successors. Subjects распределяются однозначно; shared identifiers можно
  указать в нескольких successors явно. Неполная partition, duplicate event
  assignment и нарушение kind invariants откатывают операцию.
- Review проверяет итоговое состояние всей группы. Conflict с confirmed group
  **того же дерева** возвращает `409`; не выполняет скрытый merge. Group
  другого дерева не участвует ни в проверке uniqueness, ни в diagnostics.
- Mutations блокируют строку корневого investigation в одной транзакции;
  это сериализует все group writes данного дерева. Scope/evidence membership проверяется
  до записи. Native FK/unique/check constraints используются там, где выражают
  инвариант; ограничения по active state других таблиц проверяются под root
  lock, без междеревных блокировок.
- SQL FK group/member/operation связывает `project_id` и
  `root_investigation_id`; atomic FK сохраняет project scope. Принадлежность
  atomic evidence активному дереву проверяется через investigation memberships
  в транзакции. Наличие atomic row в проекте само по себе недостаточно.

### Audit и idempotency

Append-only `group_operations` хранит project/root, operation ID, kind,
затронутые group IDs, actor из auth context, reason, expected/result versions,
before/after membership decisions, lineage и безопасный результат операции.
Audit не сохраняет credentials и не пишет аналитические решения в общие
canonical metadata. Для group references обеспечиваются FK в той же области
(для двух families — отдельные typed links/columns, не непроверяемый ID).

Source refresh использует stable source key; agent proposals — `proposal_id`;
review/merge/split — `operation_id`. Ключи дедупликации всегда включают
project/root, payload hash учитывает originating investigation и все refs.
Идентичный retry возвращает прежний результат; тот же ID с другим payload —
`409`. Совпадение ID в другом дереве не раскрывает чужой результат и не создаёт
conflict. Повторный import не отменяет review и не воскрешает superseded group.

## Security и privacy

- `project_id` берётся только из проверенного `X-Project-ID`.
- Каждый lookup/mutation дополнительно ограничен вычисленным или проверенным
  `root_investigation_id`. Чужой project **или независимый root того же
  проекта** отвечает тем же `404`, что и отсутствующая запись.
- Provenance не содержит credentials, auth material, cookies, payload/PCAP или
  vendor raw.
- `origin_ref` для агента/аналитика использует безопасный run/rule/subject ID.
- Agent proposal не может стать confirmed через импорт без отдельной review
  операции.
- Group projection не раскрывает atomic member, которого нет в investigation
  context пользователя.
- Поиск candidates, uniqueness, cache keys, pagination cursors, idempotency,
  lineage и error responses включают tree scope. Новый cache в PR не нужен;
  если он появится, ключ включает также view/hypothesis/filters и версии.
- Нельзя получить группу другого дерева через совпадающий atomic ID, source
  object, group_key, operation/proposal ID, history или merge/split target.
- Scope-changing fields body не могут переопределить scope запроса. Invalid
  reference откатывает весь batch; в ошибку не включается чужой объект.
- Текущие одинаковые IR-права JWT пользователей не меняются; изоляция
  группировок не объявляется новым per-case ACL. Проверка caller capability
  не заменяется принадлежностью тому же проекту или дереву.

## Совместимость и миграции

Проект находится в pre-production режиме без значимых production-данных.
Реализация добавляет блоки таблиц в существующую baseline migration
`db/migrations/investigations/010-initial_schema.sql`, а не создаёт цепочку
маленьких `ALTER TABLE` migrations.

Изменение API начинается с `api/investigations/` и штатной генерации Go/TS
contracts. Старые clients продолжают получать raw graph. `GraphNode.id`
остаётся UUID; virtual string IDs живут только в новом projection DTO.
Shared generated TypeScript contracts обновляются, но `apps/dashboard/**`
не меняется. Новый domain требует регистрации в `transport.API` и router.

На первой итерации backfill не нужен. Existing rows начинают участвовать в
groups после следующего deterministic import/resolve; ручная одноразовая
команда backfill проектируется отдельно, если понадобится до production.

## План реализации одного replacement PR

Новый implementation PR не базируется на `feat/nad-investigation-graph-semantics`.
Он создаётся от актуального `main`; PR #11 не используется как база и не
закрывается автоматически. Scope — весь согласованный backend, без frontend.
Пункты ниже описывают будущую работу, а не уже выполненную реализацию.

### 1. Scope, source contracts и baseline schema

- Добавить модели/таблицы entity groups/members, event groups/members и
  append-only operations/lineage в baseline migration. Везде зафиксировать
  project/root scope, версии, статус, evidence и provenance.
- Реализовать серверное разрешение корня по `parent_id`, scoped lookups,
  scope-safe uniqueness и проверку принадлежности evidence активному дереву.
  Не вводить отдельный контейнер case collection или настраиваемые scope kinds.
- Описать новый `groups` OpenAPI domain: projection, details/history,
  transactional review/merge/split и errors. Расширить `AgentResultBatch` и
  import result optional group proposals/IDs. Выполнить штатную генерацию
  Go/TS; generated файлы вручную не менять.
- Зарегистрировать новый domain в transport. Store остаётся единственным
  местом SQL; новый сервис, очередь, worker и зависимости не нужны.

### 2. Source grouping в import transaction

- В `ImportContext` подключить deterministic grouping после разрешения
  atomic refs и investigation membership, до commit; сохранить hypotheses.
- PT NAD: device anchor только с HostID, temporal identifiers без weak merge.
- Для всех providers: session → composite, finding → correlation, uncovered
  subevent_of → composite. Использовать только scope-eligible source members.
- Реализовать повторный import из root/child в одну group, additive partial
  refresh, сохранение review и маршрутизацию source assertions после
  merge/split. Не искать survivor вне текущего дерева.

### 3. Agent proposals и scoped idempotency

- Расширить оба пути agent results — investigation и hypothesis — с едиными
  правилами проверки local refs, why/evidence и SOM issues.
- Entity/event proposals сохраняются как proposed в том же transaction, что
  evidence. Root вычисляется из investigation запроса, не из agent payload.
- Retry proposal/operation идентичен только внутри своего project/root и с
  тем же payload. Невалидный либо cross-tree reference откатывает весь batch.

### 4. Review, merge, split и audit

- Separate entity/event bulk review с expected versions и reason; scope
  actions — всё дерево. Конфликты того же дерева → `409`, чужие targets → `404`.
- Full merge с explicit survivor и full split с explicit partition/roles/order.
  Сохранить source history, provenance и lineage; не менять atomic facts.
- Root lock защищает state invariants;
  stale versions, неполный batch и конфликт откатываются целиком.
- Details/history superseded groups возвращают successors только своего дерева.
  Actor берётся из auth context, а не из request body.

### 5. Grouped projection и MCP

- Отдельный DTO и endpoint, подтверждённые active groups, детерминированные
  collapse/expand, ambiguity fallback и lossless edge aggregation.
- Scope root вычисляется раньше чтения groups; затем пересечение с текущим
  investigation/subtree/hypothesis view. Child view не расширяется до root.
- Добавить `projection: raw | grouped` в `get_investigation_graph`, default raw.
  HTTP/MCP используют один код; investigation/hypothesis view не расширяется.
- Agent writes остаются через `add_investigation_agent_results`. Review,
  merge/split MCP tools не добавлять.

### 6. Проверки и runtime acceptance

- Unit/store/server/adapter tests для правил, idempotency, review, lineage,
  source refresh, projection и scope matrix ниже.
- DB integration tests выполняются на отдельной disposable DB/schema из
  обновлённой baseline migration. Не применять `db:wipe` к общему стенду.
- Выполнить generation drift, Go tests/build/vet, TS typecheck и dashboard
  compatibility check без изменения его кода. `task check` проверять на
  чистом состоянии согласованных generated artifacts.
- Live E2E — отдельный явно отмеченный результат, не синоним unit/build/CI.

### 7. Handoff и новый PR

- Обновить этот документ по реально реализованному scope, приложить примеры
  raw/projection, proposals, review/conflict и history responses.
- Перед symbol edits выполнить GitNexus impact; для PT NAD decomposition ранее
  выявлен HIGH blast radius по трём resolve flows, перепроверить перед работой.
  Перед commit выполнить detect_changes и scoped diff review.
- Повторить попытку Stele context перед реализацией. Если недоступен, явно
  использовать verified repository contracts/rules, не придумывать product rules.
- Один новый backend PR; никаких изменений Dashboard, закрытия старого PR или
  переноса групп между независимыми расследованиями в этой работе.

## Frontend handoff: структура и идея, без реализации

- Raw graph и UUID-based CRUD остаются как есть. Новый projection DTO имеет
  virtual string IDs, group kind, root scope, member node/edge IDs, provenance,
  ambiguity diagnostics и originating investigation IDs.
- Пользователь работает в текущем case/hypothesis view; toggle grouped/raw и
  expand не добавляют отсутствующее evidence. Неполный вид группы нужно
  обозначить как filtered view, не как полный состав группы.
- Перед review/merge/split UI открывает явный root-scoped detail и сообщает:
  «Решение применяется к корневому расследованию и всем его дочерним кейсам».
  Version conflict требует перечитать состояние; UI не повторяет mutation с
  новой версией без подтверждения пользователя.
- Proposed/rejected не сворачиваются. Ambiguous identifier остаётся отдельным
  узлом; sequence/correlation отображаются как контекст/annotation.
- Не показывать действия share/copy/merge между независимыми деревьями.
  Выбор root scope не является обходом auth/capability.

Probabilistic matching, field-level profile fusion, analyst UI и универсальные
correlation rules остаются вне этого PR.

## Проверки и acceptance criteria

### Entity fixtures

- Один source device UID с изменившимся IP остаётся одной resolved group.
- Два разных strong device IDs с одинаковым IP не объединяются.
- Одинаковый одиночный MAC без strong evidence не вызывает auto-merge.
- Shared/NAT IP может быть identifier у нескольких groups и остаётся видимым
  как ambiguous при collapse.
- Повторный import не создаёт новые group/member rows.
- Одинаковый device UID в двух независимых деревьях создаёт разные groups;
  confirmation/rejection в одном не участвует в matching другого.
- Раздельные temporal assertions не растягиваются через ненаблюдавшийся gap.

### Event fixtures

- Explicit parent + parts образуют одну `composite` group и раскрываются без
  потери atomic events.
- Два похожих события в одном time bucket не становятся `same_event` без
  достаточных identity signals.
- Confirmed duplicate сворачивается в grouped graph, но остаётся виден в
  expanded evidence; raw timeline и его counts не меняются этим PR.
- Sequence сохраняет порядок; correlation не притворяется sequence.
- Один event может участвовать в composite и correlation одновременно.
- Source refresh после rejected/merged/split состояния не теряет решения и не
  воскрешает старую группу; unknown split member остаётся raw с warning.

### Projection fixtures

- `raw` response совпадает с поведением до фичи.
- `grouped → expand` восстанавливает все исходные node/edge/evidence IDs.
- Результат не зависит от порядка SQL rows.
- Group не подтягивает atomic member за пределами явно запрошенного
  investigation/subtree/hypothesis view.
- HTTP и MCP для одинакового projection получают одинаковое membership.
- Dashboard собирается без source edits и продолжает получать прежний raw DTO.

### Tree scope и защита от распространения решений

Тестовый project содержит root A с child A1, grandchild A11 и sibling A2,
а также независимый root B. A и B специально используют одинаковые source
object IDs и некоторые одинаковые atomic entity/event IDs.

- Root A, A1, A11 и A2 получают одинаковый root scope; одинаковый source import
  переиспользует одну group только в этом дереве.
- Review/merge/split через A меняет общую group для A1/A11/A2, но ни одной
  group/member/version/history записи B. Операции B аналогично не затрагивают A.
- В A и B допустимы разные confirmed resolution и same_event decisions для
  одной atomic row; ложного cross-tree uniqueness conflict нет.
- Group ID, member ID, merge target, successor, history cursor и operation ID
  дерева B нельзя использовать для чтения/записи в контексте A. Missing и
  foreign resource неразличимы; payload не раскрывает названия или evidence B.
- Совпадающие proposal/operation IDs в A и B независимы. Retry A никогда не
  возвращает cached/stored response B.
- Source/candidate scans, labels, evidence provenance, confidence и diagnostics
  в A не включают knowledge, известное только B. Poisoned proposal в B не
  меняет A даже при общем identifier/source container.
- View A1 без include_subtree показывает только A1; с include_subtree —
  A1+A11, но не A2 и не B. Root A с include_subtree показывает всё дерево A.
  Hypothesis и agent capability не расширяют membership до дерева.
- Cross-tree/foreign-project refs и невалидный batch откатывают весь import;
  stale versions и неполные partitions откатывают review/merge/split.
- Soft-deleted child не добавляет единственные его evidence в effective view;
  удалённый root не отдаёт группы. История не переезжает в другое дерево.

### Обязательные проверки репозитория

```text
task spec
task gen
task check
task test
task build
task vet
task typecheck
task dashboard-check
git diff --check
```

Live E2E отдельно проверяет на изолированных данных PT NAD import/repeat,
raw/grouped HTTP+MCP, agent proposal → review → merge → split, root/child reuse
и отсутствие утечек в независимый root того же проекта и чужой project.
Проверяется сохранность raw evidence и scope. Статические проверки не считаются
этим E2E; если runtime недоступен, это явно остаётся непроверенным.

## Альтернативы

### Физически объединять `entities`/`events`

Отклонено: необратимые false merges, потеря provenance, сложный split и
невозможность показать разногласия источников.

### Сворачивать только в Dashboard

Отклонено: UI, API и MCP получают разные модели; порядок edges способен выбрать
случайного владельца identifier.

### Одна универсальная таблица групп

Отклонено: polymorphic FK, размытые роли и невозможность выразить разные
ограничения `same_entity`, `same_event`, `composite` и `sequence`.

### Connected components по `has_identifier`

Отклонено: weak signal становится транзитивным merge; shared IP, recycled MAC
или hostname способны склеить несвязанные устройства.

### Общепроектные группы

Отклонено: project может содержать независимые расследования, а shared atomic
facts не дают права распространять аналитические решения между ними.
Подтверждение, poisoning или split в одном кейсе не должны менять другой.

### Отдельная группа для каждого дочернего расследования

Отклонено для выбранного workflow: корень и зависимые расследования должны
использовать общий результат. Scope ограничивается существующим деревом
`parent_id`, без отдельной сущности collection и без произвольного sharing.

## Внешние ориентиры

- [OASIS STIX 2.1](https://docs.oasis-open.org/cti/stix/v2.1/os/stix-v2.1-os.html)
  отделяет grouping общего контекста от identity и relationships.
- [OCSF Schema](https://github.com/ocsf/ocsf-schema) разделяет event classes,
  objects и attributes; observables помогают поиску, но не заменяют факты.
- [Elastic ECS event fields](https://www.elastic.co/docs/reference/ecs/ecs-event)
  различает время occurrence, creation и ingestion.
- [Sigma correlations](https://sigmahq.io/docs/meta/correlations.html)
  различает count, value-count, temporal и ordered-temporal correlation.
- [Splink blocking](https://moj-analytical-services.github.io/splink/demos/tutorials/03_Blocking.html)
  отделяет candidate generation от match decision; качество необходимо
  проверять и на pair, и на cluster уровне.
- [RFC 9724](https://www.rfc-editor.org/rfc/rfc9724.html) описывает MAC address
  randomization, поэтому MAC нельзя использовать как безусловный device ID.

## Зафиксированные решения

1. Group scope — одно корневое расследование со всеми descendants. Остальные
   деревья исключены, даже если project/source/atomic IDs совпадают.
2. Atomic evidence остаётся project-level; grouping не переписывает его
   provenance и не складывает в него общие аналитические conclusions.
3. Роли entity `subject` и `identifier` различаются; weak identifiers не
   означают same_entity. PT NAD auto-anchor требует HostID.
4. Raw `/graph` совместим с текущими clients; grouped имеет отдельный endpoint
   и DTO. MCP default raw.
5. Первый PR — весь backend: schema, storage, source import, agent proposals,
   HTTP review/merge/split/history и MCP read/proposals. Frontend только handoff.
6. Review/merge/split применяются ко всему дереву; проектного review scope,
   автоматического sharing и копирования групп между деревьями нет.
7. Отображение groups и review UX реализуют frontend-коллеги отдельной работой.
