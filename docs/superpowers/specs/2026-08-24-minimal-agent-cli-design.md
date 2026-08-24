# Минимальный CLI jotmd для агентов

## Цель

Добавить минимальный публичный agent API для четырёх операций:

1. найти устойчивое знание;
2. прочитать выбранную Markdown-заметку;
3. создать или безопасно обновить заметку;
4. безопасно переместить устаревшую заметку в system trash.

Markdown-файлы остаются единственным source of truth. CLI не создаёт database,
index, sidecar-файлы или metadata внутри Markdown.

## Команды

```text
jotmd
jotmd [--notes-dir PATH] search [--limit N] QUERY [--json]
jotmd [--notes-dir PATH] get PATH [--json]
jotmd [--notes-dir PATH] write [--if-revision REVISION] PATH [--json]
jotmd [--notes-dir PATH] rm --if-revision REVISION PATH [--json]
```

`jotmd` без subcommand сохраняет существующий TUI. Существующие configuration и
theme команды сохраняются, но не входят в agent API.

Agent subcommands никогда не задают интерактивных вопросов и не запускают
first-run setup. Все note paths задаются относительно notes root; absolute
paths, `..` и symlink traversal запрещены.

Flags принимаются до или после operands, до `--`:

```text
jotmd get projects/foo/pitfalls.md --json
jotmd --notes-dir ./notes search "refresh token redis" --json
```

## Search

```text
jotmd search [--limit N] QUERY [--json]
```

Используется существующий Unicode case-insensitive contiguous substring search
по содержимому Markdown-файлов. Один результат соответствует одной совпавшей
строке и содержит `path`, `line`, `snippet`.

В v1 отсутствуют fuzzy path search, semantic/vector search и неявная token-AND
семантика. Default limit — 20, допустимый диапазон — 1..200. Пустой результат —
успех.

```json
{
  "query": "refresh token redis",
  "results": [
    {
      "path": "projects/foo/pitfalls.md",
      "line": 12,
      "snippet": "Refresh token Redis state must outlive the access token"
    }
  ],
  "truncated": false,
  "stats": {
    "files_scanned": 17,
    "files_skipped": 1
  }
}
```

Search не возвращает content или revision. Для выбранных paths агент вызывает
`get`.

## Get

```text
jotmd get PATH [--json]
```

Читает один regular Markdown-файл. Plain stdout содержит точные bytes без
добавленного newline. JSON содержит только необходимые агенту поля:

```json
{
  "path": "projects/foo/pitfalls.md",
  "revision": "sha256:8f...",
  "content": "# Pitfalls\n..."
}
```

`modified` и `size_bytes` не входят в стабильную v1 schema.

## Revision

Revision нигде не хранится. Она вычисляется из bytes заметки:

```text
revision = "sha256:" + hex(SHA-256(file bytes))
```

Agent воспринимает revision как opaque string и возвращает её без изменений.
Metadata-only изменения conflict не создают. Revision предотвращает silent lost
updates: stale writer получает conflict вместо перезаписи новой версии.

## Write

```text
jotmd write PATH < note.md
jotmd write --if-revision REVISION PATH < note.md
```

| Target | Без condition | С `--if-revision` |
|---|---|---|
| Заметки нет | Создать | Conflict |
| Заметка существует | Conflict | Заменить при совпадении revision |
| Target — directory | Invalid input | Invalid input |
| Parent отсутствует | Not found | Not found |
| Пустой stdin | Создать пустую note | Записать пустую note |

Отдельной `create` нет. Unconditional overwrite и `--force` отсутствуют.

Revision check и atomic pathname replacement выполняются одной domain operation.
Два conditional writes с одной исходной revision дают ровно один success и один
conflict. Ошибка до commit оставляет старую note без изменений.

Plain stdout — новая revision. JSON:

```json
{
  "path": "projects/foo/pitfalls.md",
  "revision": "sha256:91...",
  "created": false
}
```

## Remove

```text
jotmd rm --if-revision REVISION PATH [--json]
```

Перемещает ровно одну Markdown-note в system trash. Revision обязательна. Если
note изменилась после `get`, операция возвращает conflict.

Directory removal, recursive delete, `--force` и `--permanent` отсутствуют.
Ошибка system trash никогда не приводит к permanent fallback. Plain stdout
пуст. JSON:

```json
{
  "path": "projects/foo/obsolete.md",
  "revision": "sha256:31...",
  "action": "trashed"
}
```

## JSON и ошибки

С `--json` successful command пишет один compact JSON object и newline в stdout,
оставляя stderr пустым. При ошибке stdout пуст, а stderr содержит один object:

```json
{
  "error": {
    "code": "revision_conflict",
    "message": "note changed since it was read",
    "details": {
      "path": "projects/foo/pitfalls.md",
      "expected_revision": "sha256:8f..."
    }
  }
}
```

Стабильны `error.code`, JSON field names и enum values. Human-readable messages
не являются стабильным API. Новые JSON fields могут добавляться; consumers
игнорируют неизвестные поля. JSON Lines отсутствует.

| Exit | Значение |
|---:|---|
| 0 | Успех, включая пустой search |
| 1 | Operational или I/O error |
| 2 | Неверная команда, flag, path, revision или input |
| 3 | Note, parent или notes root не найден |
| 4 | Target существует или revision conflict |
| 5 | Permission denied |
| 6 | Не удалось откатить mutation; нужна ручная проверка |

## Agent workflow

```sh
jotmd --notes-dir ./notes search \
  "refresh token redis" --limit 20 --json > /tmp/search.json

jotmd --notes-dir ./notes get \
  projects/foo/pitfalls.md --json > /tmp/pitfalls.json

revision=$(jq -r '.revision' /tmp/pitfalls.json)
jq -rj '.content' /tmp/pitfalls.json > /tmp/pitfalls.md

# Агент добавляет в /tmp/pitfalls.md только устойчивое знание.

jotmd --notes-dir ./notes write \
  projects/foo/pitfalls.md \
  --if-revision "$revision" \
  --json < /tmp/pitfalls.md
```

Exit 4 требует нового `get`, merge с новым content и повторного `write` с новой
revision.

## Намеренно исключено

- `root`, `list`: retrieval workflow покрывают `search` и `get`.
- `create`: отсутствующий path создаёт `write`.
- `mkdir`, `cp`, `mv`, `rename`: стандартные filesystem operations.
- permanent и recursive delete: agent API умеет только revision-safe trash.
- fuzzy/path search flags: не требуются первому durable-knowledge workflow.
- unconditional write: допускает silent lost updates.
- `context`: позже сможет композиционно использовать `search` и `get`.

## Совместимость

- `jotmd` сохраняет TUI и interactive first-run behavior.
- `--notes-dir` сохраняет precedence defaults → config → env → CLI.
- Existing config/theme commands сохраняются.
- `cmd/jotmd/main.go` и имя бинарника не меняются.
- Новая dependency не добавляется.
