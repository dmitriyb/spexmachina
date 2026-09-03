#!/usr/bin/env bash
#
# export-br.sh — Reference adapter deriving the task-state artifact spex
# plan reads (through its required --tasks flag) from br.
#
# REFERENCE IMPLEMENTATION. Vet before production use. See spec/adapters/ for the
# adapter contract that any implementation (this one or your own) must satisfy.
#
# Usage: export-br.sh [<tasks.json>]
#
#   With no args: the task-state document on stdout.
#   With one arg: written atomically to that file.
#
# Test hooks (env vars):
#   BR_BIN   override the br binary path (default: br on PATH).

set -euo pipefail

BR_BIN="${BR_BIN:-br}"

if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq not on PATH" >&2
    exit 1
fi
if ! command -v "$BR_BIN" >/dev/null 2>&1; then
    echo "error: br ('$BR_BIN') not on PATH" >&2
    exit 1
fi

OUT=""
case "$#" in
    0) ;;
    1) OUT="$1" ;;
    *) echo "usage: export-br.sh [<tasks.json>]" >&2; exit 1 ;;
esac

# Status-unfiltered and unbounded: br list's defaults hide finished tasks
# and cap the row count, and either default would silently drop in-flight
# work from the projection below.
LISTING=$("$BR_BIN" list --json --all --limit 0)

# Project onto the version-1 task-state document: keep only tasks whose
# status is open or in_progress, carry id as task_id and status verbatim,
# drop every other field. Entries keep the listing's order.
DOC=$(jq -n --argjson listing "$LISTING" '{
    version: 1,
    tasks: (
        ($listing.issues // [])
        | map(select(.status == "open" or .status == "in_progress"))
        | map({task_id: .id, status: .status})
    )
}')

if [[ -n "$OUT" ]]; then
    tmp="${OUT}.tmp"
    jq . <<< "$DOC" > "$tmp"
    mv "$tmp" "$OUT"
else
    jq . <<< "$DOC"
fi
