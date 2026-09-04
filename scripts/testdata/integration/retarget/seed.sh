#!/usr/bin/env bash
set -euo pipefail
open_id=$(br create --title "retarget target" --type task --json | jq -r '.id')
dep1_id=$(br create --title "dep already carried" --type task --json | jq -r '.id')
dep2_id=$(br create --title "dep not yet carried" --type task --json | jq -r '.id')
br dep add "$open_id" "$dep1_id" --type blocks >/dev/null

echo "$open_id" > open_id.txt
echo "$dep1_id" > dep1_id.txt
echo "$dep2_id" > dep2_id.txt

sed -i.bak "s/__OPEN__/$open_id/; s/__DEP1__/$dep1_id/; s/__DEP2__/$dep2_id/" changeset.json
rm -f changeset.json.bak
