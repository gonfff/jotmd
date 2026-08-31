# jotmd

Keyboard-first Markdown notes for the terminal. Browse plain files, search your
vault, and preview Markdown without leaving the command line.

![jotmd browsing a Markdown vault](docs/images/jotmd.png)

## Install

On macOS with [Homebrew](https://brew.sh/):

```sh
brew install gonfff/tap/jotmd
```

Or build from source with Go 1.26+ and [just](https://just.systems/):

```sh
git clone https://github.com/gonfff/jotmd.git
cd jotmd
just install # installs to ~/.local/bin/jotmd
```

## Quick start

Run `jotmd`. On first launch it asks where your notes live, creates the
directory if needed, and writes configuration under `~/.config/jotmd/` (or
`$XDG_CONFIG_HOME/jotmd/`). You can also skip the prompt:

```sh
mkdir -p ~/vault
jotmd --notes-dir ~/vault
```

JotMD uses `--editor` or the configured editor, then `$EDITOR`, then `vi`.

## CLI

Four non-interactive commands cover scripted and interactive workflows while
Markdown files remain the source of truth:

```text
jotmd [--notes-dir PATH] search [--limit N] QUERY [--json]
jotmd [--notes-dir PATH] get PATH [--json]
jotmd [--notes-dir PATH] write [--if-revision REVISION] PATH [--json]
jotmd [--notes-dir PATH] delete --if-revision REVISION PATH [--json]
```

`write` without `--if-revision` creates a missing note and refuses to overwrite
an existing one. To update safely, read the current revision and send it back:

```sh
jotmd --notes-dir ./notes search "refresh token redis" --json
jotmd --notes-dir ./notes get projects/foo/pitfalls.md --json > /tmp/note.json
jq -rj '.content' /tmp/note.json > /tmp/note.md
revision=$(jq -r '.revision' /tmp/note.json)

# Edit /tmp/note.md, then update only if nobody changed the note meanwhile.
jotmd --notes-dir ./notes write projects/foo/pitfalls.md \
  --if-revision "$revision" < /tmp/note.md
```

`delete` applies the same revision check and permanently removes one note. It
cannot be undone. With `--json`, successes use stdout and errors use stderr.

## Agent memory

> [!WARNING]
> `agent-memory/` is a reserved top-level vault directory. Before enabling this
> integration, move any existing user content out of that directory or explicitly
> adopt it as agent-managed memory; JotMD hides it by default and agents may recall
> it automatically. Case variants such as `Agent-Memory` are rejected.

The optional `jot-memory` skill lets supported coding agents recall useful
knowledge before substantial work and capture durable findings afterward. Install
`jotmd` first and keep it on `PATH`. Invocation is automatic but best-effort: the
model decides whether the current work needs recall or produced knowledge worth
saving.

Memory remains ordinary, editable Markdown in the configured vault:

```text
agent-memory/
├── global/
└── projects/<project-id>/
```

Automatic note access is limited to `agent-memory/`; notes elsewhere remain user
notes unless the user explicitly requests access. This boundary does not sandbox
an agent that already has unrestricted host filesystem access. Agent-created
notes show their manager, scope, dates, project when applicable, and reliability.
After capture, the agent reports every changed note path.

The TUI hides `agent-memory/` by default. Press `a` to show or hide it, or set
`show_agent_memory = true` in `config.toml` to show it at startup.

Installation is opt-in for each host:

```sh
# Claude Code
claude plugin marketplace add gonfff/jot
claude plugin install jot-memory@jotmd

# Codex local/repository testing
codex plugin marketplace add gonfff/jot
codex plugin add jot-memory@jotmd

# OpenCode global skill, after cloning/downloading this repository
mkdir -p ~/.config/opencode/skills
cp -R skills/jot-memory ~/.config/opencode/skills/
```

After publication, Codex users can install `jot-memory` from the Plugins
Directory. OpenCode needs no TypeScript plugin or manifest; it discovers the
canonical skill at `~/.config/opencode/skills/jot-memory/SKILL.md`.

## What it does

- Browses directories and `.md` files with a live, wrapping Markdown preview.
- Searches note names and contents; includes a table of contents and raw view.
- Creates, renames, copies, moves, trashes, and permanently deletes notes.
- Reloads notes, configuration, and key bindings while running.
- Ships with 11 themes and supports custom TOML themes and color overrides.

## Essential keys

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Navigate |
| `Enter` / `Space` | Expand or open |
| `Tab` | Switch tree and preview |
| `e` | Edit the selected note |
| `n` / `N` | Create a note / directory |
| `/` | Search |
| `t` | Select a theme |
| `a` | Show/hide agent memory |
| `Shift+P` | Open the command palette |
| `?` | Show all key bindings |
| `q` | Quit |

Key bindings are configurable in `~/.config/jotmd/keybindings.toml`. Run
`jotmd --dump-keys` to print the effective bindings.

## Configuration

CLI flags override environment variables, which override `config.toml` and
defaults. Configuration files are created automatically on first launch and
document every setting.

Optional configuration commands:

- `jotmd --init-themes` exports bundled themes as editable TOML files without
  overwriting existing customizations.
- `jotmd --list-themes` lists names accepted by `--theme` and `theme` in
  `config.toml`.
- `jotmd --dump-config` prints the effective configuration after defaults,
  files, environment variables, and CLI flags are merged.
- `jotmd config check [PATH]` validates the configuration, key bindings,
  editor, and theme without starting the TUI.

Useful overrides include `--notes-dir`, `--theme`, and `--editor`; run
`jotmd --help` for the complete CLI.

Set `show_agent_memory = true` in `config.toml` to include agent memory in the
tree and searches when JotMD starts. The default is `false`.

## Development

```sh
just run       # run from the checkout
just build     # build bin/jotmd
just test      # run tests
just check     # format check, vet, and tests
```

## License

[MIT](LICENSE)
