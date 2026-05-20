#!/usr/bin/env bash
# test-r14-single-skill.sh — verify assert-single-skill.sh enforces
# one-skill-per-session: first skill owns the session, the same skill
# passes, a different skill in the same session is blocked.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hook="$repo_root/scripts/hooks/assert-single-skill.sh"
session_dir="$repo_root/.claude/skill-sessions"

fail() { echo "FAIL ${1}: ${2}" >&2; exit 1; }

# Use a unique synthetic session id so the test never collides with a
# real session marker; clean it up afterwards.
sid="test-$$-$(date +%s)"
marker="$session_dir/$sid"
cleanup() { rm -f "$marker"; }
trap cleanup EXIT

stdin_for() { printf '{"session_id":"%s","tool_name":"Bash","tool_input":{"command":"git status"}}' "$1"; }

# Test 1: first skill in a fresh session — allowed, marker written.
rm -f "$marker"
out="$(stdin_for "$sid" | "$hook" spec 2>/dev/null)"
[[ -z "$out" ]] || fail "T1-first-skill" "first skill should be allowed, got: $out"
[[ -f "$marker" ]] || fail "T1-marker-written" "marker file not created"
[[ "$(cat "$marker")" == "spec" ]] || fail "T1-marker-content" "marker should say 'spec', got '$(cat "$marker")'"

# Test 2: same skill again in the same session — allowed.
out="$(stdin_for "$sid" | "$hook" spec 2>/dev/null)"
[[ -z "$out" ]] || fail "T2-same-skill" "same skill should be allowed, got: $out"

# Test 3: a different skill in the same session — blocked.
out="$(stdin_for "$sid" | "$hook" fix 2>/dev/null)"
[[ -n "$out" ]] || fail "T3-mixing" "different skill should be blocked"
rule="$(echo "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason' | jq -r '.rule // empty')"
[[ "$rule" == "skill-mixing-detected" ]] || fail "T3-rule" "want skill-mixing-detected, got '$rule'"

# Test 4: a fresh session id starts clean — different skill allowed there.
sid2="test2-$$-$(date +%s)"
marker2="$session_dir/$sid2"
out="$(stdin_for "$sid2" | "$hook" fix 2>/dev/null)"
rm -f "$marker2"
[[ -z "$out" ]] || fail "T4-fresh-session" "fresh session should allow any skill, got: $out"

# Test 5: missing session_id — fail open (allow).
out="$(printf '{"tool_name":"Bash","tool_input":{"command":"git status"}}' | "$hook" spec 2>/dev/null)"
[[ -z "$out" ]] || fail "T5-no-session-id" "missing session_id should fail open, got: $out"

# Test 6: missing skill arg — fail open (allow).
out="$(stdin_for "$sid" | "$hook" 2>/dev/null)"
[[ -z "$out" ]] || fail "T6-no-skill-arg" "missing skill arg should fail open, got: $out"

echo "ok"
