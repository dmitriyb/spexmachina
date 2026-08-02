#!/usr/bin/env bash
#
# link-spread.sh — links belong in the prose, not appended to it.
#
# Usage: link-spread.sh <spec/MOD> [<base>]
#
#   <spec/MOD>  module directory to check — a directory holding module.json
#   <base>      baseline: a git ref, or a directory holding the baseline copy of
#               that same module directory      (default: origin/main)
#
# Why this exists: appending every declared link to a leaf satisfies
# link-check.sh, changes the leaf's bytes so the completeness checker is
# satisfied, and represents zero migration work. It is the cheapest way to pass
# every other gate while doing nothing. A link earns its place next to the
# sentence that makes the claim.
#
# ---------------------------------------------------------------------------
# How the dump is detected, and why not by line shape
#
# Two rounds of review established that a dump cannot be told from honest prose
# by the shape or the position of the lines it sits on. The densest honest
# leaves in the corpus (`spec/render/arch_render_command.md`, 8 content lines
# and 8 declared edges; `spec/validator/arch_validate_command.md`, 12 and 11)
# are indistinguishable by ratio or position from a wholesale end-of-file dump,
# because any tail window scales with the dump. Worse, the two presentation
# forms this migration mandates — a numbered markdown list and a two-column
# condition->outcome table, replacing control-flow fences — look exactly like a
# dump to any shape rule: `1. [[<id>|Name]] runs first` and
# `| [[<id>|Name]] | rejects |` are both short, both link-led, both repeated.
#
# So the primary rule uses the one thing a shape rule cannot see, and that this
# script already has in hand: <base>.
#
#   LINK_APPENDED  links were added to this leaf since <base>, and not one line
#                  of its prose changed.
#
# The prose view (check-lib.sh's `prose_lines_*`) is every line of the leaf with
# its `[[<id>|Name]]` links DELETED and its whitespace collapsed. Deleted, not
# replaced by the display text — so wrapping an existing bare name in a link
# changes that line, and the rule stays silent. An honest migration rewrites the
# sentences the links sit in, converts a fence into a list or a table, or at the
# very least edits the line it links from; every one of those removes a line
# from the base prose view. A dump leaves every existing line exactly as it was
# and adds new ones. That is the whole difference, and it holds regardless of
# where the links sit, what heading they are under, how much filler wraps them,
# or which presentation form they arrive in.
#
# Two more rules, both narrower:
#
#   LINK_FENCED    a `[[<id>|Name]]` sitting in a fenced or indented code block,
#                  an HTML comment or an inline code span, for an id the leaf
#                  does not link anywhere in visible text. Hidden text never
#                  satisfies a link obligation (link-check.sh agrees — both
#                  scripts count links through check-lib.sh's one scanner), so a
#                  real node id in a fence is an obligation being faked, not an
#                  example. To show the link syntax in a fence, use a
#                  placeholder that is not 12 hex digits, or link the node for
#                  real in the prose as well.
#   LINK_HEADING   a heading absent at <base>, at any level and with any text,
#                  whose section is nothing but links: at least two thirds of
#                  its content lines carry one, and it spends fewer than eight
#                  words of prose per link. Requiring EVERY line to carry a link
#                  let one three-word sentence appended to a `## References`
#                  full of links turn the whole file green. Lines in a mandated
#                  presentation form — an ordered-list step, a condition->outcome
#                  table row — count on neither side of the ratio.
#
# And one backstop, for the case the primary rule cannot see:
#
#   LINK_DUMP      at least half the leaf's links, and at least two of them, sit
#                  on lines that are *only* a link. Applied ONLY to a leaf that
#                  is new or empty at <base> — with no base prose there is
#                  nothing for LINK_APPENDED to compare. "Only a link" means one
#                  prose word or fewer once the links and any list marker are
#                  removed, and never a table row, so neither mandated
#                  presentation form trips it.
#
# LINK_CLUSTER is gone. It armed on 9 of the 62 leaves with declared edges, the
# two densest of those escaped a wholesale dump anyway, and it false-failed an
# honest closing paragraph of three link-carrying sentences. The header used to
# claim "at eight links or more LINK_CLUSTER catches it"; that claim was false.
# ---------------------------------------------------------------------------
#
# Output tags: LINK_FENCED, LINK_APPENDED, LINK_HEADING, LINK_DUMP
#
# Out-of-repo trees are first class: `link-spread.sh /tmp/gate/validator
# /tmp/base/validator` needs no git repository anywhere.
#
# Both sides must be MODULE directories. A spec root passed as <spec/MOD> or as
# <base> exits 2 — it used to examine zero leaves and exit 0.
#
# Exit: 0 every leaf's links are spread through its prose
#       1 at least one is not, one line each
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
install_err_trap link-spread.sh

dir=${1:-}
base=${2:-origin/main}

if [[ -z $dir ]]; then
    echo "usage: link-spread.sh <spec/MOD> [<base>]" >&2
    exit 2
fi
[[ -d $dir ]] || die "module dir '$dir' not found"
dir=${dir%/}
assert_module_dir "$dir"

base_resolve "$dir" "$base"

base_all=$(base_list) || die "cannot list '$dir' at base '$base'"
[[ -n $base_all ]] || die "'$dir' holds no files at base '$base' — wrong base or wrong module dir"
assert_base_module_dir "$base" "$base_all"

shopt -s nullglob
leaves=("$dir"/*.md)
shopt -u nullglob

base_md=0
while IFS= read -r p; do
    case $p in
        ''|*/*) continue ;;
        *.md) base_md=$((base_md + 1)) ;;
    esac
done <<<"$base_all"

if (( base_md > 0 && ${#leaves[@]} == 0 )); then
    die "'$dir' holds no markdown leaves but '$base' holds $base_md — wrong dir, or the tree is empty"
fi
if (( base_md == 0 && ${#leaves[@]} == 0 )); then
    die "'$dir' holds no markdown leaves, and neither does '$base' — nothing was examined"
fi

# Headings of a scan, as "<level>\t<text>", one per line.
scan_headings() {
    local scan=$1 kind f1 f2 f3 f4 f5
    while IFS=$'\t' read -r kind f1 f2 f3 f4 f5; do
        if [[ $kind == HD ]]; then printf '%s\t%s\n' "$f2" "$f3"; fi
    done <<<"$scan"
}

# Visible links in a scan.
count_vis_links() {
    local scan=$1 kind f1 f2 f3 f4 f5 n=0
    while IFS=$'\t' read -r kind f1 f2 f3 f4 f5; do
        if [[ $kind == LK && $f2 == vis ]]; then n=$((n + 1)); fi
    done <<<"$scan"
    printf '%s\n' "$n"
}

fail=0
examined=0
for leaf in ${leaves[@]+"${leaves[@]}"}; do
    [[ -f $leaf ]] || continue
    examined=$((examined + 1))

    scan=$(scan_file "$leaf") || die "cannot scan '$leaf'"

    # --- unpack the scan ---------------------------------------------------
    declare -A cls=() nlink=() bare=() words=() vis_ids=() form=()
    hid_hits=()
    content_lines=()            # line numbers of content lines, in order
    hd_lines=() hd_levels=() hd_texts=()
    total=0 bare_links=0 maxline=0

    while IFS=$'\t' read -r kind f1 f2 f3 f4 f5; do
        case $kind in
            LN)
                cls[$f1]=$f2; nlink[$f1]=$f3; bare[$f1]=$f4; words[$f1]=$f5
                if (( f1 > maxline )); then maxline=$f1; fi
                if [[ $f2 == content ]]; then
                    content_lines+=("$f1")
                    total=$((total + f3))
                    if (( f4 == 1 )); then bare_links=$((bare_links + f3)); fi
                fi
                ;;
            LK)
                if [[ $f2 == vis ]]; then vis_ids[$f3]=1; else hid_hits+=("$f1"$'\t'"$f3"); fi
                ;;
            HD)
                hd_lines+=("$f1"); hd_levels+=("$f2"); hd_texts+=("$f3")
                ;;
            FM)
                form[$f1]=$f2
                ;;
        esac
    done <<<"$scan"

    # --- the baseline copy of this leaf ------------------------------------
    name=$(basename -- "$leaf")
    base_scan=
    base_prose=
    base_links=0
    have_base=0
    if base_text=$(base_cat "$name"); then
        have_base=1
        base_scan=$(scan_text <<<"$base_text") || die "cannot scan '$name' at base '$base'"
        base_links=$(count_vis_links "$base_scan")
        base_prose=$(prose_lines_text <<<"$base_text") || die "cannot read '$name' at base '$base'"
    fi

    # --- LINK_FENCED: a real id hidden where it cannot count ---------------
    for hit in ${hid_hits[@]+"${hid_hits[@]}"}; do
        hl=${hit%%$'\t'*}
        hid_id=${hit#*$'\t'}
        if [[ -z ${vis_ids[$hid_id]+x} ]]; then
            echo "LINK_FENCED $leaf:$hl $hid_id (link is inside a fence, comment or code span; it does not count)"
            fail=1
        fi
    done

    # --- LINK_APPENDED: links added, prose untouched -----------------------
    if (( have_base == 1 && ${#base_prose} > 0 && total > base_links )); then
        # every base line that no longer survives, links deleted from both
        # sides. Zero means the author added and removed nothing.
        gone=$(LC_ALL=C comm -23 \
                   <(printf '%s\n' "$base_prose" | LC_ALL=C sort) \
                   <(prose_lines_file "$leaf" | LC_ALL=C sort) | wc -l)
        if (( gone == 0 )); then
            echo "LINK_APPENDED $leaf $((total - base_links)) links added since $base, and not one line of its prose changed"
            fail=1
        fi
    fi

    # --- LINK_DUMP: the backstop for a leaf with no baseline prose ----------
    if (( have_base == 0 || ${#base_prose} == 0 )); then
        if (( total >= 2 && bare_links >= 2 && 2 * bare_links >= total )); then
            lines=
            for n in ${content_lines[@]+"${content_lines[@]}"}; do
                if (( ${bare[$n]} == 1 )); then lines="${lines:+$lines,}$n"; fi
            done
            echo "LINK_DUMP $leaf $bare_links/$total links on link-only lines $lines"
            fail=1
        fi
    fi

    # --- LINK_HEADING: a new section that is nothing but links -------------
    declare -A base_hd=()
    while IFS= read -r hrec; do
        if [[ -n $hrec ]]; then base_hd[$hrec]=1; fi
    done < <(if [[ -n $base_scan ]]; then scan_headings "$base_scan"; fi)

    nh=${#hd_lines[@]}
    h=0
    while (( h < nh )); do
        key="${hd_levels[$h]}"$'\t'"${hd_texts[$h]}"
        if [[ -n ${base_hd[$key]+x} ]]; then h=$((h + 1)); continue; fi

        start=$(( ${hd_lines[$h]} + 1 ))
        if (( h + 1 < nh )); then end=$(( ${hd_lines[$((h + 1))]} - 1 )); else end=$maxline; fi

        # Lines written in a mandated presentation form — an ordered-list step,
        # a condition->outcome table row — are evidence of nothing and are
        # excluded from both sides of the ratio. `1. [[<id>|Name]] runs first`
        # repeated four times under `## Order of checks` is precisely what this
        # migration asks an author to write in place of a control-flow fence,
        # and it is shaped exactly like a dump.
        sec_links=0 sec_content=0 sec_linky=0 sec_words=0
        n=$start
        while (( n <= end )); do
            if [[ ${cls[$n]:-} == content && -z ${form[$n]:-} ]]; then
                sec_content=$((sec_content + 1))
                sec_links=$((sec_links + ${nlink[$n]}))
                sec_words=$((sec_words + ${words[$n]}))
                if (( ${nlink[$n]} > 0 )); then sec_linky=$((sec_linky + 1)); fi
            fi
            n=$((n + 1))
        done

        # "nothing but links": at least two thirds of the section's content
        # lines carry a link, and it spends fewer than eight words of prose per
        # link. An honest section spends far more than eight words on each claim
        # it links — three sentences of 11, 21 and 16 words carrying one link
        # each is 48 words for 3 links and stays silent. A "See also X for
        # more." list fails both halves, and appending one short sentence to it
        # no longer buys it a pass. The heading's TEXT is never consulted, so
        # renaming `## References` to `## Notes`, dropping to
        # `### Further reading` or `## SEE ALSO`, or appending `{#refs}`,
        # changes nothing.
        if (( sec_links >= 2 && sec_content > 0 && 3 * sec_linky >= 2 * sec_content \
              && sec_words < 8 * sec_links )); then
            echo "LINK_HEADING $leaf :: ${hd_texts[$h]} ($sec_links links, $sec_words words of prose in a section added since $base)"
            fail=1
        fi
        h=$((h + 1))
    done

    unset cls nlink bare words vis_ids form base_hd
done

if (( examined == 0 )); then
    die "no leaf of '$dir' was examined — wrong dir, or the tree is empty"
fi

exit $fail
