#!/usr/bin/env bash
# test-emit-halt.sh — verify the emit_halt helper produces a valid
# PreToolUse wire envelope whose permissionDecisionReason parses as a
# spex-halt/v1 payload with the expected fields.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
source "$repo_root/scripts/hooks/lib/emit-halt.sh"

# Run emit_halt in a subshell so the log-violation side effect can be
# isolated; capture wire output.
out="$(emit_halt \
  "test-rule" \
  "test command" \
  "test invariant" \
  "TEST:1" \
  "first recovery step" false \
  "destructive recovery step" true \
  2>/dev/null)"

# Outer envelope must be valid JSON with the right keys.
echo "$out" | jq -e '
  .hookSpecificOutput.hookEventName == "PreToolUse"
  and .hookSpecificOutput.permissionDecision == "deny"
  and (.hookSpecificOutput.permissionDecisionReason | type) == "string"
' >/dev/null

# Inner payload (the reason string) must parse and contain expected fields.
inner="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason')"
echo "$inner" | jq -e '
  .protocol == "spex-halt/v1"
  and .rule == "test-rule"
  and .command == "test command"
  and .invariant == "test invariant"
  and .source == "TEST:1"
  and .directive == "halt"
  and (.recovery | length) == 2
  and .recovery[0].path == "first recovery step"
  and .recovery[0].destructive == false
  and .recovery[1].path == "destructive recovery step"
  and .recovery[1].destructive == true
' >/dev/null

# Empty SPEX_SKILL must produce null skill (not "").
echo "$inner" | jq -e '.skill == null' >/dev/null

# With SPEX_SKILL set, skill must be the string.
out2="$(SPEX_SKILL=review emit_halt \
  "test-rule" "cmd" "inv" "TEST:2" 2>/dev/null)"
echo "$out2" | jq -r '.hookSpecificOutput.permissionDecisionReason' | \
  jq -e '.skill == "review"' >/dev/null

echo "ok"
