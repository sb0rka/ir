# OpenAPI

`api/investigations/` — источник правды публичного Investigations API.

```text
openapi.yaml      общая информация, теги и Bearer JWT
paths/            шесть доменов и их локальные схемы
shared/           переиспользуемые enum, параметры, ошибки и ответы
```

Домены: `investigations`, `events`, `entities`, `graph`, `reference`, `som`. В
текущем контракте 24 пути и 37 операций. Реализован вертикальный срез страницы
расследования, атомарное сохранение выбранных данных из Gateway и полный edge
CRUD/review; остальные незаявленные операции отвечают `501 not_implemented`.

## Проверка и генерация

```bash
task spec       # api/investigations/bundle.yaml + Redocly lint
task spec-docs  # api/investigations/docs.html
task gen        # Go, TypeScript и заглушки обработчиков
```

`bundle.yaml`, `build/` и `docs.html` — производные файлы и в Git не хранятся.

Redocly `join` объединяет карты `paths` из доменных документов. Поэтому каждый
файл в `paths/` содержит собственные `servers` и `security`; без них итоговая
операция станет публичной или потеряет `/api/v1`.

## Изменение контракта

1. Добавить операцию и локальные схемы в соответствующий файл `paths/`.
2. Общую схему переносить в `shared/` только при втором потребителе.
3. Для нового домена добавить тег в `openapi.yaml`, пакет в `transport.API` и
   регистрацию в `registerDomains`.
4. Запустить `task gen` и `task check`.

## Конвенции

- Auth: Bearer JWT платформы проверяется по issuer/audience/kid/typ из окружения;
  project выбирается обязательным заголовком `X-Project-ID`.
- Ошибка: `{"error":{"code","message","details?"}}`.
- Пагинация: `limit` + `cursor`; ответ содержит массив с именем ресурса
  (`events`, `entities`, `investigations`, `nodes` или `edges`) и
  `next_cursor`, который передаётся в следующий запрос как `?cursor=...`.
  Непагинируемый ответ, содержащий только один однородный список, возвращает
  массив без верхнеуровневой обёртки.
- PATCH и batch review используют `version`; конфликт возвращает `409`.
- Gateway остаётся владельцем поиска больших потоков; Investigations сначала
  создаёт пустое расследование, затем принимает выбранные `context` и
  `agent-results` отдельными запросами. Клиент передаёт только коды источников
  и исходные идентификаторы, а `ir-api` получает данные через Gateway.
- События адресуются по `source_code + source_event_id`, сущности объединяются
  по `type_code + canonical_key` и сохраняют отдельный список исходных ссылок
  `source_code + source_entity_id`.

Покрытие требований и фактическая готовность — в [COVERAGE.md](COVERAGE.md).
