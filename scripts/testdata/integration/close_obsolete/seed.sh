#!/usr/bin/env bash
set -euo pipefail
victim_id=$(br create --title "victim" --type feature --labels spex:55 --json | jq -r '.id')
# Write a synthetic mapping so the adapter can resolve {"ref":"spec_node",
# "spec_node_id":"sn-victim"} to the seeded bead.
cat > .bead-map.json <<EOF
{"records":[{"id":1,"spec_node_id":"sn-victim","bead_id":"$victim_id"}]}
EOF
