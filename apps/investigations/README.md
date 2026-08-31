## Investigations API

Go API сервис `ir-api` расследований Sb0rka.

`ir-api` не является хранилищем потока событий. Страница первичного разбора
читает нормализованные events/entities/relations через Gateway. SOM-агент
работает через единый investigation MCP: читает прикреплённый context и ищет
дополнительное evidence в разрешённых Gateway sources:

- `POST /api/v1/investigations` создаёт расследование и привязывает выбранные
  рабочие пространства SOM, не принимая события и сущности;
- `POST /api/v1/investigations/{id}/context` добавляет выбранный аналитиком
  контекст: `ir-api` сам получает актуальные нормализованные данные из Gateway
  и строит подтверждённую часть графа;
- `POST /api/v1/investigations/{id}/agent-results` принимает один явный batch
  SOM-агента. Только перечисленные `nodes` и `edges` попадают на граф; ноды
  получают origin `agent`, рёбра — status `proposed`;
- `/api/v1/investigations/{id}/edges` поддерживает ручные confirmed analyst
  рёбра, редактирование, evidence и удаление, а `/review` атомарно подтверждает
  или отклоняет предложенные связи с optimistic locking по `version`.

Нормализованный event хранится в `normalized_data`, provenance и source URL —
отдельно, vendor raw payload не сохраняется. Все investigation endpoints требуют
`X-Project-ID`. SOM access token берётся из секрета
`DEMO_SOM_ACCESS_TOKEN` выбранного проекта и кэшируется в памяти; входящий
Bearer используется для чтения Sb0rka Secrets, но не передаётся в SOM.

Удалённый Streamable HTTP MCP опубликован на `/mcp` тем же бинарником `ir-api`
и использует официальный Go SDK. Временный demo-path берёт обычный
пользовательский JWT из `ACCESS_KEY` в Environment variables профиля OpenCode.
В daemon request попадает только ссылка `Bearer {env:ACCESS_KEY}` и
`X-Project-ID` текущего расследования; значение подставляет OpenCode. Этот же
JWT агент может использовать для прямого Gateway REST. Token хранится в
`profiles.json`, действует с правами пользователя и обычно быстро истекает,
поэтому его обновляют непосредственно перед демо и не используют admin token.

MCP-запись принимает `event_id`/`entity_id`, уже прикреплённые к investigation,
существующий `node_id` либо выбранные MCP Gateway records по
`source_code + source_event_id/source_entity_id`. Для remote MCP требуется HTTPS; локальный HTTP
разрешается только через `MCP_ALLOW_INSECURE_HTTP=true`.

Событие в Gateway и `ir-api` находится по паре `source_code + source_event_id`.
Сущность объединяется по `type_code + canonical_key`, а её исходные записи — по
`source_code + source_entity_id`. Эти значения читаемы и позволяют сразу
перейти к исходному инструменту при разборе ошибки.

API страницы расследования включает investigation list/card, timeline и event
detail, entity list/card, graph/nodes/edges, edge review и reference
dictionaries. Списки используют keyset cursor.
