#!/usr/bin/env bash
set -euo pipefail
# The mixed op's task must carry a blocks dependency on both the ref:op
# anchor (new-to-new) and the ref:task existing dep — the two ref shapes
# the changeset admits, resolved to the same edge type either way.
existing_id=$(cat existing_id.txt)
anchor_id=$(br list --json --all --limit 0 --label spex:deadbeefcafef00d:op-component-a1b2c3d4e5f6 | jq -r '.issues[0].id')
mixed_id=$(br list --json --all --limit 0 --label spex:deadbeefcafef00d:op-component-b2c3d4e5f6a1 | jq -r '.issues[0].id')

[[ -n "$anchor_id" && "$anchor_id" != "null" ]] || { echo "missing anchor task"; exit 1; }
[[ -n "$mixed_id" && "$mixed_id" != "null" ]] || { echo "missing mixed task"; exit 1; }

br show "$mixed_id" --format json | jq -e --arg a "$anchor_id" \
    '(.[0].dependencies // []) | any(.id == $a and .dependency_type == "blocks")' >/dev/null || {
    echo "mixed task $mixed_id missing blocks dep on ref:op anchor $anchor_id"; br show "$mixed_id" --format json; exit 1;
}
br show "$mixed_id" --format json | jq -e --arg e "$existing_id" \
    '(.[0].dependencies // []) | any(.id == $e and .dependency_type == "blocks")' >/dev/null || {
    echo "mixed task $mixed_id missing blocks dep on ref:task existing $existing_id"; br show "$mixed_id" --format json; exit 1;
}
