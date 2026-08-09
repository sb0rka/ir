# OpenAPI

`api/investigations/` — источник правды публичного Investigations API.

```text
openapi.yaml      общая информация, теги и Bearer JWT
paths/            пять доменов и их локальные схемы
shared/           переиспользуемые enum, параметры, ошибки и ответы
```

Домены: `investigations`, `events`, `entities`, `graph`, `reference`. В текущем
контракте 20 путей и 32 операции. Это описание интерфейса, не готовности:
доменные обработчики сервиса пока отвечают `501 not_implemented`.

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

- Auth: Bearer JWT платформы с audience `api.local`; project выбирается обязательным
  заголовком `X-Project-ID` и проверяется по `role_bindings`.
- Ошибка: `{"error":{"code","message","details?"}}`.
- Пагинация: `limit` + `cursor`, ответ `items` + `next_cursor`.
- PATCH и batch review используют `version`; конфликт возвращает `409`.

Покрытие требований и фактическая готовность — в [COVERAGE.md](COVERAGE.md).
