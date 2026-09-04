#!/usr/bin/env bash
set -euo pipefail
# tasks.json is produced by the harness (export-br.sh tasks.json, the
# <tasks.json> argument form) before this script runs.

jq -e '.version == 1' tasks.json >/dev/null || { echo "version != 1"; exit 1; }
jq -e '.tasks | length == 3' tasks.json >/dev/null || { echo "expected exactly 3 tasks"; jq . tasks.json; exit 1; }
jq -e '.tasks[] | keys == ["status","task_id"]' tasks.json >/dev/null || { echo "task entry carries an extra or missing key"; jq . tasks.json; exit 1; }

while IFS= read -r id; do
    jq -e --arg id "$id" '.tasks[] | select(.task_id == $id) | .status == "open"' tasks.json >/dev/null \
        || { echo "$id missing or not open in tasks.json"; jq . tasks.json; exit 1; }
done < open_ids.txt

claimed_id=$(cat claimed_id.txt)
jq -e --arg id "$claimed_id" '.tasks[] | select(.task_id == $id) | .status == "in_progress"' tasks.json >/dev/null \
    || { echo "$claimed_id missing or not in_progress in tasks.json"; jq . tasks.json; exit 1; }

while IFS= read -r id; do
    jq -e --arg id "$id" '[.tasks[] | select(.task_id == $id)] | length == 0' tasks.json >/dev/null \
        || { echo "closed task $id unexpectedly present in tasks.json"; jq . tasks.json; exit 1; }
done < closed_ids.txt

# TODO(bead:spexmachina-swvx.21): once PlanCommand's --tasks flag ships
# (still --beads today; PlanCommand and TaskReader are open beads in this
# epic), also assert `spex plan --tasks tasks.json ...` exits 0 here, per
# test_br_integration.md's "Export against a live sandbox" scenario.
