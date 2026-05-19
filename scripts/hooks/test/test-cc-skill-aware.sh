#!/usr/bin/env bash
# test-cc-skill-aware.sh — verify R6, R9, R10, R12 hooks respect the
# skill-context marker (and the SPEX_REBASELINE override for R12).
#
# Each test sets the marker to a known state, invokes the hook with a
# crafted stdin, and asserts deny/allow.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$repo_root/scripts/hooks"
marker="$repo_root/.claude/skill-context.json"

# Save and restore any existing marker so a live session isn't disrupted.
backup=""
if [[ -f "$marker" ]]; then
  backup="$(mktemp)"
  cp "$marker" "$backup"
fi
restore() {
  if [[ -n "$backup" ]]; then mv "$backup" "$marker"; else rm -f "$marker"; fi
}
trap restore EXIT

set_skill() {
  printf '{"skill":"%s","started_at":"%s","pid":%d}\n' \
    "$1" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > "$marker"
}

clear_skill() { rm -f "$marker"; }

fail() { echo "FAIL ${1}: ${2}" >&2; exit 1; }

assert_deny() {
  local hook="$1" stdin="$2" expected_rule="$3" name="$4"
  local out
  out="$(printf '%s' "$stdin" | "$hook" 2>/dev/null)"
  [[ -z "$out" ]] && fail "$name" "expected deny envelope, got nothing"
  local rule
  rule="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason' \
          | jq -r '.rule // empty')"
  [[ "$rule" == "$expected_rule" ]] || fail "$name" "want rule '$expected_rule', got '$rule'"
}

assert_allow() {
  local hook="$1" stdin="$2" name="$3"
  local out
  out="$(printf '%s' "$stdin" | "$hook" 2>/dev/null)"
  [[ -z "$out" ]] || fail "$name" "expected allow, got: $out"
}

# ============================================================================
# R6: br close
# ============================================================================
br_close='{"tool_name":"Bash","tool_input":{"command":"br close spexmachina-1dj1 --reason foo"}}'

# Active skill = review → allow
set_skill "review"
assert_allow "$hooks_dir/check-br-close-skill.sh" "$br_close" "R6-skill-review-allows"

# Active skill = implement → deny
set_skill "implement"
assert_deny "$hooks_dir/check-br-close-skill.sh" "$br_close" \
  "br-close-outside-review" "R6-skill-implement-denies"

# No skill marker → deny (fail-closed)
clear_skill
assert_deny "$hooks_dir/check-br-close-skill.sh" "$br_close" \
  "br-close-outside-review" "R6-no-skill-denies"

# Non-Bash tool → allow
assert_allow "$hooks_dir/check-br-close-skill.sh" \
  '{"tool_name":"Read","tool_input":{"file_path":"x"}}' \
  "R6-non-bash-allows"

# Bash but not br close → allow
assert_allow "$hooks_dir/check-br-close-skill.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"br show spexmachina-1dj1"}}' \
  "R6-br-show-allows"

# br close mentioned inside heredoc body → allow (doc string, not command)
heredoc='{"tool_name":"Bash","tool_input":{"command":"git commit -m \"$(cat <<EOF\nbr close ...\nEOF\n)\""}}'
set_skill "fix"
assert_allow "$hooks_dir/check-br-close-skill.sh" "$heredoc" \
  "R6-br-close-in-heredoc-allows"

# ============================================================================
# R9 + R10: git commit by skill
# ============================================================================
git_commit='{"tool_name":"Bash","tool_input":{"command":"git commit -m foo"}}'

# skill=spec → deny
set_skill "spec"
assert_deny "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" \
  "skill-must-not-commit" "R9-spec-denies"

# skill=converge → deny
set_skill "converge"
assert_deny "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" \
  "skill-must-not-commit" "R9-converge-denies"

# skill=propose → deny (no commits from /propose either)
set_skill "propose"
assert_deny "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" \
  "skill-must-not-commit" "R9-propose-denies"

# skill=fix → allow (R10)
set_skill "fix"
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" "R10-fix-allows"

# skill=review → allow (R10)
set_skill "review"
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" "R10-review-allows"

# skill=implement → allow (R10)
set_skill "implement"
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" "R10-implement-allows"

# skill=cleanup → allow (R10)
set_skill "cleanup"
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" "R10-cleanup-allows"

# No marker → allow (fail-open per R9 asymmetry)
clear_skill
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" "$git_commit" \
  "R9-no-skill-allows-fail-open"

# Non-Bash tool → allow
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" \
  '{"tool_name":"Edit","tool_input":{"file_path":"x"}}' \
  "R9-non-bash-allows"

# Bash but not git commit → allow
assert_allow "$hooks_dir/check-skill-commit-allowed.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
  "R9-not-commit-allows"

# git -c commit.gpgsign=true commit → still matches as a commit invocation
set_skill "spec"
assert_deny "$hooks_dir/check-skill-commit-allowed.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git -c commit.gpgsign=true commit -m x"}}' \
  "skill-must-not-commit" "R9-git-c-flag-still-denies"

# ============================================================================
# R12: spex hash
# ============================================================================
spex_hash='{"tool_name":"Bash","tool_input":{"command":"bin/spex hash"}}'

# Default → deny
unset SPEX_REBASELINE
clear_skill
assert_deny "$hooks_dir/check-spex-hash-rebaseline.sh" "$spex_hash" \
  "spex-hash-bypasses-pipeline" "R12-default-denies"

# Even in /converge → deny
set_skill "converge"
assert_deny "$hooks_dir/check-spex-hash-rebaseline.sh" "$spex_hash" \
  "spex-hash-bypasses-pipeline" "R12-converge-still-denies"

# SPEX_REBASELINE=1 → allow
out="$(printf '%s' "$spex_hash" \
  | SPEX_REBASELINE=1 "$hooks_dir/check-spex-hash-rebaseline.sh" 2>/dev/null)"
[[ -z "$out" ]] || fail "R12-rebaseline-1" "SPEX_REBASELINE=1 should allow, got: $out"

# Non-hash subcommand → allow
assert_allow "$hooks_dir/check-spex-hash-rebaseline.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"bin/spex diff"}}' \
  "R12-spex-diff-allows"

# Heredoc body mention → allow
assert_allow "$hooks_dir/check-spex-hash-rebaseline.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"$(cat <<EOF\nbin/spex hash bla\nEOF\n)\""}}' \
  "R12-spex-hash-in-heredoc-allows"

echo "ok"
