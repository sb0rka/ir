## Investigations API

Go API сервис `ir-api` расследований Sb0rka.

`ir-api` не является хранилищем потока событий. Страница первичного разбора
читает нормализованные events/entities/relations через Gateway. SOM-агент
работает только с уже прикреплённым investigation context через MCP:

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
и использует официальный Go SDK. Перед запуском SOM `ir-api` обменивает
пользовательский access JWT в Sb0rka Auth на четырёхчасовой `agent+jwt` с
audience `ir-mcp`, фиксированными scopes и одной investigation. Daemon добавляет
`Authorization: Bearer ...` только в OpenCode конкретного environment, не меняя
глобальную MCP-конфигурацию и не сохраняя token в SQLite. Agent JWT не
передаётся в Gateway, Platform API или Secrets.

MCP-запись принимает только `event_id`/`entity_id`, уже прикреплённые к этой
investigation, либо существующий `node_id`. Human REST-путь сохраняет импорт
новых source refs через Gateway. Для remote MCP требуется HTTPS; локальный HTTP
разрешается только через `MCP_ALLOW_INSECURE_HTTP=true`.

Событие в Gateway и `ir-api` находится по паре `source_code + source_event_id`.
Сущность объединяется по `type_code + canonical_key`, а её исходные записи — по
`source_code + source_entity_id`. Эти значения читаемы и позволяют сразу
перейти к исходному инструменту при разборе ошибки.

API страницы расследования включает investigation list/card, timeline и event
detail, entity list/card, graph/nodes/edges, edge review и reference
dictionaries. Списки используют keyset cursor.
