#!/usr/bin/env bash
#
# ci-pipeline_test.sh — acceptance coverage for CIPipeline and SpecGate
# together, per spec/delivery/test_ci.md's "Cases — trigger tiers" and the
# paths-filter edge case. The spec gate's own verdict contract (clean pass,
# completeness error, the two ways a build can fail, the warning_count and
# notes-never-gate edge cases) is covered by scripts/spec-gate_test.sh — not
# duplicated here.
#
# test_ci.md is explicit that these are workflow-level checks exercised
# against a throwaway branch and pull request on the real repository, or
# act-style local runs where noted, because Go unit tests do not apply to
# workflow YAML. This harness cannot fabricate that: there is no `act` or
# `docker` in this sandbox, and no live GitHub repository to open a PR
# against. What it does instead, without pretending to be more than it is:
#
#  1. Structural assertions read the actual, current text of
#     .github/workflows/{pr,main,nightly-fuzz}.yml — job composition per
#     tier, the concurrency/cancellation declaration, and the paths filter —
#     rather than a hardcoded copy that could silently drift from the files
#     it is meant to guard.
#  2. The paths filter is exercised as a real filter: patterns are pulled
#     out of the YAML and matched against representative file paths, so
#     "README-only triggers nothing" and "spec-only still runs the gate"
#     are decided by the filter's actual content, not by inspection alone.
#  3. Fuzz target discovery is exercised for real: the exact `run:` block is
#     extracted from nightly-fuzz.yml (not retyped) and executed against
#     scratch Go modules — one with no Fuzz function, one with a newly added
#     one — proving discovery finds a new target with no workflow edit and
#     fails loudly when it finds none.
#
# Non-Responsibilities (need the real repository or `act`, named in
# test_ci.md itself, out of scope for this offline harness):
#   - A superseded run's in-progress jobs actually being cancelled by GitHub
#     on a second push (this harness only confirms the concurrency
#     declaration that makes that possible is present).
#   - Branch protection marking a job name required actually blocking a
#     red-check PR from merging — there is no branch-protection-as-code file
#     in this repository; it is a live GitHub setting.
#
# Exit code: 0 if every case passes; 1 on first failure.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"
WORKFLOWS="$REPO_ROOT/.github/workflows"
PR_YML="$WORKFLOWS/pr.yml"
MAIN_YML="$WORKFLOWS/main.yml"
FUZZ_YML="$WORKFLOWS/nightly-fuzz.yml"

for f in "$PR_YML" "$MAIN_YML" "$FUZZ_YML"; do
    [[ -f "$f" ]] || { echo "missing workflow file: $f" >&2; exit 1; }
done

PASS=0
FAIL=0

report_ok()   { PASS=$((PASS + 1)); echo "ok   - $1"; }
report_fail() { FAIL=$((FAIL + 1)); echo "FAIL - $1: $2"; }

# ---- extraction helpers ------------------------------------------------

# job_names <file> — top-level job keys under `jobs:`, in file order.
job_names() {
    awk '
        /^jobs:/ { injobs = 1; next }
        injobs && /^[^ ]/ { injobs = 0 }
        injobs && /^  [A-Za-z0-9_-]+:/ {
            line = $0
            sub(/^  /, "", line)
            sub(/:.*/, "", line)
            print line
        }
    ' "$1"
}

# path_filter_patterns <file> — the on.<event>.paths list, in file order.
path_filter_patterns() {
    awk '
        /paths:/ { inpaths = 1; next }
        inpaths && /^[ \t]*-/ {
            line = $0
            gsub(/^[ \t]*-[ \t]*"?/, "", line)
            gsub(/"?[ \t]*$/, "", line)
            print line
            next
        }
        inpaths { inpaths = 0 }
    ' "$1"
}

# run_block <file> — the literal text of the (single) `run: |` block scalar,
# dedented to column zero. Fails loudly (empty output) if the file's `run: |`
# shape ever changes, rather than silently extracting nothing.
run_block() {
    awk '
        match($0, /^([ \t]*)run: \|[ \t]*$/, m) {
            indent = length(m[1])
            inblock = 1
            next
        }
        inblock {
            if ($0 ~ /^[ \t]*$/) { print ""; next }
            match($0, /^[ \t]*/)
            cur_indent = RLENGTH
            if (cur_indent <= indent) { inblock = 0; exit }
            print substr($0, indent + 3)
        }
    ' "$1"
}

# assert_eq <name> <want> <got>
assert_eq() {
    local name="$1" want="$2" got="$3"
    if [[ "$want" == "$got" ]]; then
        report_ok "$name"
    else
        report_fail "$name" "want $(printf '%q' "$want"), got $(printf '%q' "$got")"
    fi
}

# assert_contains <name> <haystack> <needle>
assert_contains() {
    local name="$1" haystack="$2" needle="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
        report_ok "$name"
    else
        report_fail "$name" "expected to find $(printf '%q' "$needle")"
    fi
}

# assert_not_contains <name> <haystack> <needle>
assert_not_contains() {
    local name="$1" haystack="$2" needle="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        report_ok "$name"
    else
        report_fail "$name" "expected NOT to find $(printf '%q' "$needle")"
    fi
}

# ---- job composition per tier -------------------------------------------
#
# arch_ci.md: PR tier is build/vet/test/spec-gate, no race. Main tier is the
# same plus race, paid once post-merge rather than on every PR push.

pr_jobs=$(job_names "$PR_YML" | LC_ALL=C sort | tr '\n' ',')
main_jobs=$(job_names "$MAIN_YML" | LC_ALL=C sort | tr '\n' ',')

assert_eq "pr.yml runs build, vet, test, spec-gate — no race" \
    "build,spec-gate,test,vet," "$pr_jobs"

assert_eq "main.yml runs the same four jobs plus race" \
    "build,race,spec-gate,test,vet," "$main_jobs"

# ---- triggers -------------------------------------------------------------

pr_yml_text=$(cat "$PR_YML")
main_yml_text=$(cat "$MAIN_YML")
fuzz_yml_text=$(cat "$FUZZ_YML")

assert_contains "pr.yml triggers on pull_request" "$pr_yml_text" "pull_request:"
assert_contains "main.yml triggers on push to main" "$main_yml_text" $'branches:\n      - main'
assert_contains "nightly-fuzz.yml has a schedule trigger" "$fuzz_yml_text" "schedule:"
assert_contains "nightly-fuzz.yml also allows manual dispatch" "$fuzz_yml_text" "workflow_dispatch:"

# ---- concurrency: cancel a superseded run rather than queue it ----------
#
# This is the necessary declaration, not a live demonstration — GitHub
# actually cancelling a superseded run needs a real second push, which is
# out of reach here (see header).

for f in "$PR_YML" "$MAIN_YML" "$FUZZ_YML"; do
    name=$(basename "$f")
    text=$(cat "$f")
    assert_contains "$name declares a concurrency group" "$text" 'group: ${{ github.workflow }}-${{ github.ref }}'
    assert_contains "$name cancels in-progress runs of that group" "$text" "cancel-in-progress: true"
done

# ---- paths filter: exercised as a real filter, not just inspected --------
#
# The filter must never exclude spec/ (the gate exists precisely to judge
# spec changes) and must exclude changes that touch neither Go nor spec
# (e.g. a README-only branch). Patterns are read from the files themselves;
# an unrecognized pattern shape fails loudly rather than matching nothing.

path_matches() {
    local pattern="$1" path="$2"
    case "$pattern" in
        '**/*.go') [[ "$path" == *.go ]] ;;
        'spec/**') [[ "$path" == spec/* ]] ;;
        go.mod|go.sum) [[ "$path" == "$pattern" ]] ;;
        *) echo "path_matches: unrecognized pattern shape $(printf '%q' "$pattern") — extend the matcher" >&2; return 2 ;;
    esac
}

any_path_matches() {
    local file="$1" path="$2" pattern matched=1
    while IFS= read -r pattern; do
        [[ -n "$pattern" ]] || continue
        if path_matches "$pattern" "$path"; then
            matched=0
        fi
    done < <(path_filter_patterns "$file")
    return $matched
}

for wf in "$PR_YML" "$MAIN_YML"; do
    name=$(basename "$wf")
    patterns=$(path_filter_patterns "$wf" | LC_ALL=C sort | tr '\n' ',')
    assert_eq "$name paths filter is exactly the Go+spec set" \
        '**/*.go,go.mod,go.sum,spec/**,' "$patterns"

    if any_path_matches "$wf" "README.md"; then
        report_fail "$name: README-only change" "matched the paths filter but must not"
    else
        report_ok "$name: README-only change does not match the paths filter"
    fi

    if any_path_matches "$wf" "spec/delivery/module.json"; then
        report_ok "$name: spec-only change matches the paths filter"
    else
        report_fail "$name: spec-only change" "did not match the paths filter but must"
    fi

    if any_path_matches "$wf" "cmd/spex/main.go"; then
        report_ok "$name: a Go change matches the paths filter"
    else
        report_fail "$name: a Go change" "did not match the paths filter but must"
    fi

    if any_path_matches "$wf" "go.mod"; then
        report_ok "$name: go.mod matches the paths filter"
    else
        report_fail "$name: go.mod" "did not match the paths filter but must"
    fi
done

# ---- spec gate seat: CI builds the binary from the PR's own tree ---------

assert_contains "pr.yml's spec-gate job builds spex from the checked-out tree" \
    "$pr_yml_text" "go build -o \"\$RUNNER_TEMP/spex\" ./cmd/spex"
assert_contains "pr.yml's spec-gate job runs scripts/spec-gate.sh" \
    "$pr_yml_text" "./scripts/spec-gate.sh"
assert_contains "main.yml's spec-gate job builds spex from the checked-out tree" \
    "$main_yml_text" "go build -o \"\$RUNNER_TEMP/spex\" ./cmd/spex"
assert_contains "main.yml's spec-gate job runs scripts/spec-gate.sh" \
    "$main_yml_text" "./scripts/spec-gate.sh"

# ---- race: paid once, post-merge only ------------------------------------

assert_not_contains "pr.yml's build/vet/test steps never pass -race" "$pr_yml_text" "-race"
assert_contains "main.yml's race job runs go test -race" "$main_yml_text" "go test -race ./..."

# ---- fuzz target discovery: exercised for real ---------------------------
#
# The exact `run:` block is extracted from nightly-fuzz.yml — not retyped —
# and run against scratch modules under a repo-local temp dir. /tmp is
# noexec in some sandboxes (see spec-gate_test.sh), and `go test` must
# actually execute the compiled test binary to list or run a fuzz target, so
# both GOTMPDIR-sensitive TMPDIR and the module dir itself live under
# .gotmp/.

fuzz_script=$(run_block "$FUZZ_YML")
if [[ -z "$fuzz_script" ]]; then
    report_fail "nightly-fuzz.yml discovery" "could not extract the run: | block — shape changed?"
else
    assert_not_contains "nightly-fuzz.yml discovers targets rather than listing them by name" \
        "$fuzz_script" "FuzzThing"

    mkdir -p "$REPO_ROOT/.gotmp"
    export TMPDIR="$REPO_ROOT/.gotmp"

    empty_mod=$(mktemp -d "$REPO_ROOT/.gotmp/fuzz-empty.XXXXXX")
    trap 'rm -rf "$empty_mod"' RETURN 2>/dev/null || true
    (
        cd "$empty_mod"
        go mod init fuzzempty >/dev/null 2>&1
        cat > main.go <<'EOF'
package main

func main() {}
EOF
        cat > main_test.go <<'EOF'
package main

import "testing"

func TestNothing(t *testing.T) {}
EOF
    )
    empty_out=$(cd "$empty_mod" && FUZZTIME=1x bash -c "$fuzz_script" 2>&1)
    empty_rc=$?
    rm -rf "$empty_mod"

    if [[ "$empty_rc" == "1" ]]; then
        report_ok "discovery over a tree with no Fuzz functions exits 1"
    else
        report_fail "discovery over a tree with no Fuzz functions" "want exit 1, got $empty_rc"
    fi
    assert_contains "empty discovery fails loudly, naming the tree" "$empty_out" "no fuzz targets discovered"

    found_mod=$(mktemp -d "$REPO_ROOT/.gotmp/fuzz-found.XXXXXX")
    (
        cd "$found_mod"
        go mod init fuzzfound >/dev/null 2>&1
        cat > main.go <<'EOF'
package main

func main() {}
EOF
        cat > fuzz_test.go <<'EOF'
package main

import "testing"

func FuzzThing(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {})
}
EOF
    )
    found_out=$(cd "$found_mod" && FUZZTIME=1x bash -c "$fuzz_script" 2>&1)
    found_rc=$?
    rm -rf "$found_mod"

    if [[ "$found_rc" == "0" ]]; then
        report_ok "a newly added Fuzz function is picked up with no workflow edit"
    else
        report_fail "a newly added Fuzz function" "want exit 0, got $found_rc: $found_out"
    fi
    assert_contains "the discovered target actually ran" "$found_out" "FuzzThing"
    assert_contains "discovery reports what it found" "$found_out" "discovered and ran 1 fuzz target(s)"
fi

# ---- summary ----------------------------------------------------------------

echo
echo "$PASS passed, $FAIL failed"
if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
exit 0
