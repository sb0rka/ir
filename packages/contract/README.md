# Go-контракт

Пакеты `entities`, `events`, `graph`, `investigations` и `reference` содержат
модели, strict server interfaces и HTTP-роутеры, сгенерированные из `api/investigations/paths`.
Пакет `spec` встраивает собранную OpenAPI-спеку для `/openapi.json` и Swagger.

Файлы `*.gen.go` вручную не редактируются:

```bash
task gen
task check-generated
```
