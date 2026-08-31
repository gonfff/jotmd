# Agent Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship one portable `jot-memory` skill that recalls and captures durable Markdown memory through the existing JotMD CLI, while the JotMD TUI hides the reserved `agent-memory/` tree by default.

**Architecture:** Keep the filesystem and existing `jotmd search/get/write` commands as the source of truth. The skill scopes every automatic CLI operation to either the global or current-project memory directory. The TUI retains the full store snapshot for watching, derives a filtered snapshot for every visible tree/search/preview operation, and changes that filter through one runtime toggle.

**Tech Stack:** Go 1.26, Bubble Tea v2, TOML configuration, portable Agent Skills Markdown, POSIX `sh`, Git, Codex/Claude Code plugin manifests.

**Spec:** [`docs/superpowers/specs/2026-08-27-agent-memory-design.md`](../specs/2026-08-27-agent-memory-design.md)

## Global Constraints

- Keep one canonical skill at `skills/jot-memory/SKILL.md`; do not duplicate it for individual hosts.
- Add no Go or plugin runtime dependency, hook, global instruction file, MCP server, daemon, vector store, or new JotMD CLI command.
- Use JotMD for all note-content reads and writes. A small Git/POSIX helper is allowed only for deterministic project identity.
- Never automatically address notes outside `<vault>/agent-memory/global` or `<vault>/agent-memory/projects/<project-id>`.
- Preserve optimistic revisions, retry one conflict at most, and never overwrite an existing note without `--if-revision`.
- Keep automatic recall to at most three loaded notes per coherent work unit.
- Treat TUI hiding as presentation, not security. Keep the watcher on the complete snapshot.
- Do not touch the existing untracked `docs/images/jotmd-logo*.svg` files.
- Do not add an environment variable for `show_agent_memory`; the approved contract is TOML plus the runtime action.

---

### Task 1: Add the `show_agent_memory` configuration contract

**Files:**

- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/assets/config.toml`
- Test: `internal/config/config_test.go`
- Test: `internal/app/run_test.go`

- [ ] **Step 1: Write the failing configuration test**

Add `TestShowAgentMemoryDefaultsLoadsMergesAndDumps` to `internal/config/config_test.go`:

```go
func TestShowAgentMemoryDefaultsLoadsMergesAndDumps(t *testing.T) {
	if config.Defaults().ShowAgentMemory {
		t.Fatal("Defaults() ShowAgentMemory = true, want false")
	}
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "show_agent_memory = true\n")
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ShowAgentMemory {
		t.Fatal("Load() ShowAgentMemory = false, want true")
	}
	disabled := false
	got = config.Merge(got, config.Partial{ShowAgentMemory: &disabled})
	if got.ShowAgentMemory {
		t.Fatal("Merge() ShowAgentMemory = true, want false")
	}
	got.ShowAgentMemory = true
	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "show_agent_memory = true") {
		t.Fatalf("Dump() = %q, want show_agent_memory", output.String())
	}
}
```

Also add `"# show_agent_memory = false"` to the expected template lines in `TestRunInitConfigCreatesTemplatesBeforeStartingTUI`.

- [ ] **Step 2: Run the focused tests and confirm the contract is missing**

Run:

```sh
go test ./internal/config ./internal/app -run 'TestShowAgentMemory|TestRunInitConfigCreatesTemplatesBeforeStartingTUI' -count=1
```

Expected: compilation fails because `ShowAgentMemory` does not exist, or the template assertion fails.

- [ ] **Step 3: Implement the smallest configuration change**

Add the field beside `ShowHidden` in both public configuration values:

```go
type Config struct {
	// existing fields
	ShowHidden      bool
	ShowAgentMemory bool
	// existing fields
}

type Partial struct {
	// existing fields
	ShowHidden      *bool `toml:"show_hidden"`
	ShowAgentMemory *bool `toml:"show_agent_memory"`
	// existing fields
}
```

Set `ShowAgentMemory: false` in `Defaults`, merge a non-nil partial in `Merge`, and add the field to `dumpConfig` and `Dump`. No validation branch is needed for a boolean.

Document the setting after `show_hidden` in `internal/config/assets/config.toml`:

```toml
# Include agent-memory notes in the tree and search.
# show_agent_memory = false
```

Update existing full-value expectations such as `TestDefaults` only where needed; do not reformat unrelated literals.

- [ ] **Step 4: Run the package tests**

Run:

```sh
go test ./internal/config ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the configuration contract**

```sh
git add internal/config/config.go internal/config/validate.go internal/config/assets/config.toml internal/config/config_test.go internal/app/run_test.go
git commit -m "feat: configure agent memory visibility"
```

---

### Task 2: Search content inside an existing snapshot

**Files:**

- Modify: `internal/notes/search.go`
- Modify: `internal/app/cli.go`
- Test: `internal/notes/search_test.go`
- Test: `internal/app/cli_test.go` (existing tests only unless a regression appears)

- [ ] **Step 1: Write the failing snapshot-scoped search test**

Add `TestSearchContentSnapshotOnlyReadsProvidedEntries` to `internal/notes/search_test.go`. Use the existing `writeTree` fixture helper:

```go
func TestSearchContentSnapshotOnlyReadsProvidedEntries(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{
		{path: "agent-memory/global/topic.md", content: "needle", mode: 0o600},
		{path: "user.md", content: "needle", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	visible := Snapshot{Root: snapshot.Root}
	for _, entry := range snapshot.Entries {
		if entry.Path == "user.md" {
			visible.Entries = append(visible.Entries, entry)
		}
	}

	matches, stats, err := store.SearchContentSnapshot(context.Background(), visible, "needle", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := matchPaths(matches); fmt.Sprint(got) != "[user.md]" || stats.FilesScanned != 1 {
		t.Fatalf("SearchContentSnapshot() = %v, %#v, want user.md only", got, stats)
	}
}
```

The `limit=1` is intentional: this fails if exclusion happens after scanning or limiting.

- [ ] **Step 2: Run the focused test and confirm the method is absent**

```sh
go test ./internal/notes -run TestSearchContentSnapshotOnlyReadsProvidedEntries -count=1
```

Expected: compilation fails because `SearchContentSnapshot` does not exist.

- [ ] **Step 3: Extract the existing search loop without changing its behavior**

Keep the current early context/query/limit checks in `SearchContent`, scan once, then delegate:

```go
func (s *Store) SearchContent(ctx context.Context, query string, maxFileBytes int64, limit int) ([]Match, SearchStats, error) {
	limit = searchLimit(limit)
	if err := ctx.Err(); err != nil {
		return nil, SearchStats{}, err
	}
	if query == "" || limit == 0 {
		return nil, SearchStats{}, nil
	}
	snapshot, err := s.Scan(ctx)
	if err != nil {
		return nil, SearchStats{}, err
	}
	return s.SearchContentSnapshot(ctx, snapshot, query, maxFileBytes, limit)
}

func (s *Store) SearchContentSnapshot(ctx context.Context, snapshot Snapshot, query string, maxFileBytes int64, limit int) ([]Match, SearchStats, error) {
	// The current normalized-query, entry loop, size cap, cancellation,
	// snippet, and truncation logic moves here unchanged.
}
```

Change `runSearch` in `internal/app/cli.go` to reuse the snapshot it already scanned:

```go
contentMatches, stats, err := store.SearchContentSnapshot(ctx, snapshot, query, 8<<20, invocation.limit-len(matches))
```

This removes a redundant scan while keeping the CLI result contract unchanged.

- [ ] **Step 4: Run notes and agent CLI tests**

```sh
go test ./internal/notes ./internal/app -count=1
```

Expected: PASS, including existing Unicode, cancellation, truncation, and agent JSON tests.

- [ ] **Step 5: Commit the search primitive**

```sh
git add internal/notes/search.go internal/notes/search_test.go internal/app/cli.go
git commit -m "refactor: search note content in a snapshot"
```

---

### Task 3: Hide agent memory from all TUI read surfaces

**Files:**

- Modify: `internal/ui/model.go`
- Modify: `internal/ui/update.go`
- Modify: `internal/ui/search.go`
- Create: `internal/ui/agent_memory_test.go`

- [ ] **Step 1: Add focused failing tests for the filter and default behavior**

Create `internal/ui/agent_memory_test.go` using the existing `writeNote`, `newModel`, `updateModel`, `updateModelCommand`, `run`, and `key` helpers.

Add these tests:

```go
func TestAgentMemorySnapshotFilterMatchesReservedRootOnly(t *testing.T)
func TestModelHidesAgentMemoryFromTreeAndSearchByDefault(t *testing.T)
func TestModelRescanKeepsAgentMemoryVisibility(t *testing.T)
```

Use a vault containing:

```text
agent-memory/global/topic.md     content: memory-only needle
agent-memory-old.md              content: ordinary user note
user.md                          content: user needle
```

Assertions must cover all of these:

- the model's full snapshot contains `agent-memory/global/topic.md`;
- the tree snapshot omits `agent-memory` and its descendants by default;
- `agent-memory-old.md` remains visible;
- `notes.RankPaths(model.tree.snapshot, ...)` cannot return hidden memory;
- `run(model.searchContentCommand(...))` cannot return hidden memory;
- adding another memory note and applying a later `scanResult` still leaves it hidden;
- `Snapshot.Root` is preserved by the filter;
- a model initialized with `cfg.ShowAgentMemory = true` includes the memory tree.

- [ ] **Step 2: Run the tests and confirm hidden memory leaks through**

```sh
go test ./internal/ui -run 'TestAgentMemorySnapshotFilter|TestModelHidesAgentMemory|TestModelRescanKeepsAgentMemory' -count=1
```

Expected: compilation fails because the filter/full snapshot state is absent, or the visibility assertions fail.

- [ ] **Step 3: Add one exact reserved-root filter to the model**

Add `strings` to `internal/ui/model.go`, retain the complete snapshot, and store session visibility separately from the last configured value:

```go
type Model struct {
	// existing fields
	snapshot        notes.Snapshot
	tree            Tree
	showAgentMemory bool
	// existing fields
}

const agentMemoryRoot notes.RelPath = "agent-memory"

func isAgentMemoryPath(path notes.RelPath) bool {
	return path == agentMemoryRoot || strings.HasPrefix(string(path), string(agentMemoryRoot)+"/")
}

func visibleSnapshot(snapshot notes.Snapshot, showAgentMemory bool) notes.Snapshot {
	if showAgentMemory {
		return snapshot
	}
	visible := notes.Snapshot{Root: snapshot.Root, Entries: make([]notes.Entry, 0, len(snapshot.Entries))}
	for _, entry := range snapshot.Entries {
		if !isAgentMemoryPath(entry.Path) {
			visible.Entries = append(visible.Entries, entry)
		}
	}
	return visible
}
```

Initialize `showAgentMemory: cfg.ShowAgentMemory` in `NewModelWithOptions`.

- [ ] **Step 4: Apply the filtered snapshot before every visible operation**

In the successful `scanResult` branch:

```go
m.snapshot = message.snapshot
snapshot := visibleSnapshot(message.snapshot, m.showAgentMemory)
```

Use `snapshot` for `NewTree` and `Tree.Reconcile`, but keep the watcher on the full value:

```go
return m, tea.Batch(read, m.startWatch(message.snapshot))
```

Do not change `internal/notes/watch.go`, `internal/ui/layout.go`, `internal/ui/help.go`, or `internal/ui/palette.go`. They already consume the tree snapshot or action registry.

Change content search to use the filtered tree snapshot before scanning and limiting:

```go
matches, _, err := m.store.SearchContentSnapshot(ctx, m.tree.snapshot, query, searchMaxFileBytes, limit)
```

Path search and directory previews already use `m.tree.snapshot`, so this one filter covers them.

- [ ] **Step 5: Run UI and notes tests**

```sh
go test ./internal/ui ./internal/notes -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the default visibility filter**

```sh
git add internal/ui/model.go internal/ui/update.go internal/ui/search.go internal/ui/agent_memory_test.go
git commit -m "feat: hide agent memory from the TUI"
```

---

### Task 4: Add the runtime toggle, key binding, and reload semantics

**Files:**

- Modify: `internal/ui/actions.go`
- Modify: `internal/ui/keymap.go`
- Modify: `internal/ui/update.go`
- Modify: `internal/config/assets/keybindings.toml`
- Modify: `internal/ui/agent_memory_test.go`
- Modify: `internal/config/keymap_test.go`
- Modify: `internal/app/run_test.go`
- Test: `internal/ui/reload_test.go` (add a focused test here or keep reload cases in `agent_memory_test.go`, not both)

- [ ] **Step 1: Add failing behavior and discoverability tests**

Extend `internal/ui/agent_memory_test.go` with:

```go
func TestModelTogglesAgentMemoryAndClearsHiddenSelection(t *testing.T)
func TestModelConfigReloadOnlyOverridesChangedAgentMemorySetting(t *testing.T)
func TestAgentMemoryActionIsDiscoverableAndCustomizable(t *testing.T)
```

The toggle test must start with `ShowAgentMemory = true`, load `agent-memory/global/topic.md`, press `a`, and assert immediately that:

- the memory tree disappeared;
- a visible user entry became selected when available;
- the old memory document/preview was cleared before the replacement async read completes;
- the status is `agent memory hidden`;
- pressing `a` again exposes the memory tree and sets `agent memory visible`;
- a vault containing only memory enters the existing empty state after hiding.

The reload test must exercise this sequence:

1. configured `false`, session toggled to `true`;
2. reload with configured `false` again: session stays `true`;
3. reload with configured `true`: runtime follows the changed setting;
4. session toggled to `false`, reload with configured `true` again: session stays `false`.

The discoverability test must assert:

- default `a` maps to the new action in tree, TOC, and preview contexts;
- `BindingsForKeymap` can replace `a` with another key;
- `ActionRegistry`, help, and command palette include the `view.agent_memory` binding and `agent memory` label.

Also add `"view.agent_memory": "a"` to `TestInitKeymapTemplateLoadsEveryWorkflowBinding`, and expect `"view.agent_memory" = ["a"]` in the initialized keymap template test in `internal/app/run_test.go` if that test checks key contents.

- [ ] **Step 2: Run the focused tests and confirm the action is missing**

```sh
go test ./internal/ui ./internal/config ./internal/app -run 'AgentMemory|InitKeymapTemplate|InitConfigCreatesTemplates' -count=1
```

Expected: compilation or assertions fail because the action, binding, and reload behavior do not exist.

- [ ] **Step 3: Register the action and default binding**

Add the action:

```go
ActionAgentMemory Action = "agent-memory"
```

Add this `DefaultBindings` entry:

```go
{
	Name:     "view.agent_memory",
	Action:   ActionAgentMemory,
	Keys:     []string{"a"},
	Label:    "agent memory",
	Contexts: []Context{ContextTree, ContextTOC, ContextPreview},
},
```

Mirror it in `internal/config/assets/keybindings.toml`:

```toml
"view.agent_memory" = ["a"]
```

No dedicated help or palette code is needed; both derive their items from effective bindings.

- [ ] **Step 4: Implement one visibility transition method**

Add a method near other model state transitions:

```go
func (m *Model) setAgentMemoryVisible(visible bool) tea.Cmd {
	selected, hadSelection := m.tree.Selected()
	m.showAgentMemory = visible
	m.tree = m.tree.Reconcile(visibleSnapshot(m.snapshot, visible))
	if hadSelection && isAgentMemoryPath(selected.Path) && !visible {
		m.clearDocument()
	}
	if visible {
		m.status = "agent memory visible"
	} else {
		m.status = "agent memory hidden"
	}
	return m.readSelected()
}
```

Dispatch it from the central action switch:

```go
case ActionAgentMemory:
	return m, m.setAgentMemoryVisible(!m.showAgentMemory)
```

Keeping this in the central dispatcher makes the key, help, and palette paths identical.

- [ ] **Step 5: Apply a reloaded value only when the configured value changed**

At the start of the successful `configReloadResult` application, compare against the old `m.cfg` value before assigning it:

```go
agentMemoryChanged := m.cfg.ShowAgentMemory != message.resolved.ShowAgentMemory
m.cfg.ShowAgentMemory = message.resolved.ShowAgentMemory
```

After the other supported UI fields are applied:

```go
var visibility tea.Cmd
if agentMemoryChanged {
	visibility = m.setAgentMemoryVisible(m.cfg.ShowAgentMemory)
}
return m, tea.Batch(scan, visibility, m.renderDocument())
```

Do not set `m.showAgentMemory` on unrelated reloads. `m.cfg.ShowAgentMemory` is the last configured value; `m.showAgentMemory` is the session choice.

- [ ] **Step 6: Run the UI/config/app tests**

```sh
go test ./internal/ui ./internal/config ./internal/app -count=1
```

Expected: PASS. If a view golden changes only because the additional help row changes layout, inspect it and update only the affected golden with the existing focused golden-update command; do not regenerate unrelated goldens.

- [ ] **Step 7: Commit the runtime controls**

```sh
git add internal/ui/actions.go internal/ui/keymap.go internal/ui/update.go internal/config/assets/keybindings.toml internal/ui/agent_memory_test.go internal/ui/reload_test.go internal/config/keymap_test.go internal/app/run_test.go
git commit -m "feat: toggle agent memory visibility"
```

Before staging, omit `internal/ui/reload_test.go` if the reload test was placed only in `agent_memory_test.go` and the file did not change.

---

### Task 5: Build deterministic project identity and the canonical memory skill

**Files:**

- Create: `skills/jot-memory/SKILL.md`
- Create: `skills/jot-memory/scripts/project-id.sh`
- Create: `skills/jot-memory/scripts/project-id_test.sh`

- [ ] **Step 1: Write the failing project-identity self-test**

Create executable `skills/jot-memory/scripts/project-id_test.sh` with `set -eu`, `mktemp -d`, and a cleanup trap. It must create temporary Git repositories and assert:

```text
git@github.com:Owner/repository.git       -> github.com/Owner/repository
https://github.com/Owner/repository.git  -> github.com/Owner/repository
unsafe path segments                     -> safe segment plus stable 8-char hash
no origin, main checkout                 -> local/<name>-<8-char-hash>
no origin, linked worktree               -> same local ID as the main checkout
```

Use local Git configuration on the test commit so the script does not depend on the user's Git identity:

```sh
git -C "$repo" -c user.name=Test -c user.email=test@example.invalid commit --allow-empty -m init >/dev/null
```

- [ ] **Step 2: Run the self-test and confirm the helper is absent**

```sh
sh skills/jot-memory/scripts/project-id_test.sh
```

Expected: FAIL because `project-id.sh` does not exist.

- [ ] **Step 3: Implement the smallest portable identity helper**

Create executable `skills/jot-memory/scripts/project-id.sh` using only POSIX `sh`, Git, `sed`, `tr`, and `cut`.

The helper must:

1. run from the current repository without changing the caller's working directory;
2. obtain `origin` with `git remote get-url origin`;
3. normalize HTTP(S), `ssh://`, and SCP-style Git remotes to `<lowercase-host>/<path-without-.git>`;
4. reject empty, `.` and `..` segments;
5. replace characters outside `[A-Za-z0-9._-]` with `-`;
6. append `-$(printf %s "$canonical" | git hash-object --stdin | cut -c1-8)` to every changed segment;
7. when `origin` is absent, canonicalize `git rev-parse --git-common-dir`, derive the repository name from that common directory, and print `local/<safe-name>-<8-char-hash>`;
8. write only the final project ID to stdout and diagnostics to stderr.

Keep hash generation in one shell function:

```sh
hash8() {
	printf '%s' "$1" | git hash-object --stdin | cut -c1-8
}
```

Do not add a Go command for this helper: installed JotMD users may not have a Go toolchain, while every project identity operation already requires Git.

- [ ] **Step 4: Run the identity self-test**

```sh
sh skills/jot-memory/scripts/project-id_test.sh
```

Expected: PASS with no output beyond an optional final `project-id tests passed` line.

- [ ] **Step 5: Write the canonical portable skill**

Use only shared Agent Skills frontmatter:

```yaml
---
name: jot-memory
description: Recall relevant JotMD memory before substantial research, planning, or implementation, and capture durable knowledge after completing a coherent work unit. Use automatically when either condition applies.
---
```

The body must give the agent one work-unit checklist, not two independently invoked skills:

```text
1. Decide once whether this work unit is substantial enough for recall.
2. Recall at most once before substantial research/planning/implementation.
3. Complete the user's work normally.
4. Decide once whether durable knowledge was produced.
5. Capture at most once before the final response and report changed paths.
```

Include these exact operational rules:

- prerequisite: `command -v jotmd` must succeed; otherwise report the missing prerequisite and do not edit memory directly;
- resolve the absolute vault from `jotmd --dump-config` and its `notes_dir` value;
- resolve the project with `scripts/project-id.sh`; abort project memory on an unsafe/failed ID;
- recall searches project first, global second, using several short terms and `--limit 10`;
- inspect result paths/snippets, then load no more than three notes total;
- absent directories or no matches mean empty memory, not failure;
- create only the selected memory scope immediately before first capture with narrowly quoted `mkdir -p`;
- search the chosen scope before creating a topic;
- update only notes whose visible metadata still says `Managed by: jot-memory`;
- preserve manual content and use `get --json` revision plus `write --if-revision`;
- on `revision_conflict`, re-read, merge, retry once, then skip and report;
- never automatically read or write user notes outside `agent-memory/`;
- never copy explicitly read private user content into memory unless the user explicitly asks to remember it;
- never capture secrets, transcripts, routine summaries, intermediate hypotheses, or cheaply re-derived facts;
- project knowledge is the default; only cross-project preferences/workflows go global;
- new topic files are kebab-case and split by subject, not arbitrary size;
- time-sensitive memory is a lead that must be reverified.

Show the scoped CLI forms exactly as supported today:

```sh
jotmd --notes-dir "$project_scope" search --limit 10 "$term" --json
jotmd --notes-dir "$global_scope" search --limit 10 "$term" --json
jotmd --notes-dir "$scope" get "$path" --json
jotmd --notes-dir "$scope" write "$path" --json < "$new_note"
jotmd --notes-dir "$scope" write "$path" --if-revision "$revision" --json < "$merged_note"
```

State explicitly that note operands are relative to the scoped `--notes-dir` and that the workflow never uses `cd ..` to escape a scope.

Include the approved visible Markdown metadata template and only these reliability values: `verified`, `user-stated`, `inferred`, `mixed`. Project notes include `Project`; global notes omit it. `Last agent update` changes only with substantive content.

- [ ] **Step 6: Review the skill against the approved scenarios**

Read `skills/jot-memory/SKILL.md` once as a fresh agent would and check each scenario from the spec's “Skill scenarios” section. In particular, confirm the text makes these boundaries unambiguous:

- one recall and one capture per work unit, not per request/message;
- trivial work skips both;
- three loaded notes maximum across both scopes;
- search-before-create and project-first routing;
- unmanaged notes are read-only;
- private user content is not promoted automatically;
- exactly one conflict retry.

If any scenario needs an inferred rule, tighten the existing section rather than adding a second workflow or host-specific copy.

- [ ] **Step 7: Run the helper check and inspect the final skill size**

```sh
sh skills/jot-memory/scripts/project-id_test.sh
wc -l skills/jot-memory/SKILL.md
```

Expected: helper PASS; the skill is concise enough to load on demand and contains no duplicated platform sections.

- [ ] **Step 8: Commit the canonical skill**

```sh
git add skills/jot-memory/SKILL.md skills/jot-memory/scripts/project-id.sh skills/jot-memory/scripts/project-id_test.sh
git commit -m "feat: add portable JotMD memory skill"
```

---

### Task 6: Package the skill and document opt-in installation

**Files:**

- Create: `.codex-plugin/plugin.json`
- Create: `.claude-plugin/plugin.json`
- Create: `.claude-plugin/marketplace.json`
- Modify: `README.md`

- [ ] **Step 1: Add minimal failing packaging checks**

Before creating manifests, run:

```sh
test -f .codex-plugin/plugin.json
test -f .claude-plugin/plugin.json
test -f .claude-plugin/marketplace.json
```

Expected: FAIL.

- [ ] **Step 2: Add the thin Codex and Claude Code manifests**

Create `.codex-plugin/plugin.json`:

```json
{
  "name": "jot-memory",
  "version": "0.1.0",
  "description": "Recall and maintain durable project and global knowledge in JotMD.",
  "skills": "./skills/"
}
```

Create `.claude-plugin/plugin.json` without a pinned version so Git commits provide marketplace versions during early development:

```json
{
  "name": "jot-memory",
  "description": "Recall and maintain durable project and global knowledge in JotMD."
}
```

Create `.claude-plugin/marketplace.json` with the repository root as the plugin source, preserving the one canonical skill:

```json
{
  "name": "jotmd",
  "owner": {
    "name": "gonfff"
  },
  "plugins": [
    {
      "name": "jot-memory",
      "source": "./",
      "description": "Recall and maintain durable project and global knowledge in JotMD."
    }
  ]
}
```

Do not add `.agents/plugins/marketplace.json` in v1. Codex recognizes the Claude marketplace location as legacy-compatible; add a separate catalog only if the real Codex smoke test proves it necessary.

- [ ] **Step 3: Validate the static packaging**

Run:

```sh
test -f skills/jot-memory/SKILL.md
git diff --check
```

If Claude Code is installed, also run the official validator:

```sh
claude plugin validate .
```

Expected: files exist, whitespace check passes, and the optional validator reports `Validation passed`. If it rejects `"source": "./"`, use the validator's accepted root-relative spelling without moving or copying the canonical skill.

- [ ] **Step 4: Document installation and the user-visible behavior**

Add an `Agent memory` section to `README.md` after `CLI`. It must state:

- prerequisite: install `jotmd` and keep it on `PATH`;
- opt-in and best-effort model invocation;
- reserved vault directory and global/project layout;
- automatic access is scoped to `agent-memory/`; unrestricted host access remains unrestricted;
- memory is ordinary editable Markdown;
- `a` toggles memory in the TUI and `show_agent_memory = true` changes startup visibility;
- agent-created metadata/reliability is visible;
- capture reports changed note paths.

Document host installation separately:

```sh
# Claude Code
claude plugin marketplace add gonfff/jot
claude plugin install jot-memory@jotmd

# OpenCode global skill, after cloning/downloading this repository
mkdir -p ~/.config/opencode/skills
cp -R skills/jot-memory ~/.config/opencode/skills/
```

For Codex, document the Plugins Directory installation after publication and this marketplace registration for local/repository testing:

```sh
codex plugin marketplace add gonfff/jot
```

Do not invent a `codex plugin install` command. Note that OpenCode needs no TypeScript plugin or manifest because it discovers the canonical skill at `~/.config/opencode/skills/jot-memory/SKILL.md`.

Add `a | Show/hide agent memory` to the existing essential keys table and mention the TOML setting in Configuration.

- [ ] **Step 5: Perform opt-in host smoke tests without changing user globals silently**

Only when the user explicitly authorizes installation into their host configuration:

1. Claude Code: add the local repository marketplace, install `jot-memory@jotmd`, start a fresh session, and verify the skill is discoverable.
2. Codex: add the local repository marketplace, restart the supported plugin surface, and verify `jot-memory` appears.
3. OpenCode: copy the skill to a temporary `XDG_CONFIG_HOME`, start OpenCode there, and verify `jot-memory` is listed.
4. In one host, point JotMD at a temporary vault, seed one project and one global memory note, request a substantial investigation without naming the skill, and verify project-first recall plus at most three `get` operations.
5. Follow up on the same goal and verify recall is not repeated; finish with durable knowledge and verify one revision-checked capture.

Do not block the code handoff when a host binary is unavailable. Record which host smoke tests were run and leave unavailable public-directory submission as release work, not implementation code.

- [ ] **Step 6: Commit packaging and documentation**

```sh
git add .codex-plugin/plugin.json .claude-plugin/plugin.json .claude-plugin/marketplace.json README.md
git commit -m "docs: package and install JotMD agent memory"
```

---

### Task 7: Run integrated verification and review compatibility

**Files:**

- Verify only; modify only files that fail a relevant check.

- [ ] **Step 1: Run focused checks from the feature boundary inward**

```sh
sh skills/jot-memory/scripts/project-id_test.sh
go test ./internal/config ./internal/notes ./internal/ui ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the repository check**

```sh
just check
```

Expected: formatting, `go vet`, and all tests pass.

- [ ] **Step 3: Inspect the final diff and scope**

```sh
git status --short
git diff --check
git diff --stat HEAD~6..HEAD
```

Confirm:

- no dependency files changed;
- no user logo SVG was staged;
- no JotMD public CLI syntax changed;
- no automatic operation can construct a note operand outside a scoped memory root;
- the watcher always receives the full snapshot;
- hidden memory is excluded before content scanning and result limits;
- only the intended manifest, skill, config, UI, search, tests, and README files changed.

- [ ] **Step 4: Report explicit compatibility risks in the handoff**

Report these known limits without adding code for them:

- `agent-memory/` is now reserved; existing user content there must be moved or explicitly adopted.
- Model-invoked recall/capture is best-effort; hooks are deferred until missed invocations are observed.
- Lexical search may miss synonyms; improve existing ranking before considering embeddings.
- Repositories without a shared remote cannot share fallback memory across independent clones.
- Claude's root plugin source copies the repository into its cache; split packaging into a dedicated repository/subdirectory only if repository size becomes material.
- Codex public-directory submission requires separate publisher/review work and is not completed merely by adding manifests.

- [ ] **Step 5: Commit only if verification required a correction**

If checks required a scoped correction:

```sh
git add <only-the-corrected-files>
git commit -m "fix: complete agent memory verification"
```

Otherwise, leave the six task commits unchanged.
