#!/usr/bin/env bash
set -euo pipefail
# The two ops flanking the bad one land as tasks; the bad one leaves no trace.
br list --json --all --limit 0 --label spex:deadbeefcafef00d:op-component-a1b2c3d4e5f6 \
    | jq -e '.issues | length == 1' >/dev/null || { echo "missing before-task"; exit 1; }
br list --json --all --limit 0 --label spex:deadbeefcafef00d:op-component-c3d4e5f6a1b2 \
    | jq -e '.issues | length == 1' >/dev/null || { echo "missing after-task"; exit 1; }
br list --json --all --limit 0 --label spex:deadbeefcafef00d:op-component-b2c3d4e5f6a1 \
    | jq -e '.issues | length == 0' >/dev/null || { echo "the bad op unexpectedly left a task"; exit 1; }
br list --json --all --limit 0 | jq -e '.issues | length == 2' >/dev/null || {
    echo "expected exactly 2 tasks in the sandbox"; br list --json --all --limit 0; exit 1;
}
