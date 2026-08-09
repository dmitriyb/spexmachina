#!/usr/bin/env bash
#
# mock_spex.sh — stands in for the spex binary in spec-gate_test.sh's
# mock-mode cases. It exercises spec-gate.sh's own branching logic (the JSON
# verdict assertion, the notes-never-gate contract) against verdict shapes
# that spex's real validate/diff cannot produce today but the gate must
# still handle defensively — real validate/diff exit-code behavior is
# covered by cmd/spex's own test suite and by this harness's real-binary
# cases.
#
# Env:
#   MOCK_VALIDATE_STDOUT / MOCK_VALIDATE_EXIT
#   MOCK_DIFF_STDOUT     / MOCK_DIFF_EXIT

set -uo pipefail

sub=""
for a in "$@"; do
    case "$a" in
        validate|diff) sub="$a" ;;
    esac
done

case "$sub" in
    validate)
        printf '%s' "${MOCK_VALIDATE_STDOUT:-}"
        exit "${MOCK_VALIDATE_EXIT:-0}"
        ;;
    diff)
        printf '%s' "${MOCK_DIFF_STDOUT:-}"
        exit "${MOCK_DIFF_EXIT:-0}"
        ;;
    *)
        echo "mock_spex: unrecognized args, expected a validate or diff subcommand: $*" >&2
        exit 1
        ;;
esac
