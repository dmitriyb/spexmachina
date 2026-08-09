#!/usr/bin/env bash
# lens-lexicon.sh — spec-review mechanical lens: retired vocabulary sweep.
#
# Proposals that retire a term, flag, file, or concept declare it under a
# "## Retired vocabulary" heading — one term per list line ("- `term`").
# This script collects every declared term from every proposal (including
# obsolete/ — retirement is forever) and sweeps the spec corpus OUTSIDE
# spec/proposals/ for live mentions. Deliberate negative/historical mentions
# ("there is no --map flag") still count as hits: the reviewer judges them,
# the sweep only refuses to let them hide.
#
# Exit 1 if any hit is found; 0 otherwise.
set -euo pipefail
cd "$(dirname "$0")/.."

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
    if [ -n "$hits" ]; then
        echo "RETIRED TERM '$term' still present:"
        sed 's/^/  /' <<<"$hits"
        fail=1
    fi
done <<<"$terms"
[ "$fail" -eq 0 ] && echo "lens-lexicon: corpus clean of all retired vocabulary"
exit "$fail"
