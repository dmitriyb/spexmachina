#!/usr/bin/env bash
set -euo pipefail
# The removed node's task, closed by op-0003.
removed_id=$(br create --title "removed node" --type feature --labels spex:removed-node --json | jq -r '.id')
echo "$removed_id" > removed_id.txt
sed -i.bak "s/__REMOVED__/$removed_id/" changeset.json
rm -f changeset.json.bak

# The modified node's finished predecessor. op-0004's changeset carries no
# dep naming it — plan decided that, not the adapter — so it exists here
# only to make verify.sh's "no dependency on it" assertion meaningful.
br create --title "modified node predecessor" --type feature --labels spex:predecessor --json >/dev/null
br list --json --label spex:predecessor | jq -r '.issues[0].id' | xargs -I{} br close {} --force >/dev/null
