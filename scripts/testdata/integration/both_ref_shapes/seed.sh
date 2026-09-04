#!/usr/bin/env bash
set -euo pipefail
# The ref:task half of the mixed dep — an existing task, listed as open.
existing_id=$(br create --title "existing open dep" --type feature --labels spex:existing-dep --json | jq -r '.id')
echo "$existing_id" > existing_id.txt
sed -i.bak "s/__EXISTING__/$existing_id/" changeset.json
rm -f changeset.json.bak
