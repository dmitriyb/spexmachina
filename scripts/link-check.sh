#!/usr/bin/env bash
#
# link-check.sh — every touched arch/flow leaf links the declared edges of its node.
#
# Usage: link-check.sh [<spec-dir>] [<diff.json>] [<module>]
#
#   <spec-dir>   spec root holding <mod>/module.json    (default: spec)
#   <diff.json>  output of `spex diff --json`           (default: diff.json)
#   <module>     optional filter: only this module is checked. Accepts either a
#                bare module name (`validator`) or a path (`spec/validator`).
#                A path must be a direct subdirectory of <spec-dir>: the filter
#                used to be basenamed unconditionally, so `/etc/render` was
#                accepted and quietly checked as `<spec-dir>/render`.
#                With no third argument the behaviour is identical to the
#                two-argument form.
#
# A unit links exactly the declared edges of the nodes whose leaves it rewrites:
#   component leaf : uses ∪ implements ∪ {api.id | this component ∈ api.provided_by}
#   data_flow leaf : uses
# "Touched" is read from the diff: changes of type modified or added whose
# node_type is component or data_flow.
#
# A link only counts in visible text. A `[[<id>|Name]]` inside a fenced OR an
# indented code block, inside an HTML comment or inside an inline code span is
# an example, not a link, and does not satisfy an obligation — see check-lib.sh.
# Dumping a leaf's declared edges into a ```text fence, or into eight
# four-space-indented lines, changes the leaf's bytes without doing any of the
# work, and this check exists to say so.
#
# Output tags:
#
#   MISSING_LINK  a touched leaf does not link one of its node's declared edges
#   MISSING_LEAF  a touched node declares edges but its content file is missing
#                 from module.json, or named there and absent from disk. There
#                 is nowhere for the links to go, so the obligation cannot be
#                 met. This was unreachable dead code until the row reader
#                 stopped collapsing the empty `content` field into the next
#                 one.
#
# Exit: 0 every required link present
#       1 at least one missing, one line each
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
install_err_trap link-check.sh

spec=${1:-spec}
diffjson=${2:-diff.json}
modarg=${3:-}

need_jq
[[ -d $spec ]] || die "spec dir '$spec' not found"
[[ -e $diffjson ]] || die "diff file '$diffjson' not found"
[[ -f $diffjson && -r $diffjson ]] || die "diff file '$diffjson' is not a readable file"

# The diff must parse and must actually be a spex diff document. A malformed or
# unrelated JSON file used to yield an empty touched set and a green exit.
shape=$(jq -r 'if type == "object" and (has("changes")) and ((.changes | type) == "array")
               then "ok" else "bad" end' "$diffjson" 2>/dev/null) \
    || die "'$diffjson' is not valid JSON"
[[ $shape == ok ]] || die "'$diffjson' is not a 'spex diff --json' document (no .changes array)"

# The module filter must name a real module OF THIS SPEC DIR. A mistyped <MOD>,
# or <spec-dir> passed where <spec-dir>/<MOD> was meant, used to check nothing
# and report success — the single most dangerous way for this script to be
# wrong. A filter given as a path is resolved and must sit directly under
# <spec-dir>; basenaming it unconditionally accepted `/etc/render` and checked
# `<spec-dir>/render` instead.
mod=
if [[ -n $modarg ]]; then
    modpath=${modarg%/}
    mod=${modpath##*/}
    case $mod in
        ''|'.'|'..') die "invalid module filter '$modarg'" ;;
    esac
    if [[ $modpath == */* ]]; then
        parent=$(cd -P -- "${modpath%/*}" >/dev/null 2>&1 && pwd) \
            || die "module filter '$modarg' names no existing directory"
        specabs=$(cd -P -- "$spec" >/dev/null 2>&1 && pwd) \
            || die "cannot enter spec dir '$spec'"
        [[ $parent == "$specabs" ]] \
            || die "module filter '$modarg' is not a module of spec dir '$spec' (it resolves under '$parent')"
    fi
    [[ -d "$spec/$mod" ]] || die "module filter '$modarg' names no directory '$spec/$mod'"
    [[ -f "$spec/$mod/module.json" ]] || die "'$spec/$mod/module.json' not found (filter '$modarg')"
fi

shopt -s nullglob
module_files=("$spec"/*/module.json)
shopt -u nullglob
if (( ${#module_files[@]} == 0 )); then
    die "no <mod>/module.json under '$spec' — wrong spec dir?"
fi

touched_raw=$(jq -r '.changes[]
    | select((.type == "modified" or .type == "added")
             and (.node_type == "component" or .node_type == "data_flow"))
    | .path // empty' "$diffjson") || die "cannot read changes from '$diffjson'"

declare -A touched=()
while IFS= read -r id; do
    if [[ -n $id ]]; then touched[$id]=1; fi
done <<<"$touched_raw"

# Fields are joined on a unit separator, not a tab. `IFS=$'\t' read` collapses
# consecutive tabs because tab is whitespace, so a component with `"content":
# ""` and declared edges used to land its FIRST EDGE ID in $file and nothing in
# $want — and was then skipped as "no declared edges", silently. \x1f is not
# whitespace, so an empty field stays an empty field. None of the three values
# can contain one: two are 12-hex id lists and the third is a file name.
US=$'\x1f'
ROWS_JQ='(.apis // []) as $a
  | ((.components // [])[] | . as $c
     | [$c.id, ($c.content // ""),
        ((($c.uses // []) + ($c.implements // [])
          + [$a[] | select(((.provided_by // []) | index($c.id)) != null) | .id])
         | unique | join(" "))]),
    ((.data_flows // [])[] | [.id, (.content // ""), (((.uses // []) | unique) | join(" "))])
  | map(tostring) | join("\u001f")'

fail=0
checked_modules=0

for mj in "${module_files[@]}"; do
    d=$(dirname -- "$mj")
    if [[ -n $mod && $(basename -- "$d") != "$mod" ]]; then
        continue
    fi
    checked_modules=$((checked_modules + 1))

    rows=$(jq -r "$ROWS_JQ" "$mj") || die "'$mj' is not valid JSON, or has an unexpected shape"

    while IFS=$US read -r id file want; do
        [[ -n $id ]] || continue
        [[ -n ${touched[$id]+x} ]] || continue      # not rewritten by this unit
        [[ -n $want ]] || continue                  # no declared edges to link

        if [[ -z $file ]]; then
            echo "MISSING_LEAF $d ($id) declares edges but has no content file"
            fail=1
            continue
        fi
        if [[ ! -f "$d/$file" ]]; then
            echo "MISSING_LEAF $d/$file ($id)"
            fail=1
            continue
        fi

        scan=$(scan_file "$d/$file") || die "cannot scan '$d/$file'"
        declare -A have=()
        while IFS=$'\t' read -r kind _f1 f2 f3 _f4; do
            if [[ $kind == LK && $f2 == vis ]]; then have[$f3]=1; fi
        done <<<"$scan"

        for w in $want; do
            if [[ -z ${have[$w]+x} ]]; then
                echo "MISSING_LINK $d/$file ($id) -> $w"
                fail=1
            fi
        done
        unset have
    done <<<"$rows"
done

if (( checked_modules == 0 )); then
    die "no module matched — nothing was checked"
fi

exit $fail
