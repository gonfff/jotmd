# JotMD agent memory

The `jot-memory` skill lets Codex, Claude Code, and OpenCode recall and maintain
durable Markdown notes through the `jotmd` CLI. The agent decides when a
substantial work unit needs recall and whether its result is worth preserving.

## Prerequisite

Install `jotmd` and make sure it is on `PATH`:

```sh
brew install gonfff/tap/jotmd
command -v jotmd
```

Run `jotmd` once and select the vault when prompted. This persists the vault path
for both the TUI and agents:

```sh
jotmd
```

The skill stores agent-managed notes under `agent-memory/` in that vault. JotMD
hides this directory by default; press `a` in the TUI to show it.

## Codex

Install the marketplace and plugin:

```sh
codex plugin marketplace add gonfff/jotmd
codex plugin add jot-memory@jotmd
```

Update the marketplace snapshot and reinstall the latest plugin version:

```sh
brew upgrade jotmd
codex plugin marketplace upgrade jotmd
codex plugin add jot-memory@jotmd
```

Start a new Codex thread after installation or update so the new skill is
loaded. Use `codex plugin list` to inspect the configured marketplace and
plugin.

To remove the plugin:

```sh
codex plugin remove jot-memory@jotmd
codex plugin marketplace remove jotmd
```

## Claude Code

Install the marketplace and plugin:

```sh
claude plugin marketplace add gonfff/jotmd
claude plugin install jot-memory@jotmd
```

Update both:

```sh
brew upgrade jotmd
claude plugin marketplace update jotmd
claude plugin update jot-memory@jotmd
```

Restart Claude Code after an update. Use `claude plugin list` to verify the
installation.

To remove the plugin:

```sh
claude plugin uninstall jot-memory@jotmd
claude plugin marketplace remove jotmd
```

## OpenCode

OpenCode discovers global skills under
`~/.config/opencode/skills/<name>/SKILL.md`. Keep a shallow checkout and link
the skill into that directory so updates remain one command:

```sh
git clone --depth 1 https://github.com/gonfff/jotmd.git ~/.local/share/jotmd
mkdir -p ~/.config/opencode/skills
ln -s ~/.local/share/jotmd/skills/jot-memory ~/.config/opencode/skills/jot-memory
```

Update JotMD and the skill:

```sh
brew upgrade jotmd
git -C ~/.local/share/jotmd pull --ff-only
```

Restart OpenCode after installation or update. To remove the skill while keeping
the checkout and vault:

```sh
unlink ~/.config/opencode/skills/jot-memory
```

## Verify the integration

Start a new agent session in a Git repository and ask it to perform substantial
research or implementation. The agent should consider memory before the work
and report any note it changes afterward. Trivial requests intentionally skip
memory.

Agent-created notes are ordinary Markdown under:

```text
agent-memory/
├── global/
└── projects/<project-id>/
```

Do not create an unrelated user directory named `agent-memory/`: that top-level
name is reserved for the integration.
