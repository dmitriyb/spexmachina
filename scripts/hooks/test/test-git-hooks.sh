#!/usr/bin/env bash
# test-git-hooks.sh — verify pre-commit, post-commit, pre-push hooks
# fire correctly on a throwaway repo. The hooks self-locate their lib
# via their own script path, so pointing core.hooksPath at the real
# scripts/git-hooks dir is enough.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cd "$tmp"
git init -q -b main 2>/dev/null || { git init -q && git symbolic-ref HEAD refs/heads/main; }
git config user.email "test@local"
git config user.name  "Test"
git config commit.gpgsign false                     # local — disables signing
git config --local includeIf.condition.never "x"     # noop, just to ensure local config exists
git config core.hooksPath "$repo_root/scripts/git-hooks"

# Test 1: pre-commit blocks on main
echo "x" > a && git add a
output="$(git commit -m "should be blocked: main" 2>&1 || true)"
if ! echo "$output" | grep -q "no-commits-to-main"; then
  echo "FAIL test1: commit on main should have triggered no-commits-to-main" >&2
  echo "actual output: $output" >&2
  exit 1
fi

# Test 2: on a feature branch with gpgsign=false, pre-commit blocks unsigned
git checkout -q -b feature/test
output="$(git commit -m "should be blocked: unsigned" 2>&1 || true)"
if ! echo "$output" | grep -q "unsigned-commit-detected"; then
  echo "FAIL test2: unsigned commit should have triggered unsigned-commit-detected" >&2
  echo "actual output: $output" >&2
  exit 1
fi

# Test 3: with gpgsign=true (locally) the pre-commit hook itself
# returns 0 — we don't try the full commit (no signing key in fixture),
# we just invoke the hook directly.
git config commit.gpgsign true
if ! "$repo_root/scripts/git-hooks/pre-commit" >/dev/null 2>&1; then
  echo "FAIL test3: pre-commit blocked on feature/test with gpgsign=true" >&2
  exit 1
fi

# Test 4: pre-push blocks push to refs/heads/main
# Create a stub commit by temporarily disabling hooks AND signing so
# the test has a real sha to push (no signing key in this fixture).
git -c core.hooksPath=/dev/null -c commit.gpgsign=false \
    commit -q --allow-empty -m "stub" 2>/dev/null
sha="$(git rev-parse HEAD)"

push_input="refs/heads/feature/test $sha refs/heads/main 0000000000000000000000000000000000000000"
output="$(echo "$push_input" | "$repo_root/scripts/git-hooks/pre-push" 2>&1 || true)"
if ! echo "$output" | grep -q "no-direct-push-to-main"; then
  echo "FAIL test4: pre-push to refs/heads/main should have triggered no-direct-push-to-main" >&2
  echo "actual output: $output" >&2
  exit 1
fi

# Test 5: pre-push to a feature branch flags unsigned commits (the
# stub above is unsigned). Verify the rule slug is in the output.
push_input="refs/heads/feature/test $sha refs/heads/feature/test 0000000000000000000000000000000000000000"
output="$(echo "$push_input" | "$repo_root/scripts/git-hooks/pre-push" 2>&1 || true)"
if ! echo "$output" | grep -q "unsigned-commit-detected"; then
  echo "FAIL test5: pre-push of unsigned commits should have triggered unsigned-commit-detected" >&2
  echo "actual output: $output" >&2
  exit 1
fi

echo "ok"
