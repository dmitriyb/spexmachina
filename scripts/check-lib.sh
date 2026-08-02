#!/usr/bin/env bash
#
# check-lib.sh — shared helpers for the mechanical spec checks.
#
# Sourced, never executed. Provides:
#
#   die MSG...            exit 2 with a diagnostic on stderr (misuse/environment)
#   install_err_trap NAME install the fail-closed ERR/PIPE traps
#   need_jq               exit 2 unless jq is on PATH
#   assert_module_dir DIR       exit 2 unless DIR is a module directory
#   assert_base_module_dir B L  exit 2 unless the baseline listing L is one
#   base_resolve DIR BASE resolve <base> to a directory or a git ref
#   base_list             every file under the baseline copy of DIR, relative
#   base_cat REL          the baseline copy of DIR/REL on stdout, rc!=0 if absent
#   scan_file FILE        tab-separated line/link/heading records for a markdown leaf
#   scan_text             the same, reading the leaf from stdin
#   prose_lines_file FILE the leaf's lines with every link removed, blanks dropped
#   prose_lines_text      the same, reading the leaf from stdin
#   check_lib_loaded      rc 0 only when this library loaded completely
#
# ---------------------------------------------------------------------------
# Why every check fails closed
#
# A later wave reads exit 0 as licence to proceed. A check that cannot run must
# therefore never exit 0. The convention across all five scripts is:
#
#   0  the check ran and found nothing
#   1  the check ran and found a genuine violation
#   2  the check could not run: misuse, missing/malformed input, absent jq,
#      a nonexistent spec or module directory, a spec ROOT handed where a
#      module directory was expected (on either side), an unresolvable base,
#      an empty current tree where the baseline was not empty, zero leaves
#      compared where the baseline had leaves, or a library that would not load
#
# Each script therefore runs under `set -Eeuo pipefail` with an ERR trap that
# converts any unanticipated failure into exit 2 rather than letting it become
# an exit 1 that reads as "violation found" or an exit 0 that reads as "clean".
#
# The load of this file is itself guarded, in every script, by an EXIT trap that
# exits 2 and is cleared only once `check_lib_loaded` returns 0. A library that
# is absent, unreadable, empty, truncated, or that calls `exit` while being
# sourced therefore cannot produce a green run — nor an exit 1 that reads as a
# violation. The script's own path is resolved through symlinks first, so
# invoking a check through a symlink finds the library next to the real file.
#
# SIGPIPE: a reader that closes early (`link-check.sh ... | head -1`) used to
# kill the script with rc 141, outside the documented set. Each script traps
# PIPE and exits 2 — the report was truncated, so the check did not finish.
# The trap is a handler rather than `trap '' PIPE`, so child processes keep the
# default disposition instead of inheriting SIG_IGN.
#
# Locale: every `sort` in every check runs under `LC_ALL=C`, so a transcript
# taken on one machine matches a transcript taken on another.
# ---------------------------------------------------------------------------

# --- diagnostics -----------------------------------------------------------

die() { printf 'error: %s\n' "$*" >&2; exit 2; }

# Install the fail-closed traps. Call once, at the top of each script, right
# after the library has been verified. $1 is the script name used in messages.
install_err_trap() {
    local name=$1
    trap 'rc=$?; printf "error: %s: unexpected failure (rc=%s) at line %s\n" "'"$name"'" "$rc" "$LINENO" >&2; exit 2' ERR
    trap 'printf "error: %s: output pipe closed before the report was complete\n" "'"$name"'" >&2; exit 2' PIPE
}

need_jq() {
    command -v jq >/dev/null 2>&1 || die "jq is not on PATH"
    jq --version >/dev/null 2>&1 || die "jq is on PATH but does not run"
}

# --- what a module directory is --------------------------------------------
#
# A module directory holds `module.json`. A spec ROOT holds `project.json`
# and/or `<mod>/module.json`. Handed a root where `<spec/MOD>` was expected,
# heading-check.sh, link-spread.sh and ladder-check.sh used to compare zero
# leaves and exit 0 — a silent green over real violations, and the natural
# thing to reach for when an out-of-repo tree makes the default base fail.
# Both sides are guarded: current tree and baseline.

assert_module_dir() {
    local dir=$1 sub=()
    if [[ -f "$dir/module.json" ]]; then
        return 0
    fi
    shopt -s nullglob
    sub=("$dir"/*/module.json)
    shopt -u nullglob
    if [[ -f "$dir/project.json" ]] || (( ${#sub[@]} > 0 )); then
        die "'$dir' is a spec ROOT, not a module directory — pass <spec-dir>/<MOD>, e.g. '${dir%/}/${sub[0]:+$(basename -- "$(dirname -- "${sub[0]}")")}'"
    fi
    die "'$dir' holds no module.json — it is not a module directory"
}

# assert_base_module_dir BASE LISTING — LISTING is base_list output.
assert_base_module_dir() {
    local base=$1 listing=$2 p has_mj=0 is_root=0
    while IFS= read -r p; do
        case $p in
            module.json)   has_mj=1 ;;
            project.json)  is_root=1 ;;
            */module.json) case ${p%/module.json} in */*) : ;; *) is_root=1 ;; esac ;;
        esac
    done <<<"$listing"
    if (( has_mj == 1 )); then
        return 0
    fi
    if (( is_root == 1 )); then
        die "base '$base' holds a spec ROOT, not the baseline copy of this module directory — point <base> at <base-spec-root>/<MOD>"
    fi
    die "base '$base' holds no module.json for this module directory — wrong base"
}

# --- baseline resolution ---------------------------------------------------
#
# <base> is either a directory holding the baseline copy of DIR, or a git ref.
# A directory that exists always wins; anything else is tried as a ref.
#
# A tree outside any git repository is a first-class case: the wave gates an
# isolated copy under /tmp against another copy under /tmp. That works, and
# needs no repository, because a directory base never touches git. Only the
# default base (`origin/main`) needs one, and the diagnostic says so.

BASE_MODE=
BASE_DIR=
BASE_REF=
GIT_ROOT=
GIT_REL=

base_resolve() {
    local dir=$1 base=$2 abs
    abs=$(cd "$dir" 2>/dev/null && pwd) || die "cannot enter directory '$dir'"

    if [[ -d $base ]]; then
        BASE_MODE=dir
        BASE_DIR=$(cd "$base" 2>/dev/null && pwd) || die "cannot enter base directory '$base'"
        return 0
    fi
    if [[ -e $base ]]; then
        die "base '$base' exists but is not a directory, and cannot be a git ref"
    fi

    BASE_MODE=git
    BASE_REF=$base
    GIT_ROOT=$(git -C "$abs" rev-parse --show-toplevel 2>/dev/null) \
        || die "'$dir' is not inside a git repository, so base '$base' cannot be a git ref; pass a directory holding the baseline copy of '$dir' as <base> instead"
    git -C "$GIT_ROOT" rev-parse --verify --quiet "$base^{commit}" >/dev/null 2>&1 \
        || die "base ref '$base' does not resolve to a commit"
    if [[ $abs == "$GIT_ROOT" ]]; then
        die "'$dir' is the repository root; point the check at the spec directory"
    fi
    GIT_REL=${abs#"$GIT_ROOT"/}
    [[ $GIT_REL != "$abs" ]] || die "cannot express '$dir' relative to '$GIT_ROOT'"
}

# Every file under the baseline copy of DIR, one relative path per line.
# Empty output means the directory does not exist at <base> — callers that need
# a baseline treat that as exit 2, not as "nothing to check".
base_list() {
    if [[ $BASE_MODE == dir ]]; then
        (cd "$BASE_DIR" && find . -type f -print) | sed 's|^\./||'
    else
        local p
        git -C "$GIT_ROOT" ls-tree -r --name-only "$BASE_REF" -- "$GIT_REL" \
        | while IFS= read -r p; do
              [[ -n $p ]] || continue
              printf '%s\n' "${p#"$GIT_REL"/}"
          done
    fi
}

base_cat() {
    if [[ $BASE_MODE == dir ]]; then
        [[ -f "$BASE_DIR/$1" ]] || return 1
        cat -- "$BASE_DIR/$1"
    else
        git -C "$GIT_ROOT" show "$BASE_REF:$GIT_REL/$1" 2>/dev/null
    fi
}

# --- the leaf scanner ------------------------------------------------------
#
# One definition of "a link" shared by link-check.sh and link-spread.sh, so the
# two cannot disagree about what satisfies an obligation.
#
# A link is `[[<12 lowercase hex>|`, with the id optionally wrapped in double
# quotes: `[["8f2beb43e606"|Name]]` counts exactly like `[[8f2beb43e606|Name]]`.
# The quoted form exists because DOT node ids must be quoted — unquoted, a hex
# id beginning with a digit splits into two nodes under a real DOT parser, and
# 144 of the 229 corpus ids begin with a digit — so an author who has been
# quoting ids in a ```dot fence will quote them elsewhere too. Both forms
# resolve to the same 12-hex id.
#
# A link only counts when it is in *visible* text. These zones are not visible
# and never satisfy a link obligation:
#
#   * a fenced code block (``` or ~~~, any length >= 3, closed by the same
#     marker at the same or greater length with nothing after it)
#   * an INDENTED code block: a line indented four columns or more that opens
#     after a blank line and outside any list, through to the next non-blank
#     line indented less than four. Without this, eight
#     `    See [[id|Name]] for the parse step.` lines satisfied every link
#     obligation and tripped no spread rule — the fence dump in an uncovered
#     zone.
#   * an HTML comment `<!-- ... -->`, including one spanning several lines
#   * an inline code span `` `...` `` that opens and closes on one line
#
# An unterminated inline code span is treated as visible: hiding text on a
# stray backtick would let a link vanish from link-check.sh's view, which is
# the wrong direction to be wrong in.
#
# Headings. Both markdown heading forms are recognised, in visible text only:
#
#   * ATX — up to three leading spaces, `#`..`######`, a space, the text. An
#     optional closing sequence (`## Foo ##`) is stripped, so normalising a
#     heading to `## Foo` is not a lost heading.
#   * setext — a paragraph line followed by a line of `=` (level 1) or `-`
#     (level 2). The heading is recorded at the *text* line's number, which is
#     why records are emitted one line behind. Without this, deleting a setext
#     heading was invisible to heading-check.sh and needed no verdict from
#     ladder-check.sh.
#
# Records, tab separated, in line order:
#
#   LN <lineno> <class> <visible-links> <bare> <prose-words>
#        class ∈ content | blank | fence
#        blank covers empty lines and lines whose visible text is empty
#        (a line entirely inside an HTML comment, for instance), so neither
#        blank padding nor fence padding nor comment padding can move a
#        position-based window.
#        fence covers both fenced and indented code blocks.
#        prose-words counts what is left of the visible text once every link
#        and any list/quote/heading marker is removed.
#        bare = 1 when the line carries a link, is not a table row, and one
#        prose word or fewer is left. That is "a line that is only a link".
#        A numbered-list step (`1. [[id|Name]] runs first`) keeps two words and
#        a table row (`| [[id|Name]] | rejects |`) is excluded outright, because
#        the migration mandates both presentation forms and neither is a dump.
#   LK <lineno> <zone> <id>       zone ∈ vis | hid
#   HD <lineno> <level> <text>    heading in visible text, level 1..6
#   FM <lineno> <form>            form ∈ list | table — a content line written
#        in one of the two presentation forms this migration MANDATES for a
#        converted control-flow fence: an ordered-list step, or a row of a
#        two-column condition->outcome table. Emitted so that link-spread.sh can
#        refuse to treat either as evidence of a dump. Both are short, link-led
#        and repeated, which is to say indistinguishable by shape from the thing
#        the spread rules exist to catch; a rule that fires on the form the
#        migration asks for is a rule that punishes the work being requested.
#
# Portability: no interval regexes, no gensub, no length(array) — the program
# runs unchanged on mawk and gawk.

SCAN_AWK='
function trim(s) { sub(/^[ \t\r]+/, "", s); sub(/[ \t\r]+$/, "", s); return s }

# indent in columns, a tab counting as four
function indent_cols(line,   i, w, c) {
    w = 0; i = 1
    while (i <= length(line)) {
        c = substr(line, i, 1)
        if (c == " ") w++
        else if (c == "\t") w += 4
        else break
        i++
    }
    return w
}

# fence_run: does the line start a fence marker run? sets FR_CH, FR_N, FR_END.
function fence_run(line,   i, ch, n) {
    i = 1
    while (i <= length(line) && (substr(line, i, 1) == " " || substr(line, i, 1) == "\t")) i++
    ch = substr(line, i, 1)
    if (ch != "`" && ch != "~") return 0
    n = 0
    while (substr(line, i + n, 1) == ch) n++
    if (n < 3) return 0
    FR_CH = ch; FR_N = n; FR_END = i + n
    return 1
}

function emit_links(s, zone,   n, id) {
    n = 0
    while (match(s, LINKRE)) {
        # the match is `[[` + optional `"` + 12 hex + optional `"` + `|`; the
        # only hex characters in it are the id, so stripping everything else
        # yields the id whether or not the author quoted it.
        id = substr(s, RSTART, RLENGTH)
        gsub(/[^0-9a-f]/, "", id)
        BUF = BUF sprintf("LK\t%d\t%s\t%s\n", NR, zone, id)
        n++
        s = substr(s, RSTART + RLENGTH)
    }
    return n
}

# words left once the links and any leading marker are gone
function prose_words(s,   t, a) {
    t = s
    gsub(LINKFULL, " ", t)
    gsub(LINKRE, " ", t)
    sub(/^[ \t]*/, "", t)
    while (sub(/^>[ \t]*/, "", t)) { }
    sub(/^[-*+][ \t]+/, "", t)
    sub(/^[0-9]+[.)][ \t]+/, "", t)
    sub(/^#+[ \t]+/, "", t)
    gsub(/[^A-Za-z0-9]+/, " ", t)
    return split(t, a, " ")
}

# a pipe table row: starts and ends with `|` and has at least two of them
function table_row(s,   t, n, i) {
    t = trim(s)
    if (substr(t, 1, 1) != "|" || substr(t, length(t), 1) != "|") return 0
    n = 0
    for (i = 1; i <= length(t); i++) if (substr(t, i, 1) == "|") n++
    return (n >= 2)
}

function list_marker(t) {
    return (t ~ /^[-*+][ \t]/ || ordered_item(t))
}

function ordered_item(t) {
    return (t ~ /^[0-9]+[.)][ \t]/)
}

BEGIN {
    H = "[0-9a-f]"
    Q = "\"?"
    ID = H H H H H H H H H H H H
    LINKRE   = "\\[\\[" Q ID Q "\\|"
    LINKFULL = "\\[\\[" Q ID Q "\\|[^]]*\\]\\]"
    in_fence = 0; in_comment = 0; in_indent = 0; in_list = 0
    prev_blank = 1; prev_para = 0; prev_text = ""; PREV_BUF = ""
}

{
    line = $0
    vis = ""; hid = ""; cls = "content"
    BUF = ""
    is_atx = 0; is_underline = 0

    if (in_fence) {
        hid = line; cls = "fence"
        if (fence_run(line) && FR_CH == fence_ch && FR_N >= fence_len \
            && trim(substr(line, FR_END)) == "") in_fence = 0
    } else if (in_indent && (trim(line) == "" || indent_cols(line) >= 4)) {
        # inside an indented code block: blank lines belong to it, and so does
        # every line still indented four columns or more.
        hid = line
        cls = (trim(line) == "") ? "blank" : "fence"
    } else if (!in_comment && fence_run(line)) {
        in_indent = 0
        in_fence = 1; fence_ch = FR_CH; fence_len = FR_N
        hid = line; cls = "fence"
    } else if (!in_comment && !in_list && prev_blank && trim(line) != "" \
               && indent_cols(line) >= 4) {
        in_indent = 1
        hid = line; cls = "fence"
    } else {
        in_indent = 0
        rest = line
        while (length(rest) > 0) {
            if (in_comment) {
                p = index(rest, "-->")
                if (p == 0) { hid = hid rest; rest = "" }
                else { hid = hid substr(rest, 1, p + 2); rest = substr(rest, p + 3); in_comment = 0 }
                continue
            }
            p = index(rest, "<!--")
            q = index(rest, "`")
            if (p == 0 && q == 0) { vis = vis rest; rest = ""; continue }
            if (p > 0 && (q == 0 || p < q)) {
                vis = vis substr(rest, 1, p - 1)
                hid = hid "<!--"
                rest = substr(rest, p + 4)
                in_comment = 1
                continue
            }
            n = 0
            while (substr(rest, q + n, 1) == "`") n++
            tick = substr(rest, q, n)
            vis = vis substr(rest, 1, q - 1)
            after = substr(rest, q + n)
            r = index(after, tick)
            if (r == 0) { vis = vis tick after; rest = "" }        # unterminated: visible
            else { hid = hid substr(after, 1, r - 1); rest = substr(after, r + n) }
        }
        if (trim(vis) == "") cls = "blank"
    }

    nv = emit_links(vis, "vis")
    emit_links(hid, "hid")

    words = (cls == "content") ? prose_words(vis) : 0
    bare = (cls == "content" && nv > 0 && words <= 1 && !table_row(vis)) ? 1 : 0
    BUF = BUF sprintf("LN\t%d\t%s\t%d\t%d\t%d\n", NR, cls, nv, bare, words)

    tv = trim(vis)

    # --- a mandated presentation form ---------------------------------------
    if (cls == "content") {
        if (table_row(vis))        BUF = BUF sprintf("FM\t%d\ttable\n", NR)
        else if (ordered_item(tv)) BUF = BUF sprintf("FM\t%d\tlist\n", NR)
    }

    # --- ATX heading, up to three leading spaces, closing sequence stripped ---
    if (cls == "content" && indent_cols(line) < 4 && match(tv, /^#+[ \t]/)) {
        lev = 0
        while (substr(tv, lev + 1, 1) == "#") lev++
        if (lev <= 6) {
            is_atx = 1
            txt = substr(tv, lev + 1)
            gsub(/\t/, " ", txt)
            txt = trim(txt)
            if (match(txt, / #+$/)) txt = trim(substr(txt, 1, RSTART - 1))
            else if (txt ~ /^#+$/) txt = ""
            BUF = BUF sprintf("HD\t%d\t%d\t%s\n", NR, lev, txt)
        }
    }

    # --- setext heading: this line underlines the previous one ---------------
    # Tested on the RAW line. Stripping a hidden zone can leave `-` behind
    # (`- ` + an inline code span is a list item, not an underline), and a
    # setext underline is by definition a line of nothing but = or -.
    traw = trim(line)
    if (prev_para && cls == "content" && indent_cols(line) < 4 \
        && (traw ~ /^=+$/ || traw ~ /^-+$/)) {
        is_underline = 1
        ptxt = prev_text
        gsub(/\t/, " ", ptxt)                  # the record is tab separated
        PREV_BUF = PREV_BUF sprintf("HD\t%d\t%d\t%s\n", NR - 1, \
                                    (substr(traw, 1, 1) == "=") ? 1 : 2, ptxt)
    }

    # records are emitted one line behind so a setext heading can be recorded
    # at the line its text is on. Ordering is unchanged: line N-1 is flushed in
    # full before any record of line N.
    printf "%s", PREV_BUF
    PREV_BUF = BUF

    # List context, tracked only so that a blank-line-separated indented
    # continuation INSIDE a list is not mistaken for an indented code block.
    # Anything non-blank starting at column 0 that is not a list marker ends the
    # list — a heading, a fence, a paragraph. Missing that left in_list stuck at
    # 1 for the rest of the file after the first bullet.
    if (trim(line) != "") {
        if (cls == "content" && list_marker(tv)) in_list = 1
        else if (indent_cols(line) == 0) in_list = 0
    }
    prev_blank = (cls != "content")
    prev_para  = (cls == "content" && tv != "" && !is_atx && !is_underline \
                  && !list_marker(tv) && tv !~ /^>/ && !table_row(tv))
    prev_text  = tv
}

END { printf "%s", PREV_BUF }
'

# scan_file FILE — the leaf is fed on stdin so a path containing `=` or a
# leading `-` can never be read as an awk option or variable assignment.
scan_file() { awk "$SCAN_AWK" <"$1"; }
scan_text() { awk "$SCAN_AWK"; }

# --- the prose view --------------------------------------------------------
#
# Every line of a leaf with its `[[<id>|Name]]` links deleted, whitespace
# collapsed, and blank results dropped. This is the "did the author rewrite
# anything" view, and link-spread.sh's primary rule is built on it.
#
# It is deliberately blind to the links and to nothing else. Wrapping an
# existing bare name in a link changes the line, because the link is deleted
# rather than replaced by its display text. Appending a new link-bearing line
# changes no existing line. That difference is exactly the difference between
# migrating a leaf and dumping links into it — and unlike any line-shape rule,
# it does not care where the links sit, what heading they are under, how many
# filler words wrap them, or whether they arrive as a numbered list or a table.
#
# Every line is included, hidden zones as well as visible, so converting a
# control-flow fence into a numbered list or a condition->outcome table counts
# as a rewrite: the fence's lines are gone.

PROSE_AWK='
BEGIN {
    H = "[0-9a-f]"; Q = "\"?"
    ID = H H H H H H H H H H H H
    LINKFULL = "\\[\\[" Q ID Q "\\|[^]]*\\]\\]"
    LINKRE   = "\\[\\[" Q ID Q "\\|"
}
{
    t = $0
    gsub(LINKFULL, " ", t)
    gsub(LINKRE, " ", t)
    gsub(/[ \t\r]+/, " ", t)
    sub(/^ /, "", t); sub(/ $/, "", t)
    if (t != "") print t
}
'

prose_lines_file() { awk "$PROSE_AWK" <"$1"; }
prose_lines_text() { awk "$PROSE_AWK"; }

# --- load verification -----------------------------------------------------
#
# Defined last, and checked by every script under an EXIT trap that exits 2.
# A library that is empty, truncated, or that exits partway through sourcing
# never reaches this definition, so the trap fires and the run is exit 2 rather
# than an exit 0 that reads as clean or an exit 1 that reads as a violation.

check_lib_loaded() {
    local f
    for f in die install_err_trap need_jq assert_module_dir assert_base_module_dir \
             base_resolve base_list base_cat scan_file scan_text \
             prose_lines_file prose_lines_text; do
        declare -F "$f" >/dev/null 2>&1 || return 1
    done
    [[ -n ${SCAN_AWK:-} && -n ${PROSE_AWK:-} ]] || return 1
    return 0
}
