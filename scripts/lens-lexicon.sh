#!/usr/bin/env bash
# lens-lexicon.sh — spec-review mechanical lens: retired vocabulary sweep.
#
# Proposals that retire a term, flag, file, or concept declare it under a
# "## Retired vocabulary" heading — one term per list line ("- `term`").
# This script collects every declared term from every proposal (including
# obsolete/ — retirement is forever) and sweeps the spec corpus OUTSIDE
# spec/proposals/ for live mentions. Deliberate negative/historical mentions
# ("there is no --map flag") still count as hits unless excused by name in
# scripts/lens-lexicon.allow (<path><TAB><substring>, a reason above each
# entry — the same convention lens-dissolved-modules.allow uses): the allow
# file is where the judgment is recorded, not where it is hidden. An allow
# entry that no longer matches anything is itself an error, so excusals
# cannot outlive the lines they excuse.
#
# Exit 1 if any unexcused hit or dead allow entry is found; 0 otherwise.
set -euo pipefail
cd "$(dirname "$0")/.."

ALLOW="scripts/lens-lexicon.allow"
allow_paths=(); allow_texts=(); allow_used=()
if [ -f "$ALLOW" ]; then
    while IFS=$'\t' read -r apath atext; do
        case "$apath" in ''|'#'*) continue ;; esac
        [ -z "$atext" ] && continue
        allow_paths+=("$apath"); allow_texts+=("$atext"); allow_used+=(0)
    done < "$ALLOW"
fi

# excused <path> <line-content>: 0 when an allow entry covers this hit.
excused() {
    local path="$1" line="$2" i
    for i in "${!allow_paths[@]}"; do
        [ "${allow_paths[$i]}" = "$path" ] || continue
        case "$line" in *"${allow_texts[$i]}"*) allow_used[$i]=1; return 0 ;; esac
    done
    return 1
}

terms=$(awk '/^## Retired vocabulary/{f=1;next} /^## /{f=0} f && /^- /' \
    spec/proposals/*.md spec/proposals/obsolete/*.md 2>/dev/null \
    | sed -nE 's/^- *`([^`]+)`.*/\1/p' | sort -u || true)

if [ -z "$terms" ]; then
    echo "lens-lexicon: no retired-vocabulary declarations found in proposals"
    exit 0
fi

fail=0
while IFS= read -r term; do
    [ -z "$term" ] && continue
    hits=$(grep -rnF --include='*.md' --include='*.json' -- "$term" spec/ 2>/dev/null \
        | grep -v '^spec/proposals/' || true)
    [ -z "$hits" ] && continue
    kept=""
    while IFS= read -r hit; do
        hpath="${hit%%:*}"; rest="${hit#*:}"; hline="${rest#*:}"
        excused "$hpath" "$hline" && continue
        kept+="$hit"$'\n'
    done <<<"$hits"
    if [ -n "$kept" ]; then
        echo "RETIRED TERM '$term' still present:"
        printf '%s' "$kept" | sed 's/^/  /'
        fail=1
    fi
done <<<"$terms"
for i in "${!allow_paths[@]}"; do
    if [ "${allow_used[$i]}" -eq 0 ]; then
        echo "lens-lexicon: dead allow entry — ${allow_paths[$i]} no longer contains '${allow_texts[$i]}'" >&2
        fail=1
    fi
done
[ "$fail" -eq 0 ] && echo "lens-lexicon: corpus clean of all retired vocabulary"
exit "$fail"
