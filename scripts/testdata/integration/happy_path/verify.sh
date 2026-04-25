#!/usr/bin/env bash
set -euo pipefail
# Both labels must be present and the parent-child link must be wired.
br list --json --label spex:1 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:1"; exit 1; }
br list --json --label spex:2 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:2"; exit 1; }
parent_id=$(br list --json --label spex:1 | jq -r '.issues[0].id')
child_id=$(br list --json --label spex:2 | jq -r '.issues[0].id')
br show "$child_id" --format json | jq -e --arg pid "$parent_id" \
    '.[0].dependencies // [] | any(.id == $pid and .dependency_type == "parent-child")' >/dev/null || {
    echo "child $child_id missing parent-child link to $parent_id"; exit 1;
}
