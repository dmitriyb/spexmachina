#!/usr/bin/env bash
# test-cc-syntactic.sh — verify the six syntactic CC hooks (no skill
# context required). Each hook is invoked with a stdin payload that
# Claude Code would normally provide, and we assert:
#   - matching input: emits a deny envelope with the expected rule slug
#   - non-matching input: exits 0 with no output

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$repo_root/scripts/hooks"

# fail "<test-name>" "<reason>"
fail() {
  echo "FAIL ${1}: ${2}" >&2
  exit 1
}

# assert_deny <hook-script> <stdin-json> <expected-rule-slug> <test-name>
assert_deny() {
  local hook="$1" stdin="$2" expected_rule="$3" name="$4"
  local out exit_code
  out="$(printf '%s' "$stdin" | "$hook" 2>/dev/null)"
  exit_code=$?
  if (( exit_code != 0 )); then
    fail "$name" "hook exited $exit_code (expected 0 with deny envelope)"
  fi
  if [[ -z "$out" ]]; then
    fail "$name" "hook produced no output (expected deny envelope)"
  fi
  local reason rule
  reason="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason // empty' 2>/dev/null)"
  if [[ -z "$reason" ]]; then
    fail "$name" "output is not a wire envelope: $out"
  fi
  rule="$(echo "$reason" | jq -r '.rule // empty')"
  if [[ "$rule" != "$expected_rule" ]]; then
    fail "$name" "wrong rule slug: got '$rule', want '$expected_rule'"
  fi
}

# assert_allow <hook-script> <stdin-json> <test-name>
assert_allow() {
  local hook="$1" stdin="$2" name="$3"
  local out
  out="$(printf '%s' "$stdin" | "$hook" 2>/dev/null)"
  if [[ -n "$out" ]]; then
    fail "$name" "hook should have allowed but produced output: $out"
  fi
}

# --- R5b: signing bypass -----------------------------------------------------
assert_deny "$hooks_dir/block-signing-bypass.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git commit --no-gpg-sign -m foo"}}' \
  "signing-flag-denied" "R5b-no-gpg-sign"
assert_deny "$hooks_dir/block-signing-bypass.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git -c commit.gpgsign=false commit -m foo"}}' \
  "signing-flag-denied" "R5b-c-flag"
assert_allow "$hooks_dir/block-signing-bypass.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git commit -S -m foo"}}' \
  "R5b-allows-signed"
assert_allow "$hooks_dir/block-signing-bypass.sh" \
  '{"tool_name":"Read","tool_input":{"file_path":"/etc/hosts"}}' \
  "R5b-allows-non-bash"
# Regression: --no-gpg-sign mentioned inside a heredoc body (e.g. PR
# description text or commit message documenting R5b) must not trip.
assert_allow "$hooks_dir/block-signing-bypass.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"gh pr edit 170 --body \"$(cat <<EOF\nDescribes the --no-gpg-sign rule and commit.gpgsign=false bypass.\nEOF\n)\""}}' \
  "R5b-allows-flag-mentioned-in-heredoc"

# --- R7+R8: beads.db direct read --------------------------------------------
assert_deny "$hooks_dir/block-beads-db-read.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"sqlite3 .beads/beads.db .schema"}}' \
  "beads-db-direct-read" "R7-sqlite3"
assert_deny "$hooks_dir/block-beads-db-read.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"cat .beads/beads.db | head"}}' \
  "beads-db-direct-read" "R7-cat"
assert_deny "$hooks_dir/block-beads-db-read.sh" \
  '{"tool_name":"Read","tool_input":{"file_path":"/workspace/spexmachina/.beads/beads.db"}}' \
  "beads-db-direct-read" "R7-read-tool"
assert_allow "$hooks_dir/block-beads-db-read.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"br show 0lk.18 --json"}}' \
  "R7-allows-br"
assert_allow "$hooks_dir/block-beads-db-read.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"jq -s . .beads/issues.jsonl"}}' \
  "R7-allows-jsonl"

# --- R11: interactive git ----------------------------------------------------
assert_deny "$hooks_dir/block-interactive-git.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git rebase -i HEAD~3"}}' \
  "interactive-git-not-supported" "R11-rebase-i"
assert_deny "$hooks_dir/block-interactive-git.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git add -p"}}' \
  "interactive-git-not-supported" "R11-add-p"
assert_allow "$hooks_dir/block-interactive-git.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git rebase --exec '\''make test'\'' HEAD~3"}}' \
  "R11-allows-noninteractive-rebase"
assert_allow "$hooks_dir/block-interactive-git.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git add ."}}' \
  "R11-allows-add-all"

# --- R13: editing on main ---------------------------------------------------
# We can't change HEAD inside the test (we're in the real repo, on
# rfc/enforcement-guardrails). Validate the allow path only — the
# block path is exercised in the git-hooks fixture via the throwaway
# repo's pre-commit run on main.
assert_allow "$hooks_dir/check-not-on-main.sh" \
  '{"tool_name":"Edit","tool_input":{"file_path":"/workspace/spexmachina/CLAUDE.md"}}' \
  "R13-allows-edit-on-feature-branch"
assert_allow "$hooks_dir/check-not-on-main.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
  "R13-allows-status-bash"

# --- R3: stale-origin -------------------------------------------------------
# The current repo has .git/FETCH_HEAD; if its mtime is within TTL,
# the hook allows. Force the stale path by setting TTL to 0.
assert_deny "$hooks_dir/check-fetched-recent.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git switch -c feature/new origin/main"}}' \
  "stale-origin" "R3-stale-ttl0" \
  2>/dev/null || true
# That last assert ran with default TTL; the file may be fresh. Run
# again with TTL=0 to force stale.
out="$(printf '%s' '{"tool_name":"Bash","tool_input":{"command":"git switch -c feature/new origin/main"}}' \
  | SPEX_FETCH_TTL=0 "$hooks_dir/check-fetched-recent.sh" 2>/dev/null)"
rule="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason' | jq -r '.rule // empty')"
if [[ "$rule" != "stale-origin" ]]; then
  fail "R3-ttl0" "want stale-origin, got '$rule'"
fi
# SPEX_OFFLINE=1 must bypass.
out="$(printf '%s' '{"tool_name":"Bash","tool_input":{"command":"git switch -c feature/new origin/main"}}' \
  | SPEX_FETCH_TTL=0 SPEX_OFFLINE=1 "$hooks_dir/check-fetched-recent.sh" 2>/dev/null)"
if [[ -n "$out" ]]; then
  fail "R3-offline-bypass" "SPEX_OFFLINE=1 should allow, got: $out"
fi
# Non-branch commands always allowed.
assert_allow "$hooks_dir/check-fetched-recent.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
  "R3-allows-non-branch"

# --- R4: branch not from origin/main ----------------------------------------
assert_deny "$hooks_dir/check-branch-from-origin-main.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git checkout -b feature/x"}}' \
  "branch-not-from-origin-main" "R4-no-start-point"
assert_deny "$hooks_dir/check-branch-from-origin-main.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git switch -c feature/x HEAD"}}' \
  "branch-not-from-origin-main" "R4-from-HEAD"
assert_allow "$hooks_dir/check-branch-from-origin-main.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git switch -c feature/x origin/main"}}' \
  "R4-from-origin-main"
assert_allow "$hooks_dir/check-branch-from-origin-main.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git checkout main"}}' \
  "R4-allows-non-create"

echo "ok"
