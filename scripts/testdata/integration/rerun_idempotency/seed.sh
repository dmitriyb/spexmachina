#!/usr/bin/env bash
set -euo pipefail
# Same preconditions happy_path establishes: the removed node's task, closed
# by op-0003, and the modified node's finished predecessor.
removed_id=$(br create --title "removed node" --type feature --labels spex:removed-node --json | jq -r '.id')
echo "$removed_id" > removed_id.txt
sed -i.bak "s/__REMOVED__/$removed_id/" changeset.json
rm -f changeset.json.bak

br create --title "modified node predecessor" --type feature --labels spex:predecessor --json >/dev/null
br list --json --all --limit 0 --label spex:predecessor | jq -r '.issues[0].id' | xargs -I{} br close {} --force >/dev/null

# First run: apply the changeset once, establishing exactly the tracker
# state a prior real run would have left. The harness's own invocation of
# changeset.json (below, after seed.sh returns) is the second, idempotent
# run this scenario tests.
"$SPEX_ADAPTER_SCRIPT" changeset.json >/dev/null

# Snapshot state right after the first run so verify.sh can assert the
# second run leaves it byte-identical.
br list --json --all --limit 0 | jq -S . > state_after_run1.json
