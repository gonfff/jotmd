#!/bin/sh
set -eu

script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
helper="$script_dir/scope.sh"
tmp=$(mktemp -d)
tmp=$(CDPATH= cd "$tmp" && pwd -P)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	printf '%s\n' "scope test failed: $1" >&2
	exit 1
}

assert_rejected() {
	if "$helper" "$@" >"$tmp/stdout" 2>"$tmp/stderr"; then
		fail "accepted unsafe scope: $*"
	fi
	[ ! -s "$tmp/stdout" ] || fail "unsafe scope emitted a path: $*"
}

vault="$tmp/vault"
mkdir -p "$vault"
if "$helper" resolve "$vault" global >"$tmp/stdout" 2>"$tmp/stderr"; then
	fail 'resolve created or accepted an absent scope'
else
	status=$?
	[ "$status" -eq 2 ] || fail "absent resolve status = $status, want 2"
fi
[ ! -e "$vault/agent-memory" ] || fail 'resolve created memory directories'

want_global="$vault/agent-memory/global"
got=$("$helper" create "$vault" global) || fail 'create rejected safe global scope'
[ "$got" = "$want_global" ] || fail "global scope = '$got', want '$want_global'"
got=$("$helper" resolve "$vault" global) || fail 'resolve rejected safe global scope'
[ "$got" = "$want_global" ] || fail "resolved global scope = '$got', want '$want_global'"

want_project="$vault/agent-memory/projects/github.com/Owner/repository"
got=$("$helper" create "$vault" project github.com/Owner/repository) || fail 'create rejected safe project scope'
[ "$got" = "$want_project" ] || fail "project scope = '$got', want '$want_project'"

real_vault="$tmp/real-vault"
mkdir -p "$real_vault"
ln -s "$real_vault" "$tmp/vault-link"
got=$("$helper" create "$tmp/vault-link" global) || fail 'configured vault symlink was rejected'
[ "$got" = "$real_vault/agent-memory/global" ] || fail "symlinked vault scope = '$got'"

alias_vault="$tmp/alias-vault"
mkdir -p "$alias_vault/Agent-Memory"
assert_rejected resolve "$alias_vault" global

root_link_vault="$tmp/root-link-vault"
mkdir -p "$root_link_vault" "$tmp/outside-root/global"
ln -s "$tmp/outside-root" "$root_link_vault/agent-memory"
assert_rejected resolve "$root_link_vault" global

global_link_vault="$tmp/global-link-vault"
mkdir -p "$global_link_vault/agent-memory" "$tmp/outside-global"
ln -s "$tmp/outside-global" "$global_link_vault/agent-memory/global"
assert_rejected resolve "$global_link_vault" global

project_link_vault="$tmp/project-link-vault"
mkdir -p "$project_link_vault/agent-memory/projects" "$tmp/outside-project/Owner/repository"
ln -s "$tmp/outside-project" "$project_link_vault/agent-memory/projects/github.com"
assert_rejected resolve "$project_link_vault" project github.com/Owner/repository

final_link_vault="$tmp/final-link-vault"
mkdir -p "$final_link_vault/agent-memory/projects/github.com/Owner" "$tmp/outside-final"
ln -s "$tmp/outside-final" "$final_link_vault/agent-memory/projects/github.com/Owner/repository"
assert_rejected resolve "$final_link_vault" project github.com/Owner/repository

printf '%s\n' 'scope tests passed'
