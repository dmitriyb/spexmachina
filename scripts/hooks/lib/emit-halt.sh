#!/usr/bin/env bash
# emit-halt.sh — produce the wire-format response that Claude Code's
# PreToolUse hook protocol expects for a "deny" decision, AND log the
# canonical spex-halt/v1 payload to .claude/hook-violations.log.
#
# Sourced by every hook script under scripts/hooks/. Do not run directly.
#
# Wire protocol (Claude Code):
#   {"hookSpecificOutput": {"hookEventName": "PreToolUse",
#                           "permissionDecision": "deny",
#                           "permissionDecisionReason": "<inner JSON as string>"}}
#   Exit 0. (Exit 2 + stderr also blocks but only as raw text.)
#
# Inner spex-halt/v1 schema (the canonical form):
#   {"protocol": "spex-halt/v1",
#    "rule": "<slug>",
#    "command": "<attempted command or target>",
#    "cwd": "<pwd>",
#    "head": "<branch>",
#    "skill": "<active skill or null>",
#    "invariant": "<the rule>",
#    "source": "<file:line>",
#    "recovery": [{"path": "<step>", "destructive": <bool>}, ...],
#    "directive": "halt"}
#
# Usage:
#   emit_halt <rule> <command> <invariant> <source> [<path> <destructive> ...]
#   Trailing args are recovery items as alternating pairs.
#   Pass false/true literally (no quotes) so jq --argjson can consume them.
#
# Side effects:
#   - Appends one line of spex-halt/v1 JSON (with `ts`) to
#     .claude/hook-violations.log via scripts/hooks/log-violation.
#
# Output:
#   - Writes the wire-format envelope to stdout.
#   - Caller MUST exit 0 after calling this (the envelope is the block).

emit_halt() {
  if [[ $# -lt 4 ]]; then
    echo "emit_halt: need at least <rule> <command> <invariant> <source>" >&2
    return 1
  fi
  local rule="$1" command="$2" invariant="$3" source_loc="$4"
  shift 4

  if (( $# % 2 != 0 )); then
    echo "emit_halt: trailing recovery args must be (path, destructive) pairs" >&2
    return 1
  fi

  local recovery_json="[]"
  while [[ $# -ge 2 ]]; do
    recovery_json="$(jq -c --arg p "$1" --argjson d "$2" \
      '. + [{path:$p, destructive:$d}]' <<<"$recovery_json")"
    shift 2
  done

  local inner
  inner="$(jq -nc \
    --arg rule "$rule" \
    --arg command "$command" \
    --arg invariant "$invariant" \
    --arg source_loc "$source_loc" \
    --arg cwd "$PWD" \
    --arg head "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '')" \
    --arg skill "${SPEX_SKILL:-}" \
    --argjson recovery "$recovery_json" \
    '{
      protocol: "spex-halt/v1",
      rule: $rule,
      command: $command,
      cwd: $cwd,
      head: $head,
      skill: (if $skill == "" then null else $skill end),
      invariant: $invariant,
      source: $source_loc,
      recovery: $recovery,
      directive: "halt"
    }')"

  local repo_root log_violation
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")"
  log_violation="$repo_root/scripts/hooks/log-violation"
  if [[ -x "$log_violation" ]]; then
    printf '%s\n' "$inner" | "$log_violation" || true
  fi

  jq -nc --arg reason "$inner" \
    '{hookSpecificOutput:
        {hookEventName: "PreToolUse",
         permissionDecision: "deny",
         permissionDecisionReason: $reason}}'
}
