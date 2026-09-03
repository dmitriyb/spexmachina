#!/usr/bin/env bash
set -euo pipefail
victim_id=$(br create --title "victim" --type feature --labels spex:55 --json | jq -r '.id')
echo "$victim_id" > victim_id.txt
# Substitute the seeded task's real id into the close op's ref:task target —
# changesets carry no spex-owned file for the adapter to resolve a
# spec_node_id through, so the placeholder is filled in here instead.
sed -i.bak "s/__VICTIM__/$victim_id/" changeset.json
rm -f changeset.json.bak
