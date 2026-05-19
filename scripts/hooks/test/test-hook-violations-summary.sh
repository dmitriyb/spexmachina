#!/usr/bin/env bash
# test-hook-violations-summary.sh — verify the summary script reads
# the log, filters by rule and time, and prints a usable rollup.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
summary="$repo_root/scripts/hook-violations-summary"
log_file="$repo_root/.claude/hook-violations.log"

# Snapshot existing log and replace with a known fixture for the test.
backup=""
if [[ -f "$log_file" ]]; then
  backup="$(mktemp)"
  cp "$log_file" "$backup"
fi
restore() {
  if [[ -n "$backup" ]]; then mv "$backup" "$log_file"; else rm -f "$log_file"; fi
}
trap restore EXIT

mkdir -p "$(dirname "$log_file")"
cat > "$log_file" <<EOF
{"ts":"2026-05-19T10:00:00Z","protocol":"spex-halt/v1","rule":"no-commits-to-main","command":"git commit","cwd":"/x","head":"main","skill":"fix"}
{"ts":"2026-05-19T11:00:00Z","protocol":"spex-halt/v1","rule":"no-commits-to-main","command":"git commit","cwd":"/x","head":"main","skill":"implement"}
{"ts":"2026-05-19T12:00:00Z","protocol":"spex-halt/v1","rule":"br-close-outside-review","command":"br close x","cwd":"/x","head":"feature/y","skill":"implement"}
{"ts":"2026-05-19T13:00:00Z","protocol":"spex-halt/v1","rule":"no-commits-to-main","command":"git commit","cwd":"/x","head":"main","skill":null}
EOF

fail() { echo "FAIL ${1}: ${2}" >&2; exit 1; }

# Test 1: default rollup mentions both rules with counts.
out="$("$summary")"
echo "$out" | grep -qE "3[[:space:]]+no-commits-to-main" \
  || fail "T1-rollup-rule-count" "expected '3  no-commits-to-main', got: $out"
echo "$out" | grep -qE "1[[:space:]]+br-close-outside-review" \
  || fail "T1-rollup-br-count" "expected '1  br-close-outside-review', got: $out"

# Test 2: --rule filter shows all entries for that rule.
out="$("$summary" --rule no-commits-to-main)"
lines="$(echo "$out" | grep -c "no-commits-to-main\|2026-05-19" || true)"
# Should mention "All 3 entries" header AND three timestamped lines.
echo "$out" | grep -q "All 3 entries" \
  || fail "T2-rule-header" "expected 'All 3 entries' header, got: $out"

# Test 3: --since cutoff keeps newer entries only.
out="$("$summary" --since "2026-05-19T11:30:00Z")"
# Should include 12:00, 13:00 (br-close + last no-commit). 2 entries total.
echo "$out" | grep -qE "1[[:space:]]+br-close-outside-review" \
  || fail "T3-since-includes-newer" "expected br-close, got: $out"
echo "$out" | grep -qE "total 2 entries" \
  || fail "T3-since-total" "expected 'total 2 entries', got: $out"

# Test 4: --recent N shows last N entries.
out="$("$summary" --recent 2)"
echo "$out" | grep -q "br-close-outside-review" \
  || fail "T4-recent" "expected br-close in recent 2, got: $out"

# Test 5: empty / missing log prints message and exits 0.
rm "$log_file"
out="$("$summary" 2>&1)"
echo "$out" | grep -q "no log at" \
  || fail "T5-no-log" "expected 'no log at' message, got: $out"

echo "ok"
