#!/usr/bin/env bash
#
# no-rename-check.sh — an id that survives keeps its name, and keeps its title.
#
# Usage: no-rename-check.sh [<spec-dir>] [<base>]
#
#   <spec-dir>  spec root to check                (default: spec)
#   <base>      baseline: a git ref, or a directory holding the baseline spec
#                                                 (default: origin/main)
#
# Every id present at <base> AND still present now must carry the same `name`
# and the same `title`. This catches the one rename the change-type allowlist
# cannot see: editing a `title` while leaving `id` alone produces
# modified/requirement, which is allowlisted, and silently desyncs the identity
# hash.
#
# `name` and `title` are compared as *distinct* fields. Coalescing them
# (`.name // .title`) lets a `title` edit hide behind a newly added `name`: the
# field the check exists to protect changes while the check compares the other
# one. A field that exists now but not at base is an addition and has no
# baseline to violate.
#
# A removed id is NOT a rename: the comparison is over the intersection of the
# two id sets only. An added id has no baseline and is likewise ignored.
#
# An id is keyed by id alone, but every label it carries is collected. If one id
# appears in two files with two different labels — on either side — that is
# reported as AMBIGUOUS_ID and fails. Last-read-wins would otherwise let a
# rename in one file be masked by an unchanged copy in another.
#
# Scope: spec/project.json and spec/*/module.json. Every object anywhere in
# those files carrying a 12-hex `id` plus a `name` or `title` is covered, so
# modules, requirements, components, data_flows, test_sections,
# milestones, scenarios and apis are all included without enumeration.
#
# Output tags: RENAMED, AMBIGUOUS_ID
#
# Exit: 0 no survivor was renamed
#       1 at least one was, one line each
#       2 the check could not run (see check-lib.sh)
set -Eeuo pipefail

# --- library load, fail closed ---------------------------------------------
# See heading-check.sh for why this is not a one-line `source`.
__self=${BASH_SOURCE[0]}
__hops=0
while [[ -L $__self ]]; do
    __hops=$((__hops + 1))
    if (( __hops > 40 )); then printf 'error: %s: symlink loop resolving own path\n' "$0" >&2; exit 2; fi
    __d=$(cd -P -- "$(dirname -- "$__self")" >/dev/null 2>&1 && pwd) || __d=
    if [[ -z $__d ]]; then printf 'error: %s: cannot resolve own path\n' "$0" >&2; exit 2; fi
    __self=$(readlink -- "$__self") || { printf 'error: %s: cannot resolve own path\n' "$0" >&2; exit 2; }
    case $__self in /*) ;; *) __self=$__d/$__self ;; esac
done
LIB_DIR=$(cd -P -- "$(dirname -- "$__self")" >/dev/null 2>&1 && pwd) \
    || { printf 'error: %s: cannot resolve own directory\n' "$0" >&2; exit 2; }
CHECK_LIB=$LIB_DIR/check-lib.sh
[[ -f $CHECK_LIB && -r $CHECK_LIB ]] \
    || { printf 'error: %s: cannot read library %s\n' "$0" "$CHECK_LIB" >&2; exit 2; }
trap 'printf "error: %s: library %s failed to load\n" "$0" "$CHECK_LIB" >&2; exit 2' EXIT
# shellcheck source=scripts/check-lib.sh
source "$CHECK_LIB"
check_lib_loaded 2>/dev/null \
    || { printf 'error: %s: %s is not a complete check-lib.sh\n' "$0" "$CHECK_LIB" >&2; exit 2; }
trap - EXIT
install_err_trap no-rename-check.sh

spec=${1:-spec}
base=${2:-origin/main}

need_jq
[[ -d $spec ]] || die "spec dir '$spec' not found"
spec=${spec%/}

base_resolve "$spec" "$base"

# jq: pull (id, field, value) out of every object in the document that has a
# 12-hex id and a name and/or a title. name and title stay separate.
PAIRS_JQ='.. | objects
  | select(has("id") and ((.id | type) == "string") and (.id | test("^[0-9a-f]{12}$")))
  | . as $o
  | (if ($o | has("name")) then [$o.id, "name", ($o.name | tostring)] else empty end),
    (if ($o | has("title")) then [$o.id, "title", ($o.title | tostring)] else empty end)
  | @tsv'

# --- baseline --------------------------------------------------------------

base_all=$(base_list) || die "cannot list '$spec' at base '$base'"
[[ -n $base_all ]] || die "'$spec' holds no files at base '$base' — wrong base or wrong spec dir"

base_json=()
while IFS= read -r p; do
    case $p in
        project.json)   base_json+=("$p") ;;
        */module.json)  case ${p%/module.json} in */*) ;; *) base_json+=("$p") ;; esac ;;
    esac
done <<<"$base_all"

if (( ${#base_json[@]} == 0 )); then
    die "no project.json or <mod>/module.json under '$spec' at base '$base'"
fi

collect_base() {
    local f text
    for f in "${base_json[@]}"; do
        text=$(base_cat "$f") || { printf 'error: cannot read %s from base\n' "$f" >&2; return 1; }
        printf '%s\n' "$text" | jq -r "$PAIRS_JQ" \
            || { printf 'error: %s is not valid JSON at base\n' "$f" >&2; return 1; }
    done
}

# --- current ---------------------------------------------------------------

shopt -s nullglob
cur_json=()
if [[ -f "$spec/project.json" ]]; then cur_json+=("$spec/project.json"); fi
cur_json+=("$spec"/*/module.json)
shopt -u nullglob

if (( ${#cur_json[@]} == 0 )); then
    die "no project.json or <mod>/module.json under '$spec' — wrong spec dir, or the tree is empty"
fi

# --- compare ---------------------------------------------------------------
#
# base_val / cur_val are keyed by "<id>\t<field>" and hold a newline-joined SET
# of the values seen for that key, so two files disagreeing about one id is
# visible instead of one silently overwriting the other. cur_src records which
# files a key was seen in.

declare -A base_val=() cur_val=() cur_src=()

# set_add <assoc-name> <key> <member>
set_add() {
    local -n _set=$1
    local key=$2 member=$3 seen
    seen=${_set[$key]:-}
    if [[ $'\n'$seen$'\n' != *$'\n'"$member"$'\n'* ]]; then
        _set[$key]=${seen:+$seen$'\n'}$member
    fi
}

base_rows=$(collect_base) || die "cannot read the baseline spec at '$base'"
[[ -n ${base_rows//[[:space:]]/} ]] \
    || die "no ids found at base '$base' — wrong base or wrong spec dir"

while IFS=$'\t' read -r id field val; do
    [[ -n $id && -n $field ]] || continue
    set_add base_val "$id"$'\t'"$field" "$val"
done <<<"$base_rows"

for f in "${cur_json[@]}"; do
    rows=$(jq -r "$PAIRS_JQ" "$f") || die "'$f' is not valid JSON"
    while IFS=$'\t' read -r id field val; do
        [[ -n $id && -n $field ]] || continue
        set_add cur_val "$id"$'\t'"$field" "$val"
        set_add cur_src "$id"$'\t'"$field" "$f"
    done <<<"$rows"
done

if (( ${#cur_val[@]} == 0 )); then
    die "no ids found under '$spec' — wrong spec dir, or the tree is empty"
fi

fail=0
report=$(
    for key in "${!base_val[@]}"; do
        [[ -n ${cur_val[$key]+x} ]] || continue     # field or id removed: not a rename
        id=${key%%$'\t'*}
        field=${key#*$'\t'}
        b=${base_val[$key]}
        c=${cur_val[$key]}
        srcs=${cur_src[$key]//$'\n'/, }
        if [[ $c == *$'\n'* ]]; then
            printf 'AMBIGUOUS_ID %s (%s) %s has two values now: %s\n' \
                   "$srcs" "$id" "$field" "${c//$'\n'/ | }"
        fi
        if [[ $b == *$'\n'* ]]; then
            printf 'AMBIGUOUS_ID %s (%s) %s had two values at base: %s\n' \
                   "$srcs" "$id" "$field" "${b//$'\n'/ | }"
        fi
        if [[ $b != "$c" ]]; then
            # Only a single-valued side can be quoted as a value. Joining a set
            # with " | " and printing it as `-> "A | B"` reads as a rename to
            # that literal string; the set case says so instead, and the
            # AMBIGUOUS_ID line above carries the members.
            if [[ $b == *$'\n'* || $c == *$'\n'* ]]; then
                printf 'RENAMED %s (%s) %s changed; was {%s}, now {%s}\n' \
                       "$srcs" "$id" "$field" "${b//$'\n'/, }" "${c//$'\n'/, }"
            else
                printf 'RENAMED %s (%s) %s "%s" -> "%s"\n' \
                       "$srcs" "$id" "$field" "$b" "$c"
            fi
        fi
    done | LC_ALL=C sort -u
)

if [[ -n $report ]]; then
    printf '%s\n' "$report"
    fail=1
fi

exit $fail
