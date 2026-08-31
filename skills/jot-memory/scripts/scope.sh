#!/bin/sh
set -eu
LC_ALL=C
export LC_ALL

fail() {
	printf '%s\n' "scope: $1" >&2
	exit 1
}

[ "$#" -ge 3 ] || fail 'usage: scope.sh resolve|create VAULT global|project [PROJECT_ID]'
mode=$1
vault=$2
kind=$3
shift 3

case $mode in
	resolve|create) ;;
	*) fail 'mode must be resolve or create' ;;
esac
case $vault in
	/*) ;;
	*) fail 'vault must be absolute' ;;
esac
[ -d "$vault" ] || fail 'vault is not a directory'
vault=$(CDPATH= cd "$vault" && pwd -P) || fail 'cannot canonicalize vault'

for entry in "$vault"/*; do
	[ -e "$entry" ] || [ -L "$entry" ] || continue
	name=${entry##*/}
	lower=$(printf '%s' "$name" | tr 'ABCDEFGHIJKLMNOPQRSTUVWXYZ' 'abcdefghijklmnopqrstuvwxyz')
	if [ "$lower" = agent-memory ] && [ "$name" != agent-memory ]; then
		fail 'case-aliased agent-memory entry exists'
	fi
done

case $kind in
	global)
		[ "$#" -eq 0 ] || fail 'global scope takes no project ID'
		relative=agent-memory/global
		;;
	project)
		[ "$#" -eq 1 ] || fail 'project scope requires one project ID'
		project_id=$1
		case $project_id in
			''|/*|*/|*//* ) fail 'unsafe project ID' ;;
		esac
		rest=$project_id
		while :; do
			case $rest in
				*/*) segment=${rest%%/*}; rest=${rest#*/} ;;
				*) segment=$rest; rest= ;;
			esac
			case $segment in
				''|.|..|*[!A-Za-z0-9._-]*) fail 'unsafe project ID segment' ;;
			esac
			[ -n "$rest" ] || break
		done
		relative=agent-memory/projects/$project_id
		;;
	*) fail 'scope must be global or project' ;;
esac

current=$vault
memory_root=
rest=$relative
while :; do
	case $rest in
		*/*) component=${rest%%/*}; rest=${rest#*/} ;;
		*) component=$rest; rest= ;;
	esac
	next=$current/$component
	[ ! -L "$next" ] || fail "scope component is a symlink: $component"
	if [ ! -e "$next" ]; then
		[ "$mode" = create ] || exit 2
		mkdir "$next" || fail "cannot create scope component: $component"
	fi
	[ -d "$next" ] || fail "scope component is not a directory: $component"
	current=$(CDPATH= cd "$next" && pwd -P) || fail "cannot canonicalize scope component: $component"
	if [ -z "$memory_root" ]; then
		[ "$current" = "$vault/agent-memory" ] || fail 'agent-memory root escapes canonical vault'
		memory_root=$current
	else
		case $current/ in
			"$memory_root"/*) ;;
			*) fail 'scope escapes canonical agent-memory root' ;;
		esac
	fi
	[ -n "$rest" ] || break
done

printf '%s\n' "$current"
