#!/usr/bin/env bash
#
# spec-gate_test.sh — bash test harness for scripts/spec-gate.sh.
#
# Two modes:
#
#  1. Real-binary cases: build spex from this tree and run the gate against
#     fixture spec directories derived from this repo's own (valid) spec/ —
#     the clean pass, the completeness-error pass, and the two ways a build
#     can fail (a broken snapshot the diff pass alone reads, and an uncovered
#     requirement the structural pass alone reads). Exercises the real
#     `spex validate` / `spex diff` exit-code contract end to end.
#
#  2. Mock-binary cases, against testdata/spec-gate/mock_spex.sh: verdict
#     shapes the real binary cannot produce today but the gate must still
#     handle defensively — a `valid: true` verdict with a non-zero finding
#     count, and diff notes that must never gate the exit code.
#
# Exit code: 0 if every case passes; 1 on first failure.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"
SPEC_GATE="$ROOT/spec-gate.sh"
MOCK_SPEX="$ROOT/testdata/spec-gate/mock_spex.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "jq not on PATH — cannot run spec-gate tests" >&2
    exit 1
fi

PASS=0
FAIL=0
FAILURES=()

report_ok()   { PASS=$((PASS + 1)); echo "ok   - $1"; }
report_fail() { FAIL=$((FAIL + 1)); FAILURES+=("$1"); echo "FAIL - $1: $2"; }

# assert_case runs spec-gate.sh with the given SPEX_BIN and spec dir, then
# checks the exit code and (optionally) that stdout+stderr contains a marker
# string.
assert_case() {
    local name="$1" spex_bin="$2" spec_dir="$3" want_exit="$4" want_marker="${5:-}"
    local out rc
    out=$(SPEX_BIN="$spex_bin" bash "$SPEC_GATE" "$spec_dir" 2>&1)
    rc=$?
    if [[ "$rc" != "$want_exit" ]]; then
        report_fail "$name" "want exit $want_exit, got $rc (output: $out)"
        return
    fi
    if [[ -n "$want_marker" ]] && [[ "$out" != *"$want_marker"* ]]; then
        report_fail "$name" "output missing marker $(printf '%q' "$want_marker"): $out"
        return
    fi
    report_ok "$name"
}

# ---- Build the real spex binary once ---------------------------------------
#
# Built under .gotmp/ (repo-local, gitignored) rather than mktemp -d's
# default /tmp: some sandboxes mount /tmp noexec, which would make every
# real-binary case below fail to exec the very binary it just built.

mkdir -p "$REPO_ROOT/.gotmp"
BUILD_DIR=$(mktemp -d "$REPO_ROOT/.gotmp/spec-gate-test.XXXXXX")
trap 'rm -rf "$BUILD_DIR"' EXIT
SPEX_BIN="$BUILD_DIR/spex"
if ! CGO_ENABLED=0 go build -o "$SPEX_BIN" "$REPO_ROOT/cmd/spex" 2>"$BUILD_DIR/build.log"; then
    echo "failed to build spex for real-binary cases:" >&2
    cat "$BUILD_DIR/build.log" >&2
    exit 1
fi

# ---- Real-binary fixtures ---------------------------------------------------
# Each fixture starts from a copy of this repo's own spec/, which `spex
# validate`/`spex diff` already report clean against — self-hosting means we
# never need a synthetic spec graph to exercise the gate's real contract.

new_fixture() {
    local dir; dir=$(mktemp -d)
    cp -r "$REPO_ROOT/spec" "$dir/spec"
    echo "$dir/spec"
}

fx_clean=$(new_fixture)

fx_completeness=$(new_fixture)
# Modify a requirement's description without touching its implementing
# component's content leaf: validate stays clean (the requirement IS
# implemented), but diff's completeness check flags the drift — this is
# exactly the "silently broke the graph it claims to describe" scenario
# arch_spec_gate.md names as the reason the pass exists.
jq '.requirements[0].description += " (test edit)"' \
    "$fx_completeness/delivery/module.json" > "$fx_completeness/delivery/module.json.tmp"
mv "$fx_completeness/delivery/module.json.tmp" "$fx_completeness/delivery/module.json"

fx_diff_build_failure=$(new_fixture)
# A corrupted snapshot fails only the completeness pass: validate never reads
# it, so the structural pass stays green and the gate's diff branch is
# reached and must report the failure distinctly (not as a JSON parse error).
echo '{not valid json' > "$fx_diff_build_failure/.snapshot.json"

fx_structural_failure=$(new_fixture)
# A well-formed but uncovered requirement fails only the structural pass
# (requirement_coverage), well before the gate ever runs diff.
jq '.components[0].implements = []' \
    "$fx_structural_failure/delivery/module.json" > "$fx_structural_failure/delivery/module.json.tmp"
mv "$fx_structural_failure/delivery/module.json.tmp" "$fx_structural_failure/delivery/module.json"

trap 'rm -rf "$BUILD_DIR" "$fx_clean" "$fx_completeness" "$fx_diff_build_failure" "$fx_structural_failure"' EXIT

assert_case "clean tree: gate is green" \
    "$SPEX_BIN" "$fx_clean" 0 "completeness pass clean"

assert_case "completeness error: gate is red with the error surfaced" \
    "$SPEX_BIN" "$fx_completeness" 2 "incomplete_change"

assert_case "diff build failure (broken snapshot): reported distinctly, not as a parse error" \
    "$SPEX_BIN" "$fx_diff_build_failure" 1 "did not build the tree"

assert_case "structural pass failure: gate is red before completeness ever runs" \
    "$SPEX_BIN" "$fx_structural_failure" 1 "structural pass failed"

# ---- Mock-binary edge cases -------------------------------------------------
#
# Gated on /usr/bin/env being usable, since mock_spex.sh's shebang needs it —
# some minimal sandboxes have no /usr/bin at all (mirrors the `br`-on-PATH
# gate in apply-br_test.sh's integration mode). Skipped, not failed, when
# absent.

if [[ -x /usr/bin/env ]] && "$MOCK_SPEX" validate >/dev/null 2>&1; then
    # The gate asserts on the JSON verdict, not on exit status alone: `valid:
    # true` with a non-zero finding count must still fail the job, even though
    # the real validator can never emit this shape today (Valid is always
    # len(errors)==0 — see validator/error_reporter.go).
    MOCK_VALIDATE_STDOUT='{"valid":true,"error_count":3,"warning_count":0,"errors":[{"check":"schema","severity":"error","path":"x","message":"inconsistent verdict"}]}' \
    MOCK_VALIDATE_EXIT=0 \
        assert_case "structural pass asserts on the JSON verdict, not exit status" \
            "$MOCK_SPEX" "unused" 1 "structural pass failed"

    # warning_count is the other finding count in the verdict: a `valid: true`,
    # `error_count: 0` report with a non-zero warning_count must still fail the
    # job, even though the real validator can never emit this shape today
    # (WarningCount is always 0 — see validator/error_reporter.go).
    MOCK_VALIDATE_STDOUT='{"valid":true,"error_count":0,"warning_count":3,"errors":[]}' \
    MOCK_VALIDATE_EXIT=0 \
        assert_case "structural pass asserts on warning_count too" \
            "$MOCK_SPEX" "unused" 1 "structural pass failed"

    # Diff notes are disclosures, never violations: they must not gate the exit
    # code even though they ride along with an otherwise-clean verdict.
    MOCK_VALIDATE_STDOUT='{"valid":true,"error_count":0,"warning_count":0,"errors":[]}' \
    MOCK_VALIDATE_EXIT=0 \
    MOCK_DIFF_STDOUT='{"changes":[],"errors":[],"notes":[{"type":"unverifiable_module","message":"cannot verify removed module beta"}],"summary":{"total":0,"by_type":{},"by_impact":{}}}' \
    MOCK_DIFF_EXIT=0 \
        assert_case "diff notes are disclosures and do not gate the verdict" \
            "$MOCK_SPEX" "unused" 0 "cannot verify removed module beta"

    # Notes are disclosures on both passes, not just the completeness one: a
    # `derivation: pending` requirement surfaces as the structural pass's own
    # `pending_derivation` note, printed the same way diff notes are, and the
    # job stays green since a structural pass whose only finding is a note is
    # still zero findings — see arch_spec_gate.md's "Notes are disclosures"
    # paragraph.
    MOCK_VALIDATE_STDOUT='{"valid":true,"error_count":0,"warning_count":0,"errors":[],"notes":[{"type":"pending_derivation","message":"project requirement a65bbd37c7ec declares derivation pending","related":["a65bbd37c7ec"]}]}' \
    MOCK_VALIDATE_EXIT=0 \
    MOCK_DIFF_STDOUT='{"changes":[],"errors":[],"summary":{"total":0,"by_type":{},"by_impact":{}}}' \
    MOCK_DIFF_EXIT=0 \
        assert_case "structural pass notes are disclosures and do not gate the verdict" \
            "$MOCK_SPEX" "unused" 0 "project requirement a65bbd37c7ec declares derivation pending"
else
    echo "skip - mock-binary cases: /usr/bin/env unusable in this sandbox"
fi

# ---- Summary -----------------------------------------------------------------

echo
echo "$PASS passed, $FAIL failed"
if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
exit 0
