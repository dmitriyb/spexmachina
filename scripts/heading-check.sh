#!/usr/bin/env bash
#
# heading-check.sh — a leaf's frozen heading list may grow, never shrink.
#
# Usage: heading-check.sh <spec/MOD> [<base>]
#
#   <spec/MOD>  module directory to check — a directory holding module.json
#   <base>      baseline: a git ref, or a directory holding the baseline copy of
#               that same module directory      (default: origin/main)
#
# For every leaf present both at <base> and now, every `##` heading the leaf had
# at <base> must still be there. Added headings are fine — the list grows. This
# catches a wholesale arch-leaf rewrite, which is otherwise reported as nothing
# more interesting than modified/component.
#
# A leaf deleted outright is skipped: that is ladder-check.sh's business, and a
# deletion is visible in the diff as removed/<node_type> anyway. Leaves that do
# not exist at <base> are new and have no frozen list.
#
# `##` inside a fenced or indented code block or an HTML comment is not a
# heading and is ignored on both sides. Setext headings (`Foo` over `----`) are
# headings and are compared like any other. An ATX closing sequence is stripped,
# so normalising `## Foo ##` to `## Foo` is not a lost heading. Heading text is
# otherwise compared literally, so a heading that begins with `-` or contains
# `[[<id>|Name]]` compares correctly.
#
# Out-of-repo trees are first class: `heading-check.sh /tmp/gate/validator
# /tmp/base/validator` needs no git repository anywhere. Only the default base
# is a git ref and only that needs one.
#
# Both sides must be MODULE directories. A spec root passed as <spec/MOD> or as
# <base> exits 2 — it used to compare zero leaves and exit 0, masking every real
# LOST_HEADING in the module the operator meant to check. Zero leaves compared
# is never a pass.
#
# Output tags: LOST_HEADING
#
# Exit: 0 no heading was lost
#       1 at least one was, one line each
#       2 the check could not run (see check-lib.sh)
set -Eeuo pipefail

# --- library load, fail closed ---------------------------------------------
# `dirname "${BASH_SOURCE[0]}"` does not resolve symlinks, so invoked through a
# symlink the library was looked for beside the link: rc 1, which reads as
# "violation found". A missing library gave rc 1, an empty one rc 127 and one
# that exits while being sourced rc 0 — none of them the documented 2, because
# the ERR trap is installed only after the source succeeds. The EXIT trap below
# covers every one of those with rc 2 and is cleared only once the library has
# been confirmed complete.
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
install_err_trap heading-check.sh

dir=${1:-}
base=${2:-origin/main}

if [[ -z $dir ]]; then
    echo "usage: heading-check.sh <spec/MOD> [<base>]" >&2
    exit 2
fi
[[ -d $dir ]] || die "module dir '$dir' not found"
dir=${dir%/}
assert_module_dir "$dir"

base_resolve "$dir" "$base"

# Level-2 headings of one leaf, one per line, in file order.
leaf_headings() {
    local scan=$1 kind f1 f2 f3 f4
    while IFS=$'\t' read -r kind f1 f2 f3 f4; do
        if [[ $kind == HD && $f2 == 2 ]]; then printf '%s\n' "$f3"; fi
    done <<<"$scan"
}

base_all=$(base_list) || die "cannot list '$dir' at base '$base'"
if [[ -z $base_all ]]; then
    die "'$dir' holds no files at base '$base' — wrong base or wrong module dir"
fi
assert_base_module_dir "$base" "$base_all"

base_leaves=()
while IFS= read -r p; do
    case $p in
        ''|*/*) continue ;;
        *.md) base_leaves+=("$p") ;;
    esac
done <<<"$base_all"

shopt -s nullglob
now_leaves=("$dir"/*.md)
shopt -u nullglob
if (( ${#base_leaves[@]} > 0 && ${#now_leaves[@]} == 0 )); then
    die "'$dir' holds no markdown leaves but '$base' holds ${#base_leaves[@]} — wrong dir, or the tree is empty"
fi

fail=0
compared=0
while IFS= read -r leaf; do
    [[ -n $leaf ]] || continue
    [[ -f "$dir/$leaf" ]] || continue           # deleted leaf: ladder-check.sh's job
    compared=$((compared + 1))

    base_text=$(base_cat "$leaf") || die "cannot read '$leaf' from base '$base'"
    base_scan=$(scan_text <<<"$base_text") || die "cannot scan '$leaf' at base '$base'"
    now_scan=$(scan_file "$dir/$leaf") || die "cannot scan '$dir/$leaf'"

    declare -A now_set=()
    while IFS= read -r h; do
        if [[ -n $h ]]; then now_set[$h]=1; fi
    done < <(leaf_headings "$now_scan")

    while IFS= read -r h; do
        [[ -n $h ]] || continue
        if [[ -z ${now_set[$h]+x} ]]; then
            echo "LOST_HEADING $dir/$leaf :: $h"
            fail=1
        fi
    done < <(leaf_headings "$base_scan" | LC_ALL=C sort -u)

    unset now_set
done < <(printf '%s\n' ${base_leaves[@]+"${base_leaves[@]}"} | LC_ALL=C sort)

# Zero leaves compared is never a pass: the baseline had leaves and not one of
# them survived under the name it had, so the comparison this check exists to
# make did not happen.
if (( ${#base_leaves[@]} > 0 && compared == 0 )); then
    die "no leaf of '$dir' exists both at base '$base' and now — nothing was compared"
fi

exit $fail
