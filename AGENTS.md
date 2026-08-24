# Development guide

## Workflow

1. Start with a short plan of concrete steps.
2. Check `git status` and inspect the relevant structure, callers, tests, and
   conventions before editing. This worktree may be shared with other agents;
   preserve unrelated and concurrent changes.
3. Make the smallest change required for the goal. Keep existing architecture,
   naming, style, and formatting.
4. Run the fastest relevant tests and linters after editing.
5. Call out compatibility risks explicitly.

Use subagents for independent exploration, implementation, or review when that
meaningfully reduces latency. Avoid delegation for one-step changes and avoid
deep recursive delegation. Combine all results into one concrete conclusion.

## Project map

- `cmd/jotmd`: executable entry point.
- `internal/app`: CLI parsing and application orchestration.
- `internal/config`: configuration, environment overrides, and key maps.
- `internal/notes`: filesystem-backed note operations and watchers.
- `internal/ui`: Bubble Tea model, views, prompts, and actions.
- `internal/theme`: Markdown rendering and TOML themes.
- `themes/gallery`: embedded built-in themes.

Keep platform-specific filesystem behavior in the existing Darwin/Linux files
and preserve their build constraints. Keep configuration and key-binding
templates synchronized with runtime behavior.

## Commands

```sh
just run                # run the application
just build              # build bin/jotmd
just test               # go test ./... -count=1
just check              # formatting, go vet, and tests
just release-check      # release changes only; requires goreleaser
```

Prefer package-level tests while iterating, then run `just check` before handoff
when practical. Do not add dependencies unless explicitly requested.

## Tests

- Put reusable test data in fixtures and reuse existing fixtures where possible.
- Database tests must call the real database. Use-case tests may mock
  infrastructure.
- DTO fields must not have default values; pass every argument explicitly.

Do not add a blank line to `__init__.py` files. Do not use Ponytail for code
review.
