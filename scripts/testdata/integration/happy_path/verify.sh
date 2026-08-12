#!/usr/bin/env bash
set -euo pipefail
# Both labels must be present and the parent-child link must be wired.
br list --json --label spex:deadbeefcafef00d:op-0001 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:deadbeefcafef00d:op-0001"; exit 1; }
br list --json --label spex:deadbeefcafef00d:op-0002 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:deadbeefcafef00d:op-0002"; exit 1; }
parent_id=$(br list --json --label spex:deadbeefcafef00d:op-0001 | jq -r '.issues[0].id')
child_id=$(br list --json --label spex:deadbeefcafef00d:op-0002 | jq -r '.issues[0].id')
br show "$child_id" --format json | jq -e --arg pid "$parent_id" \
    '.[0].dependencies // [] | any(.id == $pid and .dependency_type == "parent-child")' >/dev/null || {
    echo "child $child_id missing parent-child link to $parent_id"; exit 1;
}
