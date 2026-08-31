#!/bin/sh
set -eu
LC_ALL=C
export LC_ALL

fail() {
	printf '%s\n' "project-id: $1" >&2
	exit 1
}

hash8() {
	printf '%s' "$1" | git --git-dir=/dev/null hash-object --stdin | cut -c1-8
}

hex_digit() {
	case $1 in
		[0-9]) digit=$1 ;;
		a|A) digit=10 ;;
		b|B) digit=11 ;;
		c|C) digit=12 ;;
		d|D) digit=13 ;;
		e|E) digit=14 ;;
		f|F) digit=15 ;;
		*) fail 'malformed percent escape in file URL' ;;
	esac
}

percent_decode() {
	encoded=$1
	decoded=
	while :; do
		case $encoded in
			*%*) prefix=${encoded%%\%*}; escaped=${encoded#*%} ;;
			*) printf '%s' "$decoded$encoded"; return ;;
		esac
		high=$(printf '%s' "$escaped" | cut -c1)
		low=$(printf '%s' "$escaped" | cut -c2)
		[ -n "$high" ] && [ -n "$low" ] || fail 'malformed percent escape in file URL'
		hex_digit "$high"; high=$digit
		hex_digit "$low"; low=$digit
		value=$((high * 16 + low))
		[ "$value" -ne 0 ] || fail 'NUL escape in file URL'
		[ "$value" -ne 10 ] && [ "$value" -ne 13 ] || fail 'line-break escape in file URL'
		octal=$(printf '%03o' "$value")
		decoded=$decoded$prefix$(printf "\\$octal")
		encoded=$(printf '%s' "$escaped" | cut -c3-)
	done
}

safe_segment() {
	segment=$1
	canonical=$2
	case $segment in
		''|.|..) fail 'unsafe empty or relative path segment' ;;
	esac
	safe=$(printf '%s' "$segment" | tr -c 'A-Za-z0-9._-' '-')
	if [ "$safe" != "$segment" ]; then
		safe="$safe-$(hash8 "$canonical")"
	fi
	printf '%s' "$safe"
}

safe_path() {
	path=$1
	canonical=$2
	case $path in
		''|/*|*/|*//* ) fail 'unsafe remote path' ;;
	esac
	result=
	while :; do
		case $path in
			*/*) segment=${path%%/*}; path=${path#*/} ;;
			*) segment=$path; path= ;;
		esac
		segment=$(safe_segment "$segment" "$canonical")
		if [ -n "$result" ]; then
			result="$result/$segment"
		else
			result=$segment
		fi
		[ -n "$path" ] || break
	done
	printf '%s\n' "$result"
}

local_id() {
	local_path=$1
	case $local_path in
		file://*)
			file_url=${local_path#file://}
			authority=${file_url%%/*}
			[ "$authority" != "$file_url" ] || fail 'file URL has no path'
			authority=$(printf '%s' "$authority" | tr '[:upper:]' '[:lower:]')
			case $authority in
				''|localhost) ;;
				*) fail 'unsupported file URL authority' ;;
			esac
			local_path=/$(percent_decode "${file_url#*/}")
			;;
		\~/*)
			[ -n "${HOME-}" ] || fail 'HOME is required for a tilde origin'
			local_path=$HOME/${local_path#\~/}
			;;
	esac
	case $local_path in
		/*) ;;
		*)
			repository=$(git rev-parse --show-toplevel 2>/dev/null) ||
				fail 'cannot resolve relative local origin outside a worktree'
			local_path=$repository/$local_path
			;;
	esac
	canonical=$(git -C "$local_path" rev-parse --show-toplevel 2>/dev/null) ||
		canonical=$(git -C "$local_path" rev-parse --absolute-git-dir 2>/dev/null) ||
		fail 'cannot canonicalize local origin'
	name=${canonical##*/}
	name=${name%.git}
	name=$(printf '%s' "$name" | tr -c 'A-Za-z0-9._-' '-')
	case $name in
		''|.|..) fail 'unsafe local repository name' ;;
	esac
	printf 'local/%s-%s\n' "$name" "$(hash8 "$canonical")"
}

if marked_origin=$(git remote get-url origin 2>/dev/null && printf x); then
	origin_output=${marked_origin%?}
	case $origin_output in
		*'
') origin=${origin_output%'
'} ;;
		*) fail 'origin output is not line terminated' ;;
	esac
else
	origin=
fi
if [ -z "$origin" ]; then
	common=$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null) ||
		fail 'not inside a Git repository'
	case $common in
		*/.git) repository=${common%/.git}; name=${repository##*/} ;;
		*) name=${common##*/}; name=${name%.git} ;;
	esac
	name=$(printf '%s' "$name" | tr -c 'A-Za-z0-9._-' '-')
	case $name in
		''|.|..) fail 'unsafe local repository name' ;;
	esac
	printf 'local/%s-%s\n' "$name" "$(hash8 "$common")"
	exit 0
fi

case $origin in
	*'
'*) fail 'remote contains a line break' ;;
esac

case $origin in
	file://*) local_id "$origin"; exit 0 ;;
	/*|./*|../*) local_id "$origin"; exit 0 ;;
	http://*) remote=${origin#http://} ;;
	https://*) remote=${origin#https://} ;;
	ssh://*)
		remote=${origin#ssh://}
		authority=${remote%%/*}
		[ "$authority" != "$remote" ] || fail 'remote has no path'
		host=${authority##*@}
		path=${remote#*/}
		;;
	*:* )
		authority=${origin%%:*}
		host=${authority##*@}
		path=${origin#*:}
		;;
	*) local_id "$origin"; exit 0 ;;
esac

if [ -z "${host-}" ]; then
	authority=${remote%%/*}
	[ "$authority" != "$remote" ] || fail 'remote has no path'
	host=${authority##*@}
	path=${remote#*/}
fi

host=$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')
path=${path%.git}
canonical="$host/$path"
safe_path "$canonical" "$canonical"
