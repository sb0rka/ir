# Правила разработки

## Источник правды

- OpenAPI редактируется в `api/<service>/openapi.yaml`, `paths/` и `shared/`.
- `packages/contract/**/*.gen.go` и `packages/contract-ts/**/*.d.ts` создаёт
  `task gen`; вручную их не менять.
- После изменения контракта запускать `task gen` и коммитить результат вместе
  со спекой.

## Границы кода

- `cmd/ir-api` связывает `server`, `transport` и `store/psql`; `server` зависит
  от `store.Database` и транспортных интерфейсов, а `transport` не импортирует
  `server`.
- HTTP-обработчики работают с SQL через `store.Database`.
- Общий код для `api`, `auth` и `ir-api` сначала проверять в
  `github.com/sb0rka/sb0rka/packages/core`. Не копировать конфиг, логирование,
  PostgreSQL-пул и auth context.
- `authentication` обслуживается HTTP transport: cookies и redirects не входят
  в business Server. Его generated ServerInterface входит в transport.API через
  композицию с DomainAPI; gen-stubs пропускает этот домен.
- Сгенерированный роутер регистрируется в `registerDomains`; один новый домен
  требует обновить также интерфейс `transport.API`.

## Безопасность

- `project_id` берётся только из проверенного `X-Project-ID`; любой запрос по
  идентификатору фильтруется по нему.
- Отсутствующая и чужая запись обе отвечают `404`.
- Внутри разрешённого проекта пользователи имеют одинаковые права IR. Пользовательский JWT имеет
  аудиторию `sb0rka-ir`; локальная сессия, отдельный consent и текущий RBAC API
  проверяются перед каждым project-scoped REST/MCP запросом. `X-Project-ID`
  выбирает проект, но не предоставляет доступ.
- Проектный ключ `s0_ir_agent_…` открывает доменные REST/MCP операции после
  проверки активного SHA-256 и проекта в БД IR на каждом запросе. Управление
  ключом, OIDC и конфигурацией остаётся пользовательским. Ротация сразу
  инвалидирует прежний ключ; исходное значение хранится в Platform Secrets.
- Агентская запись результатов и предложений групп не зависит от SoM issue.
  Автор аудита берётся из проверенного auth context, а не из входного JSON.
- Group decisions дополнительно ограничены корнем `parent_id` дерева:
  `(project_id, root_investigation_id)`. Независимые roots не разделяют группы,
  review, lineage или idempotency. Group scope не расширяет graph view и не
  является новым ACL. Инварианты и frontend API — в [docs/grouping.md](docs/grouping.md).
- Gateway определяет источники по серверной конфигурации backend и credentials
  проекта. Пользовательский путь проверяет grants/RBAC; агентский проверяет тот
  же Bearer в БД IR и читает локальную AES-GCM-копию конфигурации без Auth или
  Platform calls. Копия обновляется при сохранении/перечитывании через IR;
  `IR_GATEWAY_CONFIGURATION_KEY` общий для IR/Gateway и независим от session/OIDC
  ключей. Vendor URL и NAD store IDs остаются серверными параметрами.

## Проверки

```bash
task spec
task check
```

Для новой логики добавлять тесты. Минимум для transport/store — успешный путь,
невалидный ввод и чужой проект.

## Документация

README и AGENTS содержат только актуальные правила первого уровня. Подробные
решения размещаются рядом с кодом или в `docs/`. Комментарий объясняет причину
или инвариант, а не пересказывает имя функции.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **ir** (12456 symbols, 30466 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/ir/context` | Codebase overview, check index freshness |
| `gitnexus://repo/ir/clusters` | All functional areas |
| `gitnexus://repo/ir/processes` | All execution flows |
| `gitnexus://repo/ir/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
