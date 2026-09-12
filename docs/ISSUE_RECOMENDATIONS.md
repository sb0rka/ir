# Рекомендации по написанию SOM issue для IR-агентов

Issue — одно проверяемое действие. Агент читает текст сверху вниз, поэтому
порядок блоков фиксирован:

1. **Задание** — одно предложение: что найти и что должно появиться на графе.
   По нему агент выбирает инструмент.
2. **Поиск** — параметры MCP-вызова в виде `ключ: значение`. Агент копирует
   их в аргументы дословно, не интерпретируя.
3. **Обработка** — что агент делает сам, без MCP: отбор по содержимому
   `attributes`, дедупликация, подсчёт. Пусто, если отбор не нужен.
4. **Запись и отчёт** — чем писать и какие числа процитировать.

Каждый блок отвечает на один вопрос; пояснения и мотивацию в issue не
включайте — они уводят слабую модель в рассуждения.

## Два пути

| Задание | Через MCP | Сам |
|---|---|---|
| События одной entity (аккаунт, хост, процесс) | `import_entity_events` с `filter`/`sort`/`limit` — один вызов, он же пишет на граф | — |
| События по полям SIEM с отбором по содержимому | `gateway_search_events` (`include_attributes: true`, `limit: 100`, все страницы по `next_cursor`) → `add_investigation_agent_results` только для отобранных | grep по сохранённому выводу: `cmdline`, `chain`, `object.name`, `object.path`, `text` |

Если задание — «найди X и добавь только те, где Y», это второй путь: поиск
даёт X целиком, Y агент проверяет локально, пишет подмножество.

Всё, что пишет агент, остаётся **proposed** до решения аналитика, а решение
принимается по рёбрам. Поэтому у каждого event-узла должно быть хотя бы одно
ребро к entity (хост или аккаунт из задания); узел без ребра сервер отклоняет.
В issue укажите, к какой entity привязывать события.

## Правила значений

- `time_range` — явный интервал в RFC3339 с `Z` или смещением:
  `2025-10-23T15:00:00Z .. 2025-10-23T17:00:00Z`. Без окна агент берёт
  таймлайн investigation ±24h и обязан назвать это в отчёте.
- Идентификаторы подписаны: `entity_id: <uuid>`, `account: dkrylova\administrator`
  (один backslash), `source: pt-maxpatrol-siem`. Голый UUID запрещён.
- `filter` — PDQL-предикат: `field = "value"`, `field != "value"`,
  `field contains "value"`, `field in ("a", "b")`, `field is null`; условия
  через `and`/`or`, значения в двойных кавычках. Только allowlist-поля:
  `event_src.host`, `src.ip`, `dst.ip`, `subject.account.name`,
  `subject.process.name|cmdline|chain`, `object.process.name|cmdline|chain`,
  `object.name`, `object.path`, `text`, `action`, `importance`,
  `correlation_name`, `category.high`. Флаги ответа (`truncated`, `total`)
  полями не являются — вместо «truncated: false» пишите «прочитать все
  страницы».
- Источник по capability: аккаунты, процессы, auth — SIEM; NAD — только
  сетевые сущности.
- `limit` без `filter` для активного аккаунта — последние N миллисекунд
  шума. Всегда говорите, *какие* события нужны.

Типовые фильтры для одной entity:

| Цель | `filter` | `sort` |
|---|---|---|
| Сработавшие правила | `correlation_name != null` | `time asc` |
| Одна техника | `category.high = "Credential Access"` | `time asc` |
| Активность процесса | `subject.process.name = "chisel.exe"` | `time asc` |
| Зачистка в конце | `action = "stop" or action = "remove"` | `time desc` |

Если распределение неизвестно — разведочный issue без записи: «вызови
`gateway_aggregate_events` по `correlation_name` для этой entity и окна,
отчитайся счётчиками, ничего не пиши на граф».

## Шаблон A — одна entity

```text
Задание: найди <какие именно> события для entity и добавь их на граф
как nodes с proposed edges по ролям события.

Поиск (import_entity_events):
entity_id: <uuid>
account: <DOMAIN\user>
source: pt-maxpatrol-siem
time_range: <RFC3339> .. <RFC3339>
filter: <PDQL-предикат>
sort: time asc | time desc
limit: N

Запись и отчёт:
- один вызов import_entity_events с этими параметрами;
- процитировать events_total / events_found / events_imported дословно;
- events_imported == events_found и == min(N, events_total); если
  truncated=true — назвать events_total и какой срез импортирован;
- если записи не было — «nothing was written».
```

## Шаблон B — фильтр по полям SIEM с отбором

```text
Задание: найди <какие события> и добавь на граф только те, где <критерий>.

Поиск (gateway_search_events, source pt-maxpatrol-siem):
time_range: <RFC3339> .. <RFC3339>
filter: <field> = "<value>" and <field> contains "<value>"
include_attributes: true
limit: 100, читать все страницы по next_cursor

Обработка (локально, без MCP):
- в attributes (<поля>) найти <признак>;
- отобрать только такие события.

Запись и отчёт:
- add_investigation_agent_results: только отобранные события, events[] +
  nodes[] с event_ref и why + для каждого события proposed-ребро
  (actor/mentions) к entity <тип: значение>;
- в отчёте: total поиска, сколько событий прочитано, сколько отобрано и по
  какому признаку, их source_event_id; если ничего не подошло —
  «nothing was written».
```

## Пример

Плохо — пояснения вперемешку с параметрами, флаг ответа выдан за фильтр,
нет окна:

```text
Необходимо найти события с процессом загрузки утилит через фильтр contains.
Используй фильтры ниже для запросов events:
event_src.host: dkrylova.plat.form
subject.process.chain: splunkd.exe
object.process.chain: splunkd.exe
Если событий с фильтром меньше 1000, то используй: truncated: false
Проанализируй список загруженных событий и добавь на граф только те, где
присутствовала загрузка потенциально вредоносного приложения (например Mimikatz).
```

Хорошо:

```text
Задание: найди на хосте dkrylova.plat.form события, где процессы из цепочки
splunkd.exe загружали утилиты, и добавь на граф только те, где загружено
потенциально вредоносное ПО (например, Mimikatz).

Поиск (gateway_search_events, source pt-maxpatrol-siem):
time_range: 2025-10-23T15:00:00Z .. 2025-10-23T17:00:00Z
filter: event_src.host = "dkrylova.plat.form" and subject.process.chain contains "splunkd.exe" and object.process.chain contains "splunkd.exe"
include_attributes: true
limit: 100, читать все страницы по next_cursor (ожидается менее 1000 событий)

Обработка (локально, без MCP):
- в attributes (subject.process.cmdline, object.process.cmdline, object.name,
  object.path, text) найти mimikatz / Invoke-Mimikatz и другие известные
  инструменты атаки;
- отобрать только такие события.

Запись и отчёт:
- add_investigation_agent_results: только отобранные события, events[] +
  nodes[] с event_ref и why + для каждого события proposed-ребро mentions
  к entity host: dkrylova.plat.form;
- в отчёте: total поиска, сколько событий прочитано, сколько отобрано и по
  какому признаку, их source_event_id; если ничего не подошло —
  «nothing was written».
```

## Чего не делать

- Смешивать несколько расследований в одном issue.
- Объяснять мотивацию или давать альтернативы («если …, то попробуй …»).
- Подставлять IR UUID в gateway-фильтры или `source_entity_id`.
- Удваивать backslash в аккаунте.
- Просить «все события за всё время» или `limit` без `filter`.
- Писать критерий `events > 0`: он не отличает 50 из 50 от 50 из 17 000.
- Искать Windows-аккаунты в NAD.

IR при запуске дописывает `investigation_id`, `som_issue_id` и блок
`Resolved IR references`; окно времени, источник и фильтр — задача аналитика.
