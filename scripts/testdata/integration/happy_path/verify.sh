#!/usr/bin/env bash
set -euo pipefail
# Both new-task labels must be present and the parent-child link must be wired.
br list --json --label spex:deadbeefcafef00d:op-0001 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:deadbeefcafef00d:op-0001"; exit 1; }
br list --json --label spex:deadbeefcafef00d:op-0002 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:deadbeefcafef00d:op-0002"; exit 1; }
parent_id=$(br list --json --label spex:deadbeefcafef00d:op-0001 | jq -r '.issues[0].id')
child_id=$(br list --json --label spex:deadbeefcafef00d:op-0002 | jq -r '.issues[0].id')
br show "$child_id" --format json | jq -e --arg pid "$parent_id" \
    '.[0].dependencies // [] | any(.id == $pid and .dependency_type == "parent-child")' >/dev/null || {
    echo "child $child_id missing parent-child link to $parent_id"; exit 1;
}

# The removed task is closed and carries no new labels — close ops apply none.
removed_id=$(cat removed_id.txt)
br show "$removed_id" --format json | jq -e \
    '.[0].status == "closed" and (.[0].labels // []) == ["spex:removed-node"]' >/dev/null || {
    echo "$removed_id not properly closed, or carries unexpected labels"; br show "$removed_id" --format json; exit 1;
}

# The modified node's new task carries no dependency on its finished
# predecessor — the changeset named none, and the adapter invented none.
br list --json --label spex:deadbeefcafef00d:op-0004 | jq -e '.issues | length == 1' >/dev/null || { echo "missing spex:deadbeefcafef00d:op-0004"; exit 1; }
modified_id=$(br list --json --label spex:deadbeefcafef00d:op-0004 | jq -r '.issues[0].id')
br show "$modified_id" --format json | jq -e '(.[0].dependencies // []) | length == 0' >/dev/null || {
    echo "$modified_id unexpectedly carries a dependency"; br show "$modified_id" --format json; exit 1;
}
