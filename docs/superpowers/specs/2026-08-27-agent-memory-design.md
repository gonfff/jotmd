# Agent Memory Design

**Status:** approved in design review

**Date:** 2026-08-27

## Summary

JotMD will provide a portable `jot-memory` Agent Skill that gives Codex,
Claude Code, and OpenCode a shared, filesystem-backed memory. The memory is
plain Markdown inside the user's existing vault, remains readable and editable
through JotMD, and uses the existing JotMD agent CLI for search, reads, and
writes.

Memory is considered once per coherent work unit rather than once per chat
message. The agent recalls relevant notes before substantial research or
implementation and captures durable knowledge once after the work completes.
The user does not need to invoke the skill manually.

## Goals

- Keep agent memory human-readable and editable as ordinary Markdown.
- Share global memory between the same user's agents and projects.
- Share project memory between worktrees and clones of the same Git remote.
- Keep the user's other vault notes private from automatic recall.
- Let the agent decide whether recall or capture is useful for a work unit.
- Retrieve only a few relevant notes instead of loading the whole memory.
- Distribute one canonical skill to Codex, Claude Code, and OpenCode.
- Reuse the existing `jotmd search`, `get`, and revision-checked `write` CLI.

## Non-goals

- Team or multi-user memory synchronization.
- Background capture without an active agent task.
- Guaranteed per-message hooks in every agent platform.
- Embeddings, a vector database, semantic search, or an MCP server.
- Encryption or protection after the user grants an agent unrestricted host
  filesystem access.
- Automatic access to notes outside `agent-memory/`.
- Automatic summarization of every conversation or task transcript.
- A new JotMD CLI command solely for creating memory directories.

## Terminology

- **Work unit:** one coherent user goal that may span multiple messages,
  research steps, edits, and clarifications.
- **Recall:** one retrieval pass before substantial research, planning, or
  implementation.
- **Capture:** one persistence pass after a work unit reaches a stable result.
- **User notes:** every vault entry outside `agent-memory/`.
- **Agent memory:** Markdown stored below `agent-memory/`.

## Architecture

The feature has three deliberately small parts:

1. `jot-memory` describes the recall, routing, capture, and safety workflow.
2. The existing JotMD agent CLI is the only interface used to search, read, and
   change note contents.
3. The JotMD TUI adds a visibility filter for the reserved `agent-memory/`
   directory.

There is no memory daemon or additional index. Filesystem state remains the
source of truth.

## Vault Layout

```text
<vault>/
  agent-memory/
    global/
      preferences.md
      tooling.md
      workflow.md

    projects/
      github.com/
        owner/
          repository/
            architecture.md
            decisions.md
            pitfalls.md

  work/
  personal/
  any-other-user-note.md
```

`agent-memory/` is a reserved top-level name. `global/` is a separate directory
so global searches cannot recursively include every project's memory. Notes are
organized by durable topic; the example filenames are conventions, not a fixed
schema.

Everything outside `agent-memory/` is a user note and is outside the automatic
memory scope.

### Project identity

For a Git repository with an `origin` remote, the skill normalizes the remote
to a filesystem-safe host and repository path. For example, both of these map
to `github.com/owner/repository`:

```text
git@github.com:owner/repository.git
https://github.com/owner/repository.git
```

The host is lowercased, a trailing `.git` is removed, and unsafe path-segment
characters are replaced with `-`. Empty, `.` and `..` segments are forbidden.
When sanitizing a segment changes it, a short hash of the canonical remote is
appended to prevent two different remotes from collapsing to the same path.
This identity makes normal clones and Git worktrees share project memory.

When there is no `origin`, the fallback is
`local/<repository-name>-<short-hash>`, where the hash is derived from the
canonical Git common directory. Worktrees still share memory; independent
clones without a common remote cannot be matched reliably and remain separate.

## Note Format

Agent-created notes are ordinary Markdown with a visible metadata block:

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
- Run `just release-check` before tagging.

## Pitfalls

- Snapshot releases do not verify ...
```

Dates use ISO `YYYY-MM-DD`. Project notes include `Project`; global notes omit
it. `Last agent update` changes only after a substantive content change.
Time-sensitive notes may also contain `Last verified`.

Allowed reliability values are:

- `verified`: confirmed through code, a command, a test, or a current source;
- `user-stated`: explicitly stated or decided by the user;
- `inferred`: a supported inference that has not been independently confirmed;
- `mixed`: the note contains claims with different provenance; inferred parts
  must be identified inline with `**Inference:**`.

Numeric confidence is not used. It suggests precision the system cannot
justify.

The agent may automatically update a note only while its metadata says
`Managed by: jot-memory`. Removing that line or changing the value to `user`
makes the note read-only to automatic capture. Manual edits are otherwise
preserved and merged rather than replaced.

## Recall Lifecycle

Recall happens at most once at the beginning of a work unit and before the
agent begins substantial research, planning, or implementation. A follow-up
within the same goal does not trigger another recall. A material change of goal
starts a new work unit and permits another recall.

Trivial questions, typo fixes, and obvious one-step changes may skip recall.

Recall performs these steps:

1. Resolve the effective JotMD vault and current project identity.
2. Derive a small set of short search terms from the goal rather than passing
   the full user prompt as one query.
3. Search the current project scope first and the global scope second with
   `jotmd ... search --json`.
4. Inspect result paths and snippets.
5. Load no more than three relevant notes with `jotmd ... get --json`.
6. Continue normally when the directories or matching notes do not exist.

The existing search is the MVP retrieval engine. It provides fuzzy path
matching and case-insensitive substring matching in contents. The skill should
issue multiple short searches such as `signing`, `release`, and `goreleaser`
when one long phrase would be brittle.

Recall never reads the complete memory tree merely to find context.
Time-sensitive or old information is treated as a lead and reverified before
it controls the task result.

## Capture Lifecycle

Capture happens once when a substantial work unit reaches a stable result. It
is skipped when no durable knowledge was produced.

Knowledge is eligible when it is verified or clearly labeled, likely to help a
future task, not obvious from the repository itself, and costly enough to
rediscover. Typical examples include:

- a root cause found during a difficult diagnosis;
- an architecture decision and its constraint;
- a verified command or workflow;
- a non-obvious project convention or pitfall;
- a stable user preference.

The skill does not capture task transcripts, intermediate hypotheses, routine
diff summaries, secrets, or facts that are cheap to re-derive.

Capture performs these steps:

1. Route the knowledge to project memory unless it clearly applies across
   projects; stable user preferences and cross-project workflows go to global
   memory.
2. Search the chosen scope for an existing topical note.
3. Prefer updating that note over creating a similar file.
4. Read the note and its revision with `get --json`.
5. Merge the new information while preserving manual content and metadata.
6. Write through `write --if-revision`.
7. Report changed memory paths in the final user-facing response.

A new kebab-case topical file is created only when no existing note is a clear
home. Files are split when they contain unrelated subjects, not at an arbitrary
line count.

## Automatic Invocation

Version one uses a single model-invoked `jot-memory` skill. Its description
instructs the agent to use it automatically before substantial research and to
capture durable knowledge once before completing the same work unit. The user
does not need to type a slash command.

Model invocation is intentionally best-effort. Version one does not install
global instructions or platform-specific hooks. Those mechanisms are justified
only if cross-platform scenario testing or real usage shows material missed
recall or capture events.

Once loaded for a work unit, the same skill owns both phases. Separate recall
and capture skills are not used because capture would otherwise require a
second automatic invocation without a new user message.

## Bootstrap

An absent memory directory means empty memory during recall; it is not an
error. Before the first capture, the agent creates the required global or
project directory with a narrowly scoped `mkdir -p` under the resolved vault.
All note-content operations still use JotMD.

The skill resolves the active vault through JotMD's effective configuration
rather than maintaining a second vault setting. If `jotmd` is missing from
`PATH`, the skill reports the missing prerequisite and does not fall back to
editing note contents directly.

## User Notes and Privacy

Automatic recall and capture set `--notes-dir` to one of these roots:

```text
<vault>/agent-memory/global
<vault>/agent-memory/projects/<project-id>
```

The current agent CLI rejects absolute note operands, backslashes, empty path
segments, and `.` or `..`, while the note store rejects symlink escapes. These
scoped roots prevent normal memory commands from reaching user notes.

The agent may search or read outside `agent-memory/` only after an explicit user
request to use the user's notes. It may modify a user note only when the user
also asked for that modification. Reading a user note does not authorize
copying its contents into agent memory; that requires an explicit request to
remember it or an independently established non-private fact.

This boundary assumes the platform sandbox and user permissions remain active.
If the user grants unrestricted host filesystem access, the design does not
claim to protect the vault from that agent.

## JotMD UI

The TUI hides `agent-memory/` by default while leaving it on disk and available
to the agent CLI.

- New configuration: `show_agent_memory = false` by default.
- New keybinding action: `view.agent_memory`.
- Default browsing key: `a`.
- The action is also present in help and the command palette.
- Toggling shows a short `agent memory visible` or `agent memory hidden`
  status message.

When hidden, the directory and all descendants are excluded from the tree,
path search, content search, and root directory preview. Search exclusion is
applied before scanning, ranking, and result limits so hidden memory cannot
crowd user-note results out of the result set. This is a TUI visibility filter;
it does not change the note store behavior used by the agent CLI. The watcher
may still observe changes, but rescans keep the filter applied.

If the current selection is inside `agent-memory/` when it becomes hidden, the
UI selects the nearest visible user entry when possible and clears any stale
preview. An otherwise empty vault shows the existing empty state.

The runtime visibility state is initialized from `show_agent_memory`. A config
reload changes it only when that setting's value changed; unrelated reloads do
not undo a session toggle.

UI hiding is a presentation feature, not a security boundary.

## Concurrency and Failures

All updates use optimistic revisions. On a revision conflict, the skill reads
the current note, merges again, and retries once. A second conflict skips the
capture and is reported without affecting the primary task result. Existing
notes are never overwritten unconditionally.

Other behavior:

- missing memory during recall is treated as no result;
- missing directories may be created only below `agent-memory/` during capture;
- a malformed or unsafe path aborts the memory operation;
- a search failure does not block the user's primary task but is surfaced when
  it can affect the result;
- a capture failure is reported in the final response;
- an unmanaged note is not modified automatically.

## Distribution

The repository contains one canonical `skills/jot-memory/SKILL.md` using the
portable Agent Skills subset. Thin host-specific packaging references that
same skill:

- Codex: `.codex-plugin/plugin.json` and publication through the Codex plugin
  directory;
- Claude Code: `.claude-plugin/plugin.json` and a Claude plugin marketplace;
- OpenCode: installation as a global skill or from its HTTP skill catalog,
  without a TypeScript plugin.

Installation is opt-in. Version one does not modify the user's global agent
instruction files. Documentation explains the JotMD prerequisite, installation
for each host, the reserved directory, and the permission implications.

The OpenCode plugin API is not needed for a Markdown-only skill. No new runtime
dependency is added to JotMD.

## Testing

### Go tests

- configuration default, load, dump, validation, and reload behavior for
  `show_agent_memory`;
- action registry, default `a` binding, custom binding, help, and command
  palette visibility;
- tree, directory preview, path search, and content search filtering;
- selection and preview behavior when hiding the selected memory entry;
- watcher rescans while memory remains hidden or visible;
- existing agent CLI and filesystem boundary tests remain passing.

### Skill scenarios

- recalls project and global notes once before a substantial investigation;
- skips recall and capture for a trivial task;
- does not repeat recall for follow-ups in the same work unit;
- captures a verified project pitfall after completion;
- routes a stable cross-project preference to global memory;
- searches before creating a new topic file;
- loads at most three relevant notes;
- does not access user notes without an explicit request;
- does not copy explicitly accessed private content into memory by default;
- respects `Managed by: user`;
- retries one revision conflict and reports a second conflict;
- produces the required visible metadata and reliability value.

### Packaging checks

- validate both plugin manifests;
- confirm the canonical skill is discoverable in Codex and Claude Code;
- confirm OpenCode discovers the same skill from its supported global source;
- smoke-test installation without a manual skill invocation.

## Acceptance Criteria

1. Installing the integration makes `jot-memory` available without requiring a
   manual command for normal use.
2. A substantial work unit can recall relevant global and project notes before
   research and capture durable knowledge once after completion.
3. Automatic operations cannot address a user note through their scoped
   `--notes-dir` roots.
4. Retrieved context is limited to relevant search results and at most three
   loaded notes.
5. Agent-created notes remain readable Markdown with visible provenance, dates,
   scope, and reliability.
6. The TUI hides `agent-memory/` by default and toggles it with `a` without
   affecting CLI access.
7. Concurrent manual edits are preserved or produce a reported conflict; they
   are never silently overwritten.
8. The feature adds no vector store, MCP server, platform hook, or new JotMD
   runtime dependency.

## Compatibility Risks

- `agent-memory/` becomes a reserved vault name; existing user content at that
  path must be moved or explicitly adopted before enabling the feature.
- Model-invoked skills can be missed occasionally; strict hooks remain a future
  option based on evidence.
- Lexical search can miss synonyms. The first upgrade, if needed, is improved
  tokenization and ranking within the existing CLI rather than a separate
  retrieval system.
- Repositories without a shared remote cannot reliably share memory across
  independent clones.
