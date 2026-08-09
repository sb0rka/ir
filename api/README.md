# OpenAPI contracts

Редактируемые спецификации разделены по сервисам:

```text
gateway/          External tools Gateway
investigations/   Investigations API
```

Внутри каждого сервиса `openapi.yaml` хранит общие настройки, `paths/` —
доменные маршруты, `shared/` — переиспользуемые компоненты. Единая команда
`task gen` собирает оба контракта и обновляет generated-код рядом с его
потребителями.
