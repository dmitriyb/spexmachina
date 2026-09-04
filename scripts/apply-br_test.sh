#!/usr/bin/env bash
#
# apply-br_test.sh — bash test harness for scripts/apply-br.sh and
# scripts/export-br.sh.
#
# Two modes, both gated on jq being on PATH:
#
#  1. Mock-mode tests against scripts/testdata/mock_br.sh. Run
#     unconditionally (no real br needed). Fixtures under
#     scripts/testdata/{idempotency,substitution,export}.
#
#  2. Integration tests against a real br sandbox. Gated on `br` being on
#     PATH; skipped otherwise. Fixtures under scripts/testdata/integration.
#
# Each apply-half fixture directory contains:
#   changeset.json          — input to apply-br.sh.
#   state_before.json       — (mock mode) initial BR_MOCK_STATE.
#   expected_receipts.json  — receipts shape the harness diffs against.
#   expected_state.json     — (mock mode, optional) final BR_MOCK_STATE shape
#                             after the run. Only the .issues field is compared.
#   seed.sh                 — (integration mode) seeds the real br sandbox.
#   verify.sh               — (integration mode, optional) extra assertions.
#   runs.txt                — (mock mode, optional) integer N: re-run the
#                             adapter N times against the same state and check
#                             expected_receipts_runN.json.
#
# Each export-half fixture directory (scripts/testdata/export, mock mode
# only) contains:
#   state_before.json  — initial BR_MOCK_STATE.
#   expected_tasks.json — the task-state document export-br.sh must emit.
#
# The integration export fixture (scripts/testdata/integration/export) has
# no changeset.json; the harness runs export-br.sh instead of apply-br.sh
# and hands its output to verify.sh as tasks.json.
#
# Exit code: 0 if every fixture passes; 1 on first failure.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$ROOT/apply-br.sh"
EXPORT_SCRIPT="$ROOT/export-br.sh"
MOCK_BR="$ROOT/testdata/mock_br.sh"

# Exported so an integration fixture's seed.sh can drive the real adapter
# itself — needed for scenarios (re-run idempotency) whose setup IS a prior
# adapter run, not raw br state.
export SPEX_ADAPTER_SCRIPT="$SCRIPT"

if ! command -v jq >/dev/null 2>&1; then
    echo "jq not on PATH — cannot run adapter tests" >&2
    exit 1
fi

PASS=0
FAIL=0
FAILURES=()

# Pretty-diff two JSON blobs sorted; returns 0 if equal. Uses git diff
# --no-index since coreutils `diff` is not always available; falls back to
# string equality if git is missing too.
_jdiff() {
    local a b
    a=$(jq -S . <<< "$1")
    b=$(jq -S . <<< "$2")
    if [[ "$a" == "$b" ]]; then
        return 0
    fi
    if command -v git >/dev/null 2>&1; then
        local af bf
        af=$(mktemp)
        bf=$(mktemp)
        printf '%s\n' "$a" > "$af"
        printf '%s\n' "$b" > "$bf"
        git --no-pager diff --no-index --no-color "$af" "$bf" || true
        rm -f "$af" "$bf"
    else
        echo "actual: $a"
        echo "expected: $b"
    fi
    return 1
}

# Same as _jdiff but for plain text (no jq sort).
_tdiff() {
    local af="$1" bf="$2"
    if [[ "$(cat "$af")" == "$(cat "$bf")" ]]; then
        return 0
    fi
    if command -v git >/dev/null 2>&1; then
        git --no-pager diff --no-index --no-color "$af" "$bf" || true
    else
        echo "actual: $(cat "$af")"
        echo "expected: $(cat "$bf")"
    fi
    return 1
}

run_mock_case() {
    local case_dir="$1"
    local name; name=$(basename "$case_dir")
    local tmp; tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' RETURN

    cp "$case_dir/state_before.json" "$tmp/state.json"
    : > "$tmp/log.txt"

    # Optional multi-run loop.
    local runs=1
    if [[ -f "$case_dir/runs.txt" ]]; then
        runs=$(cat "$case_dir/runs.txt")
    fi

    local r
    for ((r=1; r<=runs; r++)); do
        local expected_file="$case_dir/expected_receipts.json"
        if [[ "$runs" -gt 1 ]]; then
            expected_file="$case_dir/expected_receipts_run${r}.json"
        fi
        if [[ ! -f "$expected_file" ]]; then
            echo "  [FAIL] $name: missing $expected_file"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (missing $expected_file)")
            return
        fi

        local actual
        actual=$(BR_BIN="$MOCK_BR" \
                 BR_MOCK_LOG="$tmp/log.txt" \
                 BR_MOCK_STATE="$tmp/state.json" \
                 "$SCRIPT" "$case_dir/changeset.json" 2>"$tmp/stderr.txt") || {
            echo "  [FAIL] $name (run $r): adapter exited non-zero"
            echo "    stderr:"
            sed 's/^/      /' "$tmp/stderr.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name run $r (exit non-zero)")
            return
        }

        if ! _jdiff "$actual" "$(cat "$expected_file")" >"$tmp/diff.txt" 2>&1; then
            echo "  [FAIL] $name (run $r): receipts mismatch"
            sed 's/^/    /' "$tmp/diff.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name run $r (receipts mismatch)")
            return
        fi
    done

    # Optional final-state check.
    if [[ -f "$case_dir/expected_state.json" ]]; then
        local actual_issues expected_issues
        actual_issues=$(jq '.issues | sort_by(.id)' "$tmp/state.json")
        expected_issues=$(jq '.issues | sort_by(.id)' "$case_dir/expected_state.json")
        if ! _jdiff "$actual_issues" "$expected_issues" >"$tmp/diff.txt" 2>&1; then
            echo "  [FAIL] $name: final state mismatch"
            sed 's/^/    /' "$tmp/diff.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (final state)")
            return
        fi
    fi

    # Optional invocation-log check.
    if [[ -f "$case_dir/expected_log.txt" ]]; then
        if ! _tdiff "$tmp/log.txt" "$case_dir/expected_log.txt" >"$tmp/diff.txt" 2>&1; then
            echo "  [FAIL] $name: invocation log mismatch"
            sed 's/^/    /' "$tmp/diff.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (invocation log)")
            return
        fi
    fi

    echo "  [ok]   $name"
    PASS=$((PASS+1))
}

run_export_mock_case() {
    local case_dir="$1"
    local name; name=$(basename "$case_dir")
    local tmp; tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' RETURN

    cp "$case_dir/state_before.json" "$tmp/state.json"

    local expected; expected=$(cat "$case_dir/expected_tasks.json")

    # Run twice over the unchanged sandbox and assert byte-identical
    # output — the export half reads the tracker, it never mutates it.
    local first second
    first=$(BR_BIN="$MOCK_BR" BR_MOCK_STATE="$tmp/state.json" "$EXPORT_SCRIPT" 2>"$tmp/stderr.txt") || {
        echo "  [FAIL] $name: export-br.sh exited non-zero"
        sed 's/^/    /' "$tmp/stderr.txt"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (exit non-zero)")
        return
    }
    second=$(BR_BIN="$MOCK_BR" BR_MOCK_STATE="$tmp/state.json" "$EXPORT_SCRIPT" 2>"$tmp/stderr.txt") || {
        echo "  [FAIL] $name: export-br.sh exited non-zero on second run"
        sed 's/^/    /' "$tmp/stderr.txt"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (exit non-zero, second run)")
        return
    }
    if [[ "$first" != "$second" ]]; then
        echo "  [FAIL] $name: two runs over an unchanged sandbox produced different output"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (not deterministic)")
        return
    fi

    if ! _jdiff "$first" "$expected" >"$tmp/diff.txt" 2>&1; then
        echo "  [FAIL] $name: tasks.json mismatch"
        sed 's/^/    /' "$tmp/diff.txt"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (tasks mismatch)")
        return
    fi

    echo "  [ok]   $name"
    PASS=$((PASS+1))
}

run_integration_case() {
    local case_dir="$1"
    local name; name=$(basename "$case_dir")
    local sandbox; sandbox=$(mktemp -d)
    trap 'rm -rf "$sandbox"' RETURN

    cp -r "$case_dir"/* "$sandbox/"
    (cd "$sandbox" && br init >/dev/null 2>&1)

    if [[ -f "$sandbox/seed.sh" ]]; then
        (cd "$sandbox" && bash seed.sh >/dev/null) || {
            echo "  [FAIL] $name: seed.sh failed"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (seed)")
            return
        }
    fi

    if [[ -f "$sandbox/changeset.json" ]]; then
        local actual
        actual=$(cd "$sandbox" && "$SCRIPT" changeset.json 2>stderr.txt) || {
            echo "  [FAIL] $name: adapter exited non-zero"
            sed 's/^/    /' "$sandbox/stderr.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (adapter exit)")
            return
        }

        # Integration task IDs are non-deterministic (br assigns them based
        # on the sandbox repo name). Normalize: any expected task_id of
        # "__ANY__" accepts whatever the run produced.
        local norm_actual norm_expected
        norm_actual=$(jq '.ops |= map(.task_id = (if .task_id == "" then "" else "__ANY__" end))' <<< "$actual")
        norm_expected=$(jq '.ops |= map(if .task_id == "__ANY__" then . else . end)' "$sandbox/expected_receipts.json")
        if ! _jdiff "$norm_actual" "$norm_expected" >"$sandbox/diff.txt" 2>&1; then
            echo "  [FAIL] $name: receipts mismatch"
            sed 's/^/    /' "$sandbox/diff.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (receipts)")
            return
        fi
    else
        # No changeset.json: an export-half fixture. Run export-br.sh with
        # the <tasks.json> argument form — the one place that form runs
        # under test — and hand the file it writes to verify.sh.
        (cd "$sandbox" && "$EXPORT_SCRIPT" tasks.json 2>stderr.txt) || {
            echo "  [FAIL] $name: export-br.sh exited non-zero"
            sed 's/^/    /' "$sandbox/stderr.txt"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (export exit)")
            return
        }
    fi

    if [[ -f "$sandbox/verify.sh" ]]; then
        if ! (cd "$sandbox" && bash verify.sh >verify.out 2>&1); then
            echo "  [FAIL] $name: verify.sh failed"
            sed 's/^/    /' "$sandbox/verify.out"
            FAIL=$((FAIL+1))
            FAILURES+=("$name (verify)")
            return
        fi
    fi

    echo "  [ok]   $name"
    PASS=$((PASS+1))
}

# ---- Run mock-mode suites --------------------------------------------------

for suite in idempotency substitution; do
    suite_dir="$ROOT/testdata/$suite"
    [[ -d "$suite_dir" ]] || continue
    echo "== $suite =="
    for case_dir in "$suite_dir"/*/; do
        run_mock_case "$case_dir"
    done
done

echo "== export =="
export_dir="$ROOT/testdata/export"
if [[ -d "$export_dir" ]]; then
    for case_dir in "$export_dir"/*/; do
        run_export_mock_case "$case_dir"
    done
fi

# ---- Pre-flight rejection tests --------------------------------------------
# These exercise the changeset.json v4 gate (version + required fields).
# The adapter must exit non-zero AND print a recognizable error to stderr
# WITHOUT writing receipts.

run_reject_case() {
    local name="$1" json="$2" needle="$3"
    local tmp; tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' RETURN

    echo "$json" > "$tmp/bad.json"
    : > "$tmp/log.txt"
    echo '{"issues":[],"create_ids":[]}' > "$tmp/state.json"
    if BR_BIN="$MOCK_BR" BR_MOCK_LOG="$tmp/log.txt" BR_MOCK_STATE="$tmp/state.json" \
            "$SCRIPT" "$tmp/bad.json" >"$tmp/out.txt" 2>"$tmp/err.txt"; then
        echo "  [FAIL] $name: adapter unexpectedly exited 0"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (exit 0)")
        return
    fi
    if ! grep -q -- "$needle" "$tmp/err.txt"; then
        echo "  [FAIL] $name: stderr missing expected fragment '$needle'"
        sed 's/^/    /' "$tmp/err.txt"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (no needle)")
        return
    fi
    echo "  [ok]   $name"
    PASS=$((PASS+1))
}

echo "== reject =="
run_reject_case version_mismatch \
    '{"version":2,"git_head":"x","proposal":"p","ops":[]}' \
    "unsupported changeset version: 2"
run_reject_case missing_git_head \
    '{"version":4,"proposal":"p","ops":[]}' \
    "missing required field: git_head"
run_reject_case missing_proposal \
    '{"version":4,"git_head":"x","ops":[]}' \
    "missing required field: proposal"
run_reject_case missing_ops \
    '{"version":4,"git_head":"x","proposal":"p"}' \
    "missing or malformed required field: ops"
run_reject_case invalid_json \
    'not json at all' \
    "changeset is not valid JSON"

# ---- Export listing-failure test -------------------------------------------
# export-br.sh must exit 1 on a listing failure and write no document,
# regardless of br's own exit status (flow_adapter.md: "A failure of the
# listing exits 1 and writes no document").

run_export_list_failure_case() {
    local name="export_list_failure"
    local tmp; tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' RETURN

    cat > "$tmp/fail_br.sh" <<'STUB'
#!/usr/bin/env bash
if [[ "${1:-}" == "--version" ]]; then echo "br 0.0.0-mock"; exit 0; fi
if [[ "${1:-}" == "list" ]]; then exit 2; fi
exit 0
STUB
    chmod +x "$tmp/fail_br.sh"

    BR_BIN="$tmp/fail_br.sh" "$EXPORT_SCRIPT" >"$tmp/out.txt" 2>"$tmp/err.txt"
    local rc=$?

    if [[ "$rc" -ne 1 ]]; then
        echo "  [FAIL] $name: exit code $rc, want 1 (br's own exit status leaked through)"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (exit $rc)")
        return
    fi
    if [[ -s "$tmp/out.txt" ]]; then
        echo "  [FAIL] $name: tasks document written on listing failure"
        FAIL=$((FAIL+1))
        FAILURES+=("$name (wrote output)")
        return
    fi
    echo "  [ok]   $name"
    PASS=$((PASS+1))
}

echo "== export reject =="
run_export_list_failure_case

# ---- Run integration suite (gated) -----------------------------------------

if command -v br >/dev/null 2>&1; then
    integ_dir="$ROOT/testdata/integration"
    if [[ -d "$integ_dir" ]]; then
        echo "== integration (real br) =="
        for case_dir in "$integ_dir"/*/; do
            run_integration_case "$case_dir"
        done
    fi
else
    echo "== integration: SKIPPED (br not on PATH) =="
fi

echo
echo "passed: $PASS  failed: $FAIL"
if [[ "$FAIL" -gt 0 ]]; then
    printf '  - %s\n' "${FAILURES[@]}"
    exit 1
fi
