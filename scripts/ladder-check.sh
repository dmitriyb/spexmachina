#!/usr/bin/env bash
#
# ladder-check.sh — every section of every deleted impl leaf has a verdict.
#
# Usage: ladder-check.sh <spec/MOD> <record> [<base>]
#
#   <spec/MOD>  module directory to check — a directory holding module.json
#   <record>    the implementer's record file for this unit
#   <base>      baseline: a git ref, or a directory holding the baseline copy of
#               that same module directory      (default: origin/main)
#
# Out-of-repo trees are first class: `ladder-check.sh /tmp/gate/validator rec.md
# /tmp/base/validator` needs no git repository anywhere. Only the default base
# is a git ref and only that needs one.
#
# Both sides must be MODULE directories. A spec root passed as <spec/MOD> or as
# <base> exits 2 — it used to find no impl leaves at all and report nothing.
#
# An impl leaf present at <base> and gone now takes its sections with it. This is
# the only thing tying the record to leaves that no longer exist: every `##`
# section of every deleted impl leaf, plus each leaf's `[preamble]`, must have a
# verdict line. One line per section:
#
#   <impl-leaf-basename> :: <heading text without "## "> :: arm <n> <ARM NAME> :: <destination or ->
#
# The four fields are split on the literal ` :: ` and on nothing else, so a
# heading carrying a `[[<id>|Name]]` link — the syntax segment 2 introduces —
# round-trips intact. A line with any other number of fields is reported, never
# silently folded into the last one.
#
# A section split at a fence boundary produces two lines with the same heading
# and a `(split 1/2)` / `(split 2/2)` suffix on the arm name. The two markers
# must sit on two different lines: one line carrying both would license a
# second, unmarked verdict for the same heading.
#
# The arm field is validated, not merely shaped. The number must be on the
# ladder and the name must be that number's name:
#
#   0 CONTRADICTS   1 SYNTAX       2 ALREADY SAID       3 CODE'S JOB
#   3.5 COST CLAIM  4 FALSIFIABLE  5 UNRECOVERABLE WHY  6 OTHERWISE
#
# Parentheticals — `(split 1/2)`, `(1.7 exception)` — are ignored when the name
# is compared.
#
# **Arm 0 is reported, with a non-zero exit.** It is not a defect in the record;
# it is an unresolved spec-versus-code contradiction that the plan requires the
# orchestrator to see and copy into `commit-plan.md` under `## Findings`. A
# check that stayed green on arm 0 would let one disappear.
#
# A record line naming a leaf that was not deleted — a leaf that never existed,
# an arch leaf, or an impl leaf that survived — is a violation. Half a record
# being fiction is exactly as bad as half of it being missing.
#
# Lines in <record> without a ` :: ` separator are prose and are ignored; a
# leading list marker (`- `, `* `) and backticks wrapping the whole line are
# stripped.
#
# Output tags: MISSING_RECORD, BAD_RECORD_LINE, BAD_ARM_FIELD, ARM_ZERO,
#              UNKNOWN_SECTION, UNKNOWN_LEAF, MISSING_VERDICT, BAD_SPLIT,
#              DUPLICATE_VERDICT
#
# Exit: 0 every section accounted for, no arm 0
#       1 otherwise, one line per problem
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
install_err_trap ladder-check.sh

dir=${1:-}
record=${2:-}
base=${3:-origin/main}

if [[ -z $dir || -z $record ]]; then
    echo "usage: ladder-check.sh <spec/MOD> <record> [<base>]" >&2
    exit 2
fi
[[ -d $dir ]] || die "module dir '$dir' not found"
dir=${dir%/}
assert_module_dir "$dir"

need_jq

# The environment is validated before the record is, so an unresolvable base or
# a mistyped module dir reports exit 2 rather than hiding behind MISSING_RECORD.
base_resolve "$dir" "$base"

if [[ ! -e $record ]]; then
    echo "MISSING_RECORD $record"
    exit 1
fi
[[ -f $record && -r $record ]] || die "record '$record' is not a readable file"

US=$'\x1f'

# --- the ladder ------------------------------------------------------------
# Values are the canonical names with apostrophes stripped, because that is how
# the comparison normalises both sides.
declare -A ARM_NAME=(
    [0]="CONTRADICTS"
    [1]="SYNTAX"
    [2]="ALREADY SAID"
    [3]="CODES JOB"
    [3.5]="COST CLAIM"
    [4]="FALSIFIABLE"
    [5]="UNRECOVERABLE WHY"
    [6]="OTHERWISE"
)

join_words() { local IFS=' '; echo "$*"; }

# Held in variables: bash's [[ ]] parser mis-reads a `)` inside a bracket
# expression written literally.
LIST_MARKER_RE='^([-*+][[:space:]]+|[0-9]+[.)][[:space:]]+)'
ARM_RE='^arm[[:space:]]+([0-9]+(\.[0-9]+)?)[[:space:]]+([^[:space:]].*)$'
SPLIT_RE='\(split[[:space:]]*([0-9]+)[[:space:]]*/[[:space:]]*([0-9]+)\)'
PAREN_RE='\([^)]*\)'

# --- what must be accounted for -------------------------------------------

base_all=$(base_list) || die "cannot list '$dir' at base '$base'"
[[ -n $base_all ]] || die "'$dir' holds no files at base '$base' — wrong base or wrong module dir"
assert_base_module_dir "$base" "$base_all"

declare -A base_impl=()
have_module_json=0
while IFS= read -r p; do
    case $p in
        ''|*/*)       continue ;;
        module.json)  have_module_json=1 ;;
        impl_*.md)    base_impl[$p]=1 ;;
    esac
done <<<"$base_all"

# Whatever module.json declared as an impl section counts too, so a leaf dropped
# from module.json and deleted in the same change is still covered.
if (( have_module_json == 1 )); then
    mj=$(base_cat module.json) || die "cannot read 'module.json' from base '$base'"
    declared=$(jq -r '(.impl_sections // [])[] | .content // empty' <<<"$mj") \
        || die "'module.json' at base '$base' is not valid JSON"
    while IFS= read -r c; do
        case $c in
            ''|*/*) continue ;;
            *)      base_impl[$c]=1 ;;
        esac
    done <<<"$declared"
fi

declare -A required=() deleted=()
for leaf in "${!base_impl[@]}"; do
    if [[ -f "$dir/$leaf" ]]; then continue; fi     # survived: nothing to account for
    deleted[$leaf]=1
    required["$leaf$US[preamble]"]=1
    text=$(base_cat "$leaf") || die "cannot read '$leaf' from base '$base'"
    scan=$(scan_text <<<"$text") || die "cannot scan '$leaf' at base '$base'"
    while IFS=$'\t' read -r kind _f1 f2 f3 _f4 _f5; do
        if [[ $kind == HD && $f2 == 2 && -n $f3 ]]; then required["$leaf$US$f3"]=1; fi
    done <<<"$scan"
done

# --- what the record claims ------------------------------------------------

declare -A seen=() marks=()
fail=0
lineno=0

while IFS= read -r raw || [[ -n $raw ]]; do
    lineno=$((lineno + 1))
    line=$raw

    line=${line#"${line%%[![:space:]]*}"}                       # strip indent
    line=${line%"${line##*[![:space:]]}"}                       # strip trailing space
    if [[ $line =~ $LIST_MARKER_RE ]]; then
        line=${line#"${BASH_REMATCH[0]}"}
    fi
    if [[ $line == '`'* && $line == *'`' ]]; then                # backticks wrapping the line
        while [[ $line == '`'* ]]; do line=${line#'`'}; done
        while [[ $line == *'`' ]]; do line=${line%'`'}; done
    fi

    [[ $line == *" :: "* ]] || continue                         # prose
    if [[ $line == *"$US"* ]]; then
        echo "BAD_RECORD_LINE $record:$lineno contains a raw unit separator: $raw"
        fail=1
        continue
    fi

    IFS=$US read -r -a fields <<<"${line// :: /$US}"
    if (( ${#fields[@]} != 4 )); then
        echo "BAD_RECORD_LINE $record:$lineno ${#fields[@]} fields, expected 4: $raw"
        fail=1
        continue
    fi
    leaf=${fields[0]}; head=${fields[1]}; arm=${fields[2]}; dest=${fields[3]}
    for v in leaf head arm dest; do
        printf -v "$v" '%s' "${!v#"${!v%%[![:space:]]*}"}"
        printf -v "$v" '%s' "${!v%"${!v##*[![:space:]]}"}"
    done
    if [[ -z $leaf || -z $head || -z $arm || -z $dest ]]; then
        echo "BAD_RECORD_LINE $record:$lineno empty field: $raw"
        fail=1
        continue
    fi

    # --- the arm field ---------------------------------------------------
    if [[ ! $arm =~ $ARM_RE ]]; then
        echo "BAD_ARM_FIELD $record:$lineno not 'arm <n> <NAME>': $arm"
        fail=1
        continue
    fi
    armnum=${BASH_REMATCH[1]}
    armrest=${BASH_REMATCH[3]}

    markers=()
    tmp=$armrest
    while [[ $tmp =~ $SPLIT_RE ]]; do
        markers+=("${BASH_REMATCH[1]}/${BASH_REMATCH[2]}")
        tmp=${tmp#*"${BASH_REMATCH[0]}"}
    done
    if (( ${#markers[@]} > 1 )); then
        echo "BAD_ARM_FIELD $record:$lineno one line carries ${#markers[@]} split markers (${markers[*]}): $arm"
        fail=1
        continue
    fi
    mark=${markers[0]:--}

    armname=$armrest
    while [[ $armname =~ $PAREN_RE ]]; do armname=${armname//"${BASH_REMATCH[0]}"/ }; done
    armname=${armname//\'/}
    armname=${armname//’/}
    read -r -a nwords <<<"$armname"
    armname=$(join_words ${nwords[@]+"${nwords[@]}"})
    armname=${armname^^}

    if [[ -z ${ARM_NAME[$armnum]+x} ]]; then
        echo "BAD_ARM_FIELD $record:$lineno arm '$armnum' is not on the ladder (0 1 2 3 3.5 4 5 6): $arm"
        fail=1
        continue
    fi
    if [[ $armname != "${ARM_NAME[$armnum]}" ]]; then
        echo "BAD_ARM_FIELD $record:$lineno arm $armnum is ${ARM_NAME[$armnum]}, not '$armname': $arm"
        fail=1
        continue
    fi

    if [[ $armnum == 0 ]]; then
        echo "ARM_ZERO $record:$lineno $leaf :: $head -> $dest (unresolved contradiction; report it under ## Findings)"
        fail=1
    fi

    # --- does the line name something real? ------------------------------
    key="$leaf$US$head"
    if [[ -n ${required[$key]+x} ]]; then
        seen[$key]=$(( ${seen[$key]:-0} + 1 ))
        marks[$key]="${marks[$key]:-}$mark"$'\n'
    elif [[ -n ${deleted[$leaf]+x} ]]; then
        echo "UNKNOWN_SECTION $record:$lineno $leaf :: $head"
        fail=1
    elif [[ -n ${base_impl[$leaf]+x} ]]; then
        echo "UNKNOWN_LEAF $record:$lineno $leaf was an impl leaf at $base but still exists — nothing to account for"
        fail=1
    else
        echo "UNKNOWN_LEAF $record:$lineno $leaf is not an impl leaf of '$dir' at $base"
        fail=1
    fi
done < "$record"

# --- verdict ---------------------------------------------------------------

while IFS= read -r key; do
    [[ -n $key ]] || continue
    leaf=${key%%"$US"*}
    head=${key#*"$US"}
    n=${seen[$key]:-0}
    if (( n == 0 )); then
        echo "MISSING_VERDICT $record $leaf :: $head"
        fail=1
        continue
    fi
    if (( n == 1 )); then
        if [[ ${marks[$key]} != $'-\n' ]]; then
            echo "BAD_SPLIT $record $leaf :: $head (one verdict marked (split ${marks[$key]//$'\n'/}), but there is no second half)"
            fail=1
        fi
        continue
    fi
    if (( n == 2 )); then
        got=$(printf '%s' "${marks[$key]}" | LC_ALL=C sort | tr '\n' ' ')
        if [[ $got != "1/2 2/2 " ]]; then
            echo "BAD_SPLIT $record $leaf :: $head (two verdicts marked '${got% }', not (split 1/2)+(split 2/2))"
            fail=1
        fi
        continue
    fi
    echo "DUPLICATE_VERDICT $record $leaf :: $head ($n lines)"
    fail=1
done < <(if (( ${#required[@]} > 0 )); then printf '%s\n' "${!required[@]}" | LC_ALL=C sort; fi)

exit $fail
