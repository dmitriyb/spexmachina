#!/usr/bin/env bash
set -euo pipefail
victim_id=$(jq -r '.records[0].bead_id' .bead-map.json)
br show "$victim_id" --format json | jq -e \
    '.[0].status == "closed" and ((.[0].labels // []) | (any(. == "spex:obsolete") and any(. == "commit:deadbeefcafef00d")))' >/dev/null \
    || { echo "$victim_id not properly closed/labeled"; br show "$victim_id" --format json; exit 1; }
