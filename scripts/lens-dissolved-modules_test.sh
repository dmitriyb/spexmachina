#!/usr/bin/env bash
#
# lens-dissolved-modules_test.sh — bash test harness for
# scripts/lens-dissolved-modules.sh.
#
# Every case builds a throwaway spec directory (project.json + .history.jsonl
# + one leaf) and runs the lens against it, so nothing depends on the state of
# this repo's own spec/. Two properties are under test: the DERIVATION (which
# module names the journal and project.json make dissolved) and the MATCHING
# (which textual forms count as naming one, and which are ordinary English).
#
# Exit code: 0 if every case passes; 1 if any fails. Every case runs.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
LENS="$ROOT/lens-dissolved-modules.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

pass=0; fail=0
ok()   { printf '  ok   %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  FAIL %s\n     %s\n' "$1" "$2"; fail=$((fail+1)); }

# fixture <dir> <leaf-body> [<journal-line>...]
fixture() {
    local dir="$1" body="$2"; shift 2
    mkdir -p "$dir/proposals"
    cat > "$dir/project.json" <<'JSON'
{"name": "demo", "modules": [{"id": "aaaaaaaaaaaa", "name": "merkle", "path": "merkle"}]}
JSON
    : > "$dir/.history.jsonl"
    local line
    for line in "$@"; do printf '%s\n' "$line" >> "$dir/.history.jsonl"; done
    mkdir -p "$dir/merkle"
    printf '%s\n' "$body" > "$dir/merkle/arch_thing.md"
}

removed_emit='{"event":"removed","eid":"a:1","node":"aaaaaaaaaaa1","name":"X","node_type":"component","module":"emit","before":"h","after":null,"git_head":"a","proposal":"p"}'
removed_live='{"event":"removed","eid":"a:2","node":"aaaaaaaaaaa2","name":"Y","node_type":"component","module":"merkle","before":"h","after":null,"git_head":"a","proposal":"p"}'

# run <name> <expected-exit> <must-contain-or-empty> <leaf-body> [<journal-line>...]
run() {
    local name="$1" want="$2" needle="$3" body="$4"; shift 4
    local dir="$tmp/$(echo "$name" | tr -c 'a-zA-Z0-9' '_')"
    fixture "$dir" "$body" "$@"
    local out; out="$("$LENS" "$dir" /dev/null 2>&1)"; local got=$?
    if [ "$got" -ne "$want" ]; then bad "$name" "exit $got, want $want: $out"; return; fi
    if [ -n "$needle" ] && ! grep -qF -- "$needle" <<<"$out"; then
        bad "$name" "output missing '$needle': $out"; return
    fi
    ok "$name"
}

echo "derivation"
run "removed module absent from project.json is dissolved" 1 "DISSOLVED MODULE 'emit'" "emit's ordering" "$removed_emit"
run "removed node in a LIVE module is not dissolved"       0 "no dissolved modules" "merkle's ordering" "$removed_live"
run "no removed events at all"                             0 "no dissolved modules" "emit's ordering"
run "malformed journal line costs one event, not the run"  1 "DISSOLVED MODULE 'emit'" "emit's ordering" 'not json at all' "$removed_emit"

echo "matched forms"
run "possessive"           1 "arch_thing.md" "derived by negating emit's types"                  "$removed_emit"
run "possessive, curly"    1 "arch_thing.md" "derived by negating emit’s types"                  "$removed_emit"
run "arrow, unicode"       1 "arch_thing.md" "spec change → emit → bead actions"                 "$removed_emit"
run "arrow, ascii"         1 "arch_thing.md" "spec change -> emit -> bead actions"               "$removed_emit"
run "arrow, trailing only" 1 "arch_thing.md" "emit → bead actions close the cycle"                "$removed_emit"
run "arrow, leading only"  1 "arch_thing.md" "the cycle ends at → emit"                           "$removed_emit"
run "parenthesised"        1 "arch_thing.md" "requirement 'R' (emit) description changed"        "$removed_emit"
run "path fragment"        1 "arch_thing.md" 'see emit/arch_changeset_builder.md for the rest'   "$removed_emit"
run "json module field"    1 "arch_thing.md" '{"node_type": "component", "module": "emit"}'      "$removed_emit"
run "noun phrase"          1 "arch_thing.md" "the emit module owned that step"                   "$removed_emit"
run "backticked possessive" 1 "arch_thing.md" "negating \`emit\`'s bead-producing types"          "$removed_emit"
run "backticked noun"      1 "arch_thing.md" "the \`emit\` module owned that step"                 "$removed_emit"
run "backticked arrow"     1 "arch_thing.md" "spec change → \`emit\` → bead actions"               "$removed_emit"

echo "ordinary English is not a reference"
run "verb"                 0 "corpus clean" "Emit the ops in that order."                        "$removed_emit"
run "verb, backticked"     0 "corpus clean" "3. **Emit \`receipts.json\`.**"                     "$removed_emit"
run "reverse noun phrase"  0 "corpus clean" "per-module emit views are rendered separately"      "$removed_emit"
run "substring"            0 "corpus clean" "the emitter emits emitted events"                   "$removed_emit"

# A module.json description reaches disk with the arrow ESCAPED, which is how
# the real defect (spec/proposal/module.json) evaded a literal-arrow pattern.
mkdir -p "$tmp/escape_case"
fixture "$tmp/escape_case" "nothing here" "$removed_emit"
cat > "$tmp/escape_case/merkle/module.json" <<'JSON'
{"name":"merkle","data_flows":[{"description":"spec change \u2192 emit \u2192 bead actions"}]}
JSON
out="$("$LENS" "$tmp/escape_case" /dev/null 2>&1)"; got=$?
if [ "$got" -eq 1 ] && grep -qF "module.json" <<<"$out"; then
    ok "arrow as a \\u2192 escape inside module.json"
else
    bad "arrow as a \\u2192 escape" "exit $got: $out"
fi

# A name that is not a plain module name must stop the sweep, not sweep wrongly:
# under ugrep `c++plus` parses as a possessive quantifier and matches nothing,
# which would print "corpus clean" over a corpus never actually searched.
removed_meta='{"event":"removed","eid":"a:3","node":"aaaaaaaaaaa3","name":"Z","node_type":"component","module":"c++plus","before":"h","after":null,"git_head":"a","proposal":"p"}'
mkdir -p "$tmp/meta_case"
fixture "$tmp/meta_case" "nothing here" "$removed_meta"
out="$("$LENS" "$tmp/meta_case" /dev/null 2>&1)"; got=$?
if [ "$got" -eq 2 ] && grep -qF "refusing to sweep" <<<"$out"; then
    ok "a name that is not a plain module name stops the sweep"
else
    bad "regex-metacharacter name" "exit $got: $out"
fi

echo "allow list"
# allow_case <name> <expect-exit> <needle> <allow-body>
allow_case() {
    local name="$1" want="$2" needle="$3" body="$4"
    local dir="$tmp/allow_$(echo "$name" | tr -c 'a-zA-Z0-9' '_')"
    fixture "$dir" "derived by negating emit's types" "$removed_emit"
    printf '%b' "$body" > "$dir.allow"
    local out; out="$("$LENS" "$dir" "$dir.allow" 2>&1)"; local got=$?
    if [ "$got" -eq "$want" ] && grep -qF -- "$needle" <<<"$out"; then ok "$name"
    else bad "$name" "exit $got (want $want): $out"; fi
}

allow_case "exact entry excuses the hit"            0 "1 excused"    'merkle/arch_thing.md\tnegating emit'"'"'s types\n'
allow_case "same path, different text still fires"  1 "DISSOLVED"    'merkle/arch_thing.md\tsome other line entirely\n'
allow_case "same text, different path still fires"  1 "DISSOLVED"    'merkle/other.md\tnegating emit'"'"'s types\n'
allow_case "an entry matching nothing is reported"  1 "dead allow entry" 'merkle/other.md\tnegating emit'"'"'s types\n'
allow_case "comments and blanks are ignored"        1 "DISSOLVED"    '# just a reason\n\n'

echo "scope"
mkdir -p "$tmp/proposals_case"
fixture "$tmp/proposals_case" "nothing to see here" "$removed_emit"
printf '%s\n' "emit's ordering is retired" > "$tmp/proposals_case/proposals/2026-01-01-old.md"
out="$("$LENS" "$tmp/proposals_case" /dev/null 2>&1)"; got=$?
if [ "$got" -eq 0 ] && grep -qF "corpus clean" <<<"$out"; then
    ok "proposals/ is exempt — retirement is forever"
else
    bad "proposals/ is exempt" "exit $got: $out"
fi

out="$("$LENS" "$tmp/does-not-exist" /dev/null 2>&1)"; got=$?
if [ "$got" -eq 0 ] && grep -qF "nothing to derive" <<<"$out"; then
    ok "missing spec dir is not a failure"
else
    bad "missing spec dir" "exit $got: $out"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
