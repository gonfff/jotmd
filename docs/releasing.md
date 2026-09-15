# Releasing JotMD and `jot-memory`

JotMD and the `jot-memory` plugin ship from the same repository. Publish skill
changes through the normal JotMD release instead of maintaining a second
release process.

## Update the skill

1. Edit `skills/jot-memory/SKILL.md` and its scripts.
2. Add or update the matching shell tests in `skills/jot-memory/scripts/`.
3. Bump `.codex-plugin/plugin.json` `version` using semantic versioning:
   patch for wording or fixes, minor for compatible behavior, major for
   incompatible behavior.
4. If plugin metadata changes, keep `.codex-plugin/plugin.json`,
   `.claude-plugin/plugin.json`, and `.claude-plugin/marketplace.json`
   consistent.
5. Update `docs/jot-memory.md` when installation, prerequisites, or behavior
   changes.

Run the skill-specific checks from the repository root:

```sh
sh skills/jot-memory/scripts/project-id_test.sh
sh skills/jot-memory/scripts/scope_test.sh
claude plugin validate .
```

Then run the project checks:

```sh
just check
```

Install the checkout through an isolated local marketplace when testing Codex
discovery:

```sh
test_codex_home=$(mktemp -d)
CODEX_HOME="$test_codex_home" codex plugin marketplace add .
CODEX_HOME="$test_codex_home" codex plugin add jot-memory@jotmd
CODEX_HOME="$test_codex_home" codex
```

Use a disposable vault in that new session and exercise both recall and capture.
Do not validate only by reading `SKILL.md`.

## Prepare a release

Choose the next application version, for example `0.0.6`, and create non-empty
release notes at:

```text
.github/release-notes/v0.0.6.md
```

Run the release snapshot before tagging. It requires GoReleaser:

```sh
just release-check
```

Review the diff, squash the feature work as intended, and merge it into
`master`. The release tag must point at the merged `master` commit.

## Publish

Replace `0.0.6` below with the chosen version:

```sh
git switch master
git pull --ff-only
git push origin master
git tag -a v0.0.6 -m "jotmd v0.0.6"
git push origin v0.0.6
```

Pushing the tag starts `.github/workflows/release.yml`. The workflow validates
the tag and release notes, runs `just check` and `just release-check`, then
publishes the GitHub release, macOS archives, checksums, and Homebrew formula.

Watch it to completion:

```sh
release_run_id=$(gh run list --workflow release.yml --limit 1 \
  --json databaseId --jq '.[0].databaseId')
gh run watch "$release_run_id" --exit-status
gh release view v0.0.6
```

If CI fails, inspect the failed log and rerun only after identifying whether the
failure is reproducible. Do not move or recreate a public tag to hide a failed
run.

## Post-release check

Verify both distribution paths:

```sh
brew update
brew upgrade jotmd
jotmd --version
codex plugin marketplace upgrade jotmd
codex plugin add jot-memory@jotmd
```

Start a new Codex thread and exercise recall and capture with a disposable vault.
The user-facing update commands for all supported hosts are documented in
[`docs/jot-memory.md`](jot-memory.md).
