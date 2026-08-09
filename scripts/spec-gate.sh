#!/usr/bin/env bash
#
# spec-gate.sh — the CI job that makes spex self-hosting: runs the structural
# pass (`spex validate`) and then the completeness pass (`spex diff`) against
# the PR's own tree, and asserts on both JSON verdicts rather than trusting
# exit status alone. See spec/delivery/arch_spec_gate.md.
#
# CI provides the seat (checkout, Go toolchain, building the binary from the
# PR's tree, the required-status-check job name); this script is the gate's
# verdict logic and nothing else — it never downloads a spex release.
#
# Usage: spec-gate.sh [<spec-dir>]
#
# Env:
#   SPEX_BIN   path to the spex binary built from the PR's tree (default: spex on PATH)
#
# Exit codes:
#   0 — structural pass clean, completeness pass clean (disclosures may print).
#   1 — structural pass failed, or spex diff did not build the tree at all.
#   2 — completeness pass found errors; each is surfaced verbatim.

set -uo pipefail

SPEX_BIN="${SPEX_BIN:-spex}"
SPEC_DIR="${1:-}"

if ! command -v jq >/dev/null 2>&1; then
    echo "spec gate: jq not on PATH" >&2
    exit 1
fi

spec_dir_flags=()
if [[ -n "$SPEC_DIR" ]]; then
    spec_dir_flags=(--spec-dir "$SPEC_DIR")
fi

print_notes() {
    local out="$1"
    if ! jq -e . "$out" >/dev/null 2>&1; then
        return
    fi
    local notes
    notes=$(jq -r '.notes[]? | "  note: [\(.type)] \(.message)"' "$out")
    if [[ -n "$notes" ]]; then
        echo "spec gate: disclosures (do not gate the verdict):"
        echo "$notes"
    fi
}

# ---- Structural pass --------------------------------------------------
# The gate reads the JSON verdict, not the exit status alone: a `valid: true`
# report with a non-zero finding count anywhere must still fail the job.
validate_out=$(mktemp)
"$SPEX_BIN" "${spec_dir_flags[@]}" validate >"$validate_out"
validate_rc=$?

if ! jq -e . "$validate_out" >/dev/null 2>&1; then
    echo "spec gate: structural pass — spex validate produced no parseable JSON verdict (exit $validate_rc)" >&2
    cat "$validate_out" >&2
    rm -f "$validate_out"
    exit 1
fi

valid=$(jq -r '.valid' "$validate_out")
error_count=$(jq -r '.error_count' "$validate_out")
warning_count=$(jq -r '.warning_count' "$validate_out")

if [[ "$valid" != "true" || "$error_count" != "0" || "$warning_count" != "0" ]]; then
    echo "spec gate: structural pass failed (valid=$valid, error_count=$error_count, warning_count=$warning_count)" >&2
    jq -r '.errors[]? | "  error: [\(.check)] \(.path): \(.message)"' "$validate_out" >&2
    rm -f "$validate_out"
    exit 1
fi
echo "spec gate: structural pass clean"
rm -f "$validate_out"

# ---- Completeness pass --------------------------------------------------
# Never pipe `diff` into a JSON filter: the pipe discards diff's own exit
# status, and on a build failure diff writes nothing to stdout — piping it
# into a filter would die with an opaque parse error naming no file, while a
# hypothetical clean-but-broken run would pass. Capture to a file, preserve
# the status, and branch on all three documented exit codes.
diff_out=$(mktemp)
"$SPEX_BIN" "${spec_dir_flags[@]}" diff --json >"$diff_out"
diff_rc=$?

case "$diff_rc" in
    0)
        echo "spec gate: completeness pass clean"
        print_notes "$diff_out"
        rm -f "$diff_out"
        exit 0
        ;;
    2)
        echo "spec gate: completeness pass failed" >&2
        jq -r '.errors[]? | "  error: [\(.type)] \(.message)"' "$diff_out" >&2
        print_notes "$diff_out"
        rm -f "$diff_out"
        exit 2
        ;;
    *)
        echo "spec gate: completeness pass — spex diff did not build the tree (exit $diff_rc)" >&2
        cat "$diff_out" >&2
        rm -f "$diff_out"
        exit 1
        ;;
esac
