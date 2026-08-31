#!/bin/sh
set -eu

script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
helper="$script_dir/project-id.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	printf '%s\n' "project-id test failed: $1" >&2
	exit 1
}

assert_id() {
	repo=$1
	want=$2
	got=$(cd "$repo" && "$helper") || fail "helper rejected $repo"
	[ "$got" = "$want" ] || fail "got '$got', want '$want'"
}

assert_id_from() {
	repo=$1
	directory=$2
	want=$3
	got=$(cd "$repo/$directory" && "$helper") || fail "helper rejected $repo/$directory"
	[ "$got" = "$want" ] || fail "got '$got' from $directory, want '$want'"
}

assert_id_with_locale() {
	repo=$1
	test_locale=$2
	want=$3
	got=$(cd "$repo" && LC_ALL="$test_locale" "$helper") || fail "helper rejected locale $test_locale"
	[ "$got" = "$want" ] || fail "got '$got' under $test_locale, want '$want'"
}

assert_id_with_home() {
	repo=$1
	want=$2
	test_home=$3
	got=$(cd "$repo" && HOME="$test_home" "$helper") || fail "helper rejected $repo"
	[ "$got" = "$want" ] || fail "got '$got', want '$want'"
}

assert_rejected_without_stdout() {
	repo=$1
	if (cd "$repo" && "$helper") >"$tmp/stdout" 2>"$tmp/stderr"; then
		fail "helper accepted unsafe origin in $repo"
	fi
	[ ! -s "$tmp/stdout" ] || fail "helper emitted an ID for unsafe origin"
}

new_repo() {
	repo=$1
	git init -q "$repo"
	git -C "$repo" -c user.name=Test -c user.email=test@example.invalid \
		commit --allow-empty -m init >/dev/null
}

new_repo "$tmp/scp"
git -C "$tmp/scp" remote add origin git@github.com:Owner/repository.git
assert_id "$tmp/scp" github.com/Owner/repository

new_repo "$tmp/https"
git -C "$tmp/https" remote add origin https://GITHUB.COM/Owner/repository.git
assert_id "$tmp/https" github.com/Owner/repository

new_repo "$tmp/ssh"
git -C "$tmp/ssh" remote add origin ssh://git@GitHub.com/Owner/repository.git
assert_id "$tmp/ssh" github.com/Owner/repository

new_repo "$tmp/unsafe"
unsafe_origin='https://example.com/Owner/a:b.git'
git -C "$tmp/unsafe" remote add origin "$unsafe_origin"
unsafe_hash=$(printf %s 'example.com/Owner/a:b' | git hash-object --stdin | cut -c1-8)
assert_id "$tmp/unsafe" "example.com/Owner/a-b-$unsafe_hash"

new_repo "$tmp/no-origin"
common_dir=$(git -C "$tmp/no-origin" rev-parse --path-format=absolute --git-common-dir)
local_hash=$(printf %s "$common_dir" | git hash-object --stdin | cut -c1-8)
local_id="local/no-origin-$local_hash"
assert_id "$tmp/no-origin" "$local_id"
git -C "$tmp/no-origin" worktree add -q "$tmp/no-origin-worktree"
assert_id "$tmp/no-origin-worktree" "$local_id"

new_repo "$tmp/local source"
new_repo "$tmp/local-client"
local_origin=$(git -C "$tmp/local source" rev-parse --show-toplevel)
local_origin_hash=$(printf %s "$local_origin" | git hash-object --stdin | cut -c1-8)
git -C "$tmp/local-client" remote add origin "$local_origin"
assert_id "$tmp/local-client" "local/local-source-$local_origin_hash"

mkdir -p "$tmp/relative"
new_repo "$tmp/relative/source.git"
new_repo "$tmp/relative/client"
mkdir -p "$tmp/relative/client/nested"
relative_origin=$(git -C "$tmp/relative/source.git" rev-parse --show-toplevel)
relative_hash=$(printf %s "$relative_origin" | git hash-object --stdin | cut -c1-8)
git -C "$tmp/relative/client" remote add origin ../source.git

if git init -q --object-format=sha256 "$tmp/sha256" 2>/dev/null; then
	git -C "$tmp/sha256" -c user.name=Test -c user.email=test@example.invalid \
		commit --allow-empty -m init >/dev/null
	git -C "$tmp/sha256" remote add origin "$unsafe_origin"
	assert_id "$tmp/sha256" 'example.com/Owner/a-b-209ce61e'
else
	printf '%s\n' 'project-id test: SHA-256 repository unsupported; skipped' >&2
fi

assert_id "$tmp/relative/client" "local/source-$relative_hash"
assert_id_from "$tmp/relative/client" nested "local/source-$relative_hash"

new_repo "$tmp/locale"
git -C "$tmp/locale" remote add origin 'https://example.com/Owner/répo.git'
assert_id_with_locale "$tmp/locale" C 'example.com/Owner/r--po-69a10a08'
if LC_ALL=en_US.UTF-8 locale charmap >/dev/null 2>&1; then
	assert_id_with_locale "$tmp/locale" en_US.UTF-8 'example.com/Owner/r--po-69a10a08'
fi

git -C "$tmp/local-client" remote set-url origin "file://$local_origin"
assert_id "$tmp/local-client" "local/local-source-$local_origin_hash"
git -C "$tmp/local-client" remote set-url origin "file://localhost$local_origin"
assert_id "$tmp/local-client" "local/local-source-$local_origin_hash"
git -C "$tmp/local-client" remote set-url origin "file://LOCALHOST$local_origin"
assert_id "$tmp/local-client" "local/local-source-$local_origin_hash"
encoded_origin=$(printf '%s' "$local_origin" | sed 's/ /%20/g')
git -C "$tmp/local-client" remote set-url origin "file://$encoded_origin"
assert_id "$tmp/local-client" "local/local-source-$local_origin_hash"

new_repo "$tmp/source#repo.git"
hash_origin=$(git -C "$tmp/source#repo.git" rev-parse --show-toplevel)
hash_origin_hash=$(printf %s "$hash_origin" | git hash-object --stdin | cut -c1-8)
git -C "$tmp/local-client" remote set-url origin "$hash_origin"
assert_id "$tmp/local-client" "local/source-repo-$hash_origin_hash"
encoded_hash_origin=$(printf '%s' "$hash_origin" | sed 's/#/%23/g')
git -C "$tmp/local-client" remote set-url origin "file://$encoded_hash_origin"
assert_id "$tmp/local-client" "local/source-repo-$hash_origin_hash"

new_repo "$tmp/bad%2Gescape"
git -C "$tmp/local-client" remote set-url origin "file://$tmp/bad%2Gescape"
assert_rejected_without_stdout "$tmp/local-client"
new_repo "$tmp/bad%escape"
git -C "$tmp/local-client" remote set-url origin "file://$tmp/bad%escape"
assert_rejected_without_stdout "$tmp/local-client"
new_repo "$tmp/bad%00escape"
git -C "$tmp/local-client" remote set-url origin "file://$tmp/bad%00escape"
assert_rejected_without_stdout "$tmp/local-client"

mkdir -p "$tmp/home"
new_repo "$tmp/home/source.git"
tilde_origin=$(git -C "$tmp/home/source.git" rev-parse --show-toplevel)
tilde_hash=$(printf %s "$tilde_origin" | git hash-object --stdin | cut -c1-8)
git -C "$tmp/local-client" remote set-url origin "$tilde_origin"
assert_id_with_home "$tmp/local-client" "local/source-$tilde_hash" "$tmp/home"
git -C "$tmp/local-client" remote set-url origin '~/source.git'
assert_id_with_home "$tmp/local-client" "local/source-$tilde_hash" "$tmp/home"

new_repo "$tmp/newline"
newline_origin='https://example.com/Owner/line
break.git'
git -C "$tmp/newline" remote add origin "$newline_origin"
assert_rejected_without_stdout "$tmp/newline"

new_repo "$tmp/trailing-newline"
trailing_newline_origin='https://example.com/Owner/repository.git
'
git -C "$tmp/trailing-newline" remote add origin "$trailing_newline_origin"
assert_rejected_without_stdout "$tmp/trailing-newline"

printf '%s\n' 'project-id tests passed'
