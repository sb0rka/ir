# Рекомендации по написанию SOM issue для IR-агентов

Цель issue — одно проверяемое действие, которое модель может выполнить
через `import_entity_events` или узкий набор MCP-вызовов. Не смешивайте в одном
тексте несколько независимых расследований.

## Обязательные части

1. **Цель одним предложением.** Что должно появиться на графе после успеха.
2. **Подписанные идентификаторы.** Никогда не оставляйте «голый» UUID.
   - `entity_id: b71336ed-25f7-42fa-840a-688ceb087c74`
   - `account: dkrylova\administrator` — ровно один backslash
   - при необходимости `source_code: pt-maxpatrol-siem`
3. **Окно времени.** Явный интервал вокруг интересующих событий, например
   `2025-10-22 .. 2025-10-24`. Без окна агент уходит в широкий поиск.
4. **Источник по capability.** Для Windows-аккаунтов / процессов / auth —
   SIEM (`pt-maxpatrol-siem`). NAD — только для сетевых сущностей.
5. **Стратегия отбора: `filter` и `sort`.** Лимит без фильтра означает
   «N самых поздних сырых событий». Для активного аккаунта это срез в сотни
   миллисекунд, почти целиком из шума (закрытие дескрипторов, завершение
   процессов). Поэтому всегда говорите, *какие* события нужны:
   - `filter: correlation_name != null` — только сработавшие правила (алерты);
   - `filter: category.high = "Credential Access"` — техника по категории;
   - `filter: correlation_name = "mimikatz_command"` — конкретное правило;
   - `filter: subject.process.name = "chisel.exe"` — активность процесса;
   - `sort: time asc` — начало активности, `time desc` (по умолчанию) — конец.
   Предикат — один PDQL-фрагмент без `|`, он объединяется с условием по entity.
6. **Лимит.** Сколько событий достаточно: «до 50 событий». Лимит имеет смысл
   только вместе с фильтром или с ожидаемым `events_total` того же порядка.
7. **Ожидаемые рёбра.** Либо «по ролям события (`actor`, `target`, …)», либо
   конкретный `relation_code`.
8. **Критерий приёмки через `events_total`.** Инструмент возвращает
   `events_total` (сколько событий подходит под фильтр во всём окне),
   `events_found`, `events_imported` и `truncated`. Критерий должен опираться
   на них, а не на `> 0`:
   - `events_imported == events_found`;
   - `events_imported == min(limit, events_total)`; если `truncated=true`,
     отчёт называет `events_total` и объясняет, какой срез импортирован;
   - отчёт цитирует `events_total` / `events_found` / `events_imported`
     дословно; на графе есть узлы событий и proposed-рёбра к entity.

## Как выбрать фильтр под цель

| Цель issue | `filter` | `sort` | `limit` |
|---|---|---|---|
| Какие правила сработали по аккаунту | `correlation_name != null` | `time asc` | 50–100 |
| Только одна техника | `category.high = "Credential Access"` | `time asc` | 20–50 |
| Одно правило подробно | `correlation_name = "lsass_memory_dump"` | — | 10 |
| Что делал конкретный процесс | `subject.process.name = "chisel.exe"` | `time asc` | 50 |
| С чего началась активность | без фильтра | `time asc` | 20 |
| Зачистка в конце | `action = "stop"` или `action = "remove"` | `time desc` | 20 |

Если распределение неизвестно, сделайте разведочный issue без записи:
«вызови `gateway_aggregate_events` по `correlation_name` для этого аккаунта и
окна, отчитайся списком правил и счётчиками, ничего не пиши на граф». По его
результату формулируются целевые issue с точными фильтрами.

## Одно действие — один issue

Не просите «алерты и сырые события вокруг них» в одном задании. Разбивайте:

1. Алерты: `correlation_name != null`, `time asc`, до 100.
2. Контекст конкретного алерта: `subject.process.name = "…"` в узком окне
   ±5 минут вокруг его `occurred_at`.
3. Сырой хвост или начало — отдельно, если действительно нужно.

Каждый следующий issue ссылается на числа из предыдущего отчёта.

## Чего не делать

- Не просить искать Windows-аккаунт в NAD.
- Не подставлять IR UUID в gateway-фильтры или в `source_entity_id`.
- Не удваивать backslash вручную (`dkrylova\\administrator` в тексте задачи —
  риск, что модель утроит escape в JSON).
- Не требовать «все события за всё время».
- Не задавать `limit` без `filter` для аккаунта с тысячами событий — получите
  последние N миллисекунд окна.
- Не писать критерий `events>0 / nodes>0 / edges>0`: он выполняется одним
  событием и не отличает 50 из 50 от 50 из 17 000.
- Не просить агента «подтвердить», что сущность уже на графе, без критерия.

## Шаблон

```text
Найди <какие именно> events для entity и добавь их на граф investigation
как nodes с proposed edges по ролям события.

entity_id: <uuid>
account: <DOMAIN\user>          # один backslash
source: pt-maxpatrol-siem
time_range: <YYYY-MM-DD> .. <YYYY-MM-DD>
filter: <PDQL-предикат>          # например correlation_name != null
sort: time asc | time desc
limit: до N событий
edges: по ролям события (actor/target/…)

Критерий приёмки:
- один вызов import_entity_events с этими filter/sort/limit успешен;
- events_imported == events_found и events_imported == min(limit, events_total);
- если truncated=true — отчёт называет events_total и объясняет, какой срез
  импортирован (первые/последние N по времени);
- на графе есть узлы событий и proposed-рёбра к этой entity;
- финальный отчёт цитирует events_total / events_found / events_imported
  дословно. Если записи не было — «nothing was written».
```

## Пример

Плохо:

```text
Мне необходимо чтобы ты нашел связанные events для entity
dkrylova\administrator
b71336ed-25f7-42fa-840a-688ceb087c74
и добавил их на граф в виде nodes совместно с edges
```

Первая правка (подписанные идентификаторы, окно, лимит) сделала задание
выполнимым, но без фильтра дала 50 последних сырых событий: срез в 281 мс из
17 463 подходящих, 19 из 50 — «закрыл дескриптор объекта», 1 алерт из 186.
Механически всё верно, но для расследования почти бесполезно.

Хорошо:

```text
Найди сработавшие правила корреляции по аккаунту и добавь их на граф как
nodes с proposed edges по ролям события.

entity_id: b71336ed-25f7-42fa-840a-688ceb087c74
account: dkrylova\administrator
source: pt-maxpatrol-siem
time_range: 2025-10-23T15:30:00Z .. 2025-10-23T17:00:00Z
filter: correlation_name != null
sort: time asc
limit: до 100 событий
edges: по ролям события (actor/target/…)

Критерий приёмки:
- один вызов import_entity_events с этими filter/sort/limit успешен;
- events_imported == events_found и events_imported == min(100, events_total);
- если truncated=true — отчёт называет events_total и объясняет, что
  импортированы первые N алертов по времени;
- на графе есть узлы событий и proposed-рёбра к этой entity;
- финальный отчёт цитирует events_total / events_found / events_imported
  дословно и перечисляет уникальные correlation_name среди импортированных.
  Если записи не было — «nothing was written».
```

Следующие issue по результату первого:

```text
Добавь на граф активность процесса chisel.exe от этого аккаунта.

entity_id: b71336ed-25f7-42fa-840a-688ceb087c74
account: dkrylova\administrator
source: pt-maxpatrol-siem
time_range: 2025-10-23T16:00:00Z .. 2025-10-23T16:45:00Z
filter: subject.process.name = "chisel.exe" and correlation_name = null
sort: time asc
limit: до 30 событий
edges: по ролям события
include_participants: true      # нужны узлы хоста и назначения

Критерий: тот же, плюс отчёт называет events_total и подтверждает, что среди
импортированных есть событие запуска (action = "start").
```

```text
Добавь на граф доступ к LSASS от этого аккаунта.

entity_id: b71336ed-25f7-42fa-840a-688ceb087c74
account: dkrylova\administrator
source: pt-maxpatrol-siem
filter: category.high = "Credential Access"
time_range: 2025-10-23T16:40:00Z .. 2025-10-23T16:45:00Z
sort: time asc
limit: до 20 событий
```

IR при запуске сам допишет `investigation_id`, `som_issue_id`, блок
`Resolved IR references` и suggested `time_range` из таймлайна investigation.
Задача аналитика — дать подписанный entity UUID, однозначное значение
аккаунта, узкое окно времени и фильтр, который отвечает на вопрос issue.
Инструмент честно скажет, сколько событий подошло; какие из них важны —
решает формулировка задания.
