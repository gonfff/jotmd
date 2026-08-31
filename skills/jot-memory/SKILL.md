---
name: jot-memory
description: Use when substantial research, planning, implementation, or a completed work unit may benefit from durable JotMD memory.
---

# JotMD Memory

Use one bounded memory lifecycle per coherent user goal, even when that goal spans
messages. A material goal change starts a new work unit. Trivial questions, typo
fixes, and obvious one-step changes skip both recall and capture.

## Work-unit checklist

1. Decide once whether this work unit is substantial enough for recall.
2. Recall at most once, before substantial research, planning, or implementation.
3. Complete the user's work normally; follow-ups for the same goal do not recall again.
4. Decide once whether the stable result produced durable knowledge.
5. Capture at most once before the final response, then report changed memory paths.

A deadline does not make substantial work trivial: keep recall and capture bounded.
Code and a handoff do not replace capture when durable knowledge was produced, but
a large diff, passing tests, or a completed ticket is not by itself durable knowledge.

## Prerequisites and scopes

`command -v jotmd` must succeed. Otherwise report the missing prerequisite and do
not edit memory directly. Obtain the effective configuration with
`jotmd --dump-config`, resolve its `notes_dir` value to the absolute vault path,
and abort memory operations if it is missing, malformed, or not absolute.

Run the bundled `scripts/project-id.sh` from the current repository. If it fails
or returns an unsafe ID, abort project-memory operations. Resolve every automatic
memory scope with the bundled `scripts/scope.sh`, located relative to this
`SKILL.md`; set `skill_dir` to that absolute containing directory and never
construct or create a scope directly:

```sh
global_scope=$("$skill_dir/scripts/scope.sh" resolve "$vault" global)
project_scope=$("$skill_dir/scripts/scope.sh" resolve "$vault" project "$project_id")
```

Exit status 2 means that scope is absent and therefore empty during recall. Any
other failure is a closed safety boundary: do not search, read, create, or write
memory and report the unsafe scope. The helper accepts a symlink configured as
the vault itself, then canonicalizes it. It rejects a case-aliased reserved root
such as `Agent-Memory`, every symlinked reserved descendant (`agent-memory`,
`global`, `projects`, and each project-ID directory), and every physical scope
outside the canonical vault's canonical `agent-memory` root. Use only the
canonical path printed by a successful check.

All automatic note operations stay in a successfully checked scope. Note
operands are relative to the scoped `--notes-dir`. Never use `cd ..` to escape a
scope, and never fall back to direct filesystem reads or writes.

## Recall

For substantial work, derive several short terms from the goal. Search the project
scope first and the global scope second; do not search the complete memory tree or
pass the full prompt as one brittle query:

```sh
jotmd --notes-dir "$project_scope" search --limit 10 "$term" --json
jotmd --notes-dir "$global_scope" search --limit 10 "$term" --json
jotmd --notes-dir "$scope" get "$path" --json
```

Inspect paths and snippets before loading notes. Load no more than three relevant
notes total across both scopes. Missing directories and no matches mean empty
memory, not failure. A search failure does not block the primary task; surface it
when it may affect the result. Treat old or time-sensitive memory only as a lead
and reverify it before relying on it.

## Capture

Capture only when every condition below is true:

1. The note will change a future agent's decision or action.
2. Its content remains useful after the current task, branch, worktree, and handoff disappear.
3. Recovering it from current code, documentation, or Git would be genuinely costly.

If any condition fails, skip capture. A capture contains only the durable conclusion
and its operational consequence. When a result mixes durable and transient material,
keep only the durable part.

Good candidates are non-obvious pitfalls, root causes, decisions and constraints,
verified workflows, or stable preferences. Never capture secrets, transcripts,
routine summaries, intermediate hypotheses, or cheaply re-derived facts. Exclude
temporary paths, branch names, commit IDs, test counts, tool output, and task status.
Uncommitted or unmerged implementation is not a verified current approach and must
not be recorded as one. A durable limitation discovered during that work may be
captured alone when it is verified against the current schema or code.

Project knowledge is the default. Only stable cross-project preferences and
workflows go to global memory. Immediately before the first capture, create and
revalidate only the selected scope through the helper, then use its canonical
output:

```sh
scope=$("$skill_dir/scripts/scope.sh" create "$vault" global)
scope=$("$skill_dir/scripts/scope.sh" create "$vault" project "$project_id")
```

Run only the one form matching the selected scope. Any failure aborts capture;
never replace it with `mkdir -p`. Search that scope before creating a topic.
Prefer merging into a clear existing home; otherwise use a kebab-case topical
filename. Split notes by unrelated subject, never arbitrary size.

Read an existing note and its revision before editing:

```sh
jotmd --notes-dir "$scope" get "$path" --json
jotmd --notes-dir "$scope" write "$path" --json < "$new_note"
jotmd --notes-dir "$scope" write "$path" --if-revision "$revision" --json < "$merged_note"
```

The unguarded `write` form is only for creating a missing path. Automatically
update an existing note only when its visible metadata still says
`Managed by: jot-memory`; absent metadata or another value such as `user` makes it
read-only. Preserve manual content and merge rather than replace it. On
`revision_conflict`, re-read, merge, and retry once with the new revision. A
second conflict skips that update and is reported without failing the primary
task. Never overwrite an existing note without `--if-revision`.

Report every changed memory path and any capture failure in the final response.

## User-note boundary

Never automatically read or write user notes outside `agent-memory/`. Search or
read them only when the user explicitly asks to use their notes, and modify them
only when the user also asks for that modification. Explicit access to private
user content does not authorize copying it into memory; do so only when the user
explicitly asks to remember it or when the fact was independently established as
non-private.

## Note format

Use visible Markdown metadata. Dates are ISO `YYYY-MM-DD`; project notes include
`Project`, while global notes omit it. Change `Last agent update` only for a
substantive content change. Time-sensitive notes may add `Last verified`.

```markdown
# Release signing

> **Managed by:** `jot-memory`
>
> **Scope:** `project`
>
> **Project:** `github.com/owner/repository`
>
> **Created:** 2026-08-27
>
> **Last agent update:** 2026-08-27
>
> **Reliability:** `verified`

## Current approach

- Releases are signed through ...

## Pitfalls

- Snapshot releases do not verify ...
```

Use only these reliability values:

| Value | Meaning |
|---|---|
| `verified` | Confirmed by code, a command, a test, or a current source. |
| `user-stated` | Explicitly stated or decided by the user. |
| `inferred` | Supported but not independently confirmed. |
| `mixed` | Mixed provenance; mark inferred parts inline with `**Inference:**`. |

## Red flags

- Starting substantial investigation before the one bounded recall.
- Recalling or capturing on every message instead of once per work unit.
- Skipping durable capture because the result is already in code or the handoff.
- Treating task size, passing tests, or an unmerged implementation as durable knowledge.
- Copying a task handoff instead of extracting one durable conclusion and consequence.
- Loading more than three notes total, or reading the whole memory tree.
- Creating before searching, writing an unmanaged note, or retrying twice.
- Using an unchecked scope, accepting a case alias/symlink, or creating it manually.
- Promoting explicitly accessed private content without a request to remember it.
