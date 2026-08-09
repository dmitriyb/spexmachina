#!/usr/bin/env bash
# lens-counts.sh — spec-review mechanical lens: written-out counts and
# enumerations vs the actual spec graph.
#
# Surfaces every sentence in a content leaf that states a count of graph
# objects (constructors, apis, commands, components, modules, subcommands),
# next to the graph's actual numbers from `spex render --format json --slim`.
# The reviewer judges each pairing — number words drift when nodes are added
# (the "twelve constructors" class). Report-only: always exits 0; the output
# is a worksheet, not a verdict.
set -euo pipefail
cd "$(dirname "$0")/.."

SLIM=$(bin/spex render --format json --slim)
echo "== graph actuals =="
jq -r '.nodes | group_by(.type) | map("\(.[0].type): \(length)") | .[]' <<<"$SLIM"
echo "apis by module:"
jq -r '[.nodes[] | select(.type=="api")] | group_by(.module) | map("  \(.[0].module): \(length)") | .[]' <<<"$SLIM"
echo
echo "== count claims in leaves =="
grep -rnEi '(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty|[0-9]+) (constructors?|apis?|commands?|components?|modules?|subcommands?|surfaces?)' \
    spec/*/*.md 2>/dev/null | grep -v '^spec/proposals/' || echo "(none)"
exit 0
