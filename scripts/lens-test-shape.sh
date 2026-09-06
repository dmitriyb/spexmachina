#!/usr/bin/env bash
# lens-test-shape.sh — spec-review mechanical lens: unit-shaped scenarios in test leaves.
#
# Test leaves (spec/*/test_*.md) hold module integration/acceptance scenarios; unit tests
# stay in Go _test.go files (spec/proposals/2026-03-09-test-strategy.md, Level 1). This
# lens flags the mechanical signature of a unit case: a scenario whose body calls a Go
# identifier in code (`Name(...)`) and never invokes a `spex` subcommand. A scenario is a
# `### ` heading without `#### ` children, or any `#### ` heading; bullet-style leaves
# (no scenario headings) are checked per bullet line.
#
# Measured on the 2026-09-06 audit of 749 scenarios (412 unit-shaped, 337 kept): the
# signature fires on 215 of the unit-shaped ones and on 24 of the kept ones. What it
# cannot see: a unit case written in prose without a code-formatted call. That residue is
# the /spec-review rubric's job and the /spec skill's authoring rule, not this lens's.
#
# A kept scenario that legitimately calls a Go identifier is excused by name in
# scripts/lens-test-shape.allow (<path><TAB><heading substring>, a reason above each
# entry — the lens-lexicon.allow convention): the allow file is where the judgment is
# recorded, not where it is hidden. An allow entry that no longer matches anything is
# itself an error, so excusals cannot outlive the scenarios they excuse.
#
# Every heading scenario must open with a bold `**Given**` line — the state and data it starts
# from; a scenario without one is reported as NO-GIVEN. Bullet cases (one-line scenarios in
# leaves without scenario headings) carry their state inline and are not held to it.
#
# Every test leaf must also carry a `## Setup` section — the shared ground its scenarios'
# Givens build on; a leaf without one is reported as NO-SETUP.
#
# Exit 1 if any unexcused hit or dead allow entry is found; 0 otherwise.
set -euo pipefail
cd "$(dirname "$0")/.."

ALLOW="scripts/lens-test-shape.allow"
allow_paths=(); allow_texts=(); allow_used=()
if [ -f "$ALLOW" ]; then
    while IFS=$'\t' read -r apath atext; do
        case "$apath" in ''|'#'*) continue ;; esac
        [ -z "$atext" ] && continue
        allow_paths+=("$apath"); allow_texts+=("$atext"); allow_used+=(0)
    done < "$ALLOW"
fi

# excused <path> <heading>: 0 when an allow entry covers this hit.
excused() {
    local path="$1" heading="$2" i
    for i in "${!allow_paths[@]}"; do
        [ "${allow_paths[$i]}" = "$path" ] || continue
        case "$heading" in *"${allow_texts[$i]}"*) allow_used[$i]=1; return 0 ;; esac
    done
    return 1
}

CALL='`[A-Z][A-Za-z0-9_.]*\('
SPEX='`spex [a-z-]+'

# scenarios <leaf>: prints "<start>\t<end>\t<heading>" per scenario.
scenarios() {
    local leaf="$1"
    if grep -q -E '^###+ ' "$leaf"; then
        grep -n -E '^##+ ' "$leaf" | awk -F: -v total="$(wc -l < "$leaf")" '
            { ln=$1; sub(/^[0-9]+:/,""); t=$0
              if (t ~ /^## /) { insetup = (t ~ /^## Setup/); next }   # Setup sub-headings are not scenarios
              if (insetup) next
              lvl=(t ~ /^#### /)?4:3; h=t; sub(/^#+ /,"",h)
              n++; L[n]=ln; V[n]=lvl; H[n]=h }
            END { for (i=1;i<=n;i++) {
                    if (V[i]==3 && i<n && V[i+1]==4) continue
                    end=(i<n)?L[i+1]-1:total
                    printf "%d\t%d\t%s\n", L[i], end, H[i] } }'
    else
        grep -n -E '^- ' "$leaf" | awk -F: '{ln=$1; sub(/^[0-9]+:/,""); h=$0; sub(/^- /,"",h); printf "%d\t%d\t%s\n", ln, ln, h}'
    fi
}

fail=0; checked=0; hits=0
for leaf in spec/*/test_*.md; do
    if ! grep -q -E '^## Setup' "$leaf"; then
        echo "NO-SETUP $leaf: test leaf has no '## Setup' section"
        fail=1
    fi
    while IFS=$'\t' read -r start end heading; do
        checked=$((checked+1))
        body=$(sed -n "${start},${end}p" "$leaf")
        if [ "$start" != "$end" ]; then   # heading scenario: must open with a Given line
            first=$(sed -n "$((start+1)),${end}p" "$leaf" | grep -m1 -v '^[[:space:]]*$' || true)
            case "$first" in '**Given**'*) ;; *)
                echo "NO-GIVEN $leaf:$start: $heading"; fail=1 ;;
            esac
        fi
        grep -q -E "$CALL" <<<"$body" || continue
        grep -q -E "$SPEX" <<<"$body" && continue
        excused "$leaf" "$heading" && continue
        echo "UNIT-SHAPED $leaf:$start: $heading"
        hits=$((hits+1)); fail=1
    done < <(scenarios "$leaf")
done
for i in "${!allow_paths[@]}"; do
    if [ "${allow_used[$i]}" -eq 0 ]; then
        echo "lens-test-shape: dead allow entry — ${allow_paths[$i]} has no scenario matching '${allow_texts[$i]}'" >&2
        fail=1
    fi
done
[ "$fail" -eq 0 ] && echo "lens-test-shape: $checked scenarios checked, none unit-shaped"
[ "$hits" -gt 0 ] && echo "lens-test-shape: $hits unit-shaped scenario(s) of $checked" >&2
exit "$fail"
