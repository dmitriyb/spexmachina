#!/usr/bin/env bash
set -euo pipefail
open_id=$(cat open_id.txt)
dep1_id=$(cat dep1_id.txt)
dep2_id=$(cat dep2_id.txt)

br show "$open_id" --format json | jq -e --arg l "spex:deadbeefcafef00d:op-retarget-a1b2c3d4e5f6" \
    '(.[0].labels // []) | any(. == $l)' >/dev/null || {
    echo "$open_id missing the retarget event label"; br show "$open_id" --format json; exit 1;
}
br show "$open_id" --format json | jq -e --arg d "$dep1_id" \
    '(.[0].dependencies // []) | any(.id == $d and .dependency_type == "blocks")' >/dev/null || {
    echo "$open_id missing the already-carried dep $dep1_id"; br show "$open_id" --format json; exit 1;
}
br show "$open_id" --format json | jq -e --arg d "$dep2_id" \
    '(.[0].dependencies // []) | any(.id == $d and .dependency_type == "blocks")' >/dev/null || {
    echo "$open_id missing the newly added dep $dep2_id"; br show "$open_id" --format json; exit 1;
}
