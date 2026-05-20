#!/usr/bin/env bash
# test-cc-skill-aware.sh — verify the skill-frontmatter hooks
# (deny-commit.sh, deny-br-close.sh) and the R12 project hook
# (check-spex-hash-rebaseline.sh).
#
# deny-commit.sh and deny-br-close.sh are context-free: they are
# scoped by *being declared in a skill's frontmatter*, so the script
# itself has no skill logic. The test invokes them directly and
# asserts they deny their target command and allow everything else.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$repo_root/scripts/hooks"

fail() { echo "FAIL ${1}: ${2}" >&2; exit 1; }

# assert_deny <hook> <stdin-json> <expected-rule> <name> [arg]
assert_deny() {
  local hook="$1" stdin="$2" expected_rule="$3" name="$4" arg="${5:-}"
  local out
  out="$(printf '%s' "$stdin" | "$hook" "$arg" 2>/dev/null)"
  [[ -z "$out" ]] && fail "$name" "expected deny envelope, got nothing"
  local rule
  rule="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason' \
          | jq -r '.rule // empty')"
  [[ "$rule" == "$expected_rule" ]] || fail "$name" "want rule '$expected_rule', got '$rule'"
}

# assert_allow <hook> <stdin-json> <name> [arg]
assert_allow() {
  local hook="$1" stdin="$2" name="$3" arg="${4:-}"
  local out
  out="$(printf '%s' "$stdin" | "$hook" "$arg" 2>/dev/null)"
  [[ -z "$out" ]] || fail "$name" "expected allow, got: $out"
}

# ============================================================================
# deny-commit.sh (R9) — denies `git commit`, allows everything else.
# ============================================================================
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git commit -m foo"}}' \
  "skill-must-not-commit" "R9-denies-commit" "spec"
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git -c commit.gpgsign=true commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-git-c-commit" "converge"
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
  "R9-allows-status" "spec"
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git add -p"}}' \
  "R9-allows-add" "spec"
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Edit","tool_input":{"file_path":"x"}}' \
  "R9-allows-non-bash" "spec"
# git commit mentioned inside a heredoc body must not trip.
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"echo hi > f && cat <<EOF\nrun git commit later\nEOF"}}' \
  "R9-allows-commit-in-heredoc" "spec"
# Regression S2: git global flags before `commit` must not bypass.
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git --git-dir=/r/.git commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-git-dir-flag" "spec"
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git -C /some/path commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-C-flag" "spec"
# Regression (env-prefix): a leading VAR=value / env prefix must not bypass.
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"FOO=bar git commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-env-assignment-prefix" "spec"
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"GIT_DIR=.git git commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-git-env-prefix" "spec"
assert_deny "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"cd /tmp && FOO=1 git commit -m x"}}' \
  "skill-must-not-commit" "R9-denies-env-prefix-mid-chain" "spec"
# Regression B2: `git commit` inside a single-line quoted string is prose.
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"echo \"run git commit now\""}}' \
  "R9-allows-commit-in-quotes" "spec"
# `git checkout commit` (a ref named commit) is not a commit command.
assert_allow "$hooks_dir/deny-commit.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git checkout commit"}}' \
  "R9-allows-checkout-of-ref-named-commit" "spec"

# ============================================================================
# deny-br-close.sh (R6) — denies `br close`, allows everything else.
# ============================================================================
assert_deny "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"br close spexmachina-1dj1 --reason foo"}}' \
  "br-close-outside-review" "R6-denies-br-close" "implement"
assert_allow "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"br show spexmachina-1dj1"}}' \
  "R6-allows-br-show" "implement"
assert_allow "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"br update x --status in_progress"}}' \
  "R6-allows-br-update" "implement"
assert_allow "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Read","tool_input":{"file_path":"x"}}' \
  "R6-allows-non-bash" "implement"
# br close inside a heredoc body must not trip.
assert_allow "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"$(cat <<EOF\nbr close docs\nEOF\n)\""}}' \
  "R6-allows-br-close-in-heredoc" "implement"
# Regression S1: a path-prefixed `br` must not bypass.
assert_deny "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"bin/br close x"}}' \
  "br-close-outside-review" "R6-denies-bin-br-close" "implement"
assert_deny "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"/usr/local/bin/br close x"}}' \
  "br-close-outside-review" "R6-denies-abspath-br-close" "implement"
# Regression (env-prefix): VAR=value / env prefix must not bypass.
assert_deny "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"env FOO=1 br close x"}}' \
  "br-close-outside-review" "R6-denies-env-prefix" "implement"
# Regression B2: `br close` inside a single-line quoted string is prose.
assert_allow "$hooks_dir/deny-br-close.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"echo \"to finish, run br close x\""}}' \
  "R6-allows-br-close-in-quotes" "implement"

# ============================================================================
# check-spex-hash-rebaseline.sh (R12) — project hook, env-var gated.
# ============================================================================
spex_hash='{"tool_name":"Bash","tool_input":{"command":"bin/spex hash"}}'

unset SPEX_REBASELINE
assert_deny "$hooks_dir/check-spex-hash-rebaseline.sh" "$spex_hash" \
  "spex-hash-bypasses-pipeline" "R12-default-denies"

out="$(printf '%s' "$spex_hash" \
  | SPEX_REBASELINE=1 "$hooks_dir/check-spex-hash-rebaseline.sh" 2>/dev/null)"
[[ -z "$out" ]] || fail "R12-rebaseline-1" "SPEX_REBASELINE=1 should allow, got: $out"

assert_allow "$hooks_dir/check-spex-hash-rebaseline.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"bin/spex diff"}}' \
  "R12-allows-spex-diff"
# Regression (env-prefix): VAR=value prefix must not bypass.
assert_deny "$hooks_dir/check-spex-hash-rebaseline.sh" \
  '{"tool_name":"Bash","tool_input":{"command":"FOO=1 bin/spex hash"}}' \
  "spex-hash-bypasses-pipeline" "R12-denies-env-prefix"

echo "ok"
