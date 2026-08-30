# Server

`server` реализует сгенерированные `StrictServerInterface` по одному файлу на
домен. Ещё не реализованные операции отвечают `501 not_implemented`.

После изменения OpenAPI команда `task gen` добавляет недостающие методы. Она не
удаляет старый метод после переименования `operationId`; такой метод удаляется
вручную.

Реальный обработчик должен:

1. получить identity и project scope из контекста;
2. провалидировать входные данные операции;
3. вызвать `store.Database`, передав `project_id` первым аргументом;
4. вернуть типизированный ответ контракта или `httperr.Error`.

Чужая запись не отличается от отсутствующей и возвращает `404`. Для store и
transport обязательны тесты на пересечение границы проекта.

`/mcp` использует те же handler-методы, но два режима авторизации: human
`access+jwt` + `X-Project-ID` и delegated `agent+jwt`. Во втором режиме project,
investigation и scopes берутся только из подписанных claims; agent bearer нельзя
класть в `socctx.Bearer`, чтобы он не мог уйти в Gateway/Platform/Secrets.
Gateway tools передают его только внутреннему Auth exchange; полученный
короткоживущий access JWT используется server-to-server и агенту не виден.
