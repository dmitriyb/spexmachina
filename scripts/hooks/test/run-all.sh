#!/usr/bin/env bash
# run-all.sh — run every hook test fixture under scripts/hooks/test/
# and report pass/fail. Each fixture is a script `test-*.sh` that
# returns 0 on pass, non-zero on fail.
#
# Invoked by `scripts/verify-enforcement`.

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
test_dir="$repo_root/scripts/hooks/test"

pass=0
fail=0
failed_tests=()

for t in "$test_dir"/test-*.sh; do
  [[ -e "$t" ]] || continue
  name="$(basename "$t")"
  if bash "$t" >/dev/null 2>&1; then
    printf "  PASS  %s\n" "$name"
    pass=$((pass + 1))
  else
    printf "  FAIL  %s\n" "$name"
    failed_tests+=("$name")
    fail=$((fail + 1))
  fi
done

printf "\n%d passed, %d failed\n" "$pass" "$fail"
if (( fail > 0 )); then
  printf "\nFailed:\n"
  for t in "${failed_tests[@]}"; do printf "  %s\n" "$t"; done
  exit 1
fi
exit 0
