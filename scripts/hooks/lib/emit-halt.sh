#!/usr/bin/env bash
# emit-halt.sh — build and emit spex-halt/v1 payloads.
#
# Three public functions, layered:
#
#   build_halt_inner  -> compact JSON string (the canonical payload)
#   emit_halt         -> Claude Code PreToolUse wire envelope on stdout
#                        (uses build_halt_inner, also logs)
#   emit_git_halt     -> structured stderr message for git hooks
#                        (uses build_halt_inner, also logs)
#
# Sourced by every hook script. Do not run directly.
#
# Canonical inner spex-halt/v1 schema (see docs/enforcement-rfc.md §4.1):
#   {protocol, rule, command, cwd, head, skill, invariant, source,
#    recovery: [{path, destructive}, ...], directive}
#
# Usage (all three functions take the same args):
#   <fn> <rule> <command> <invariant> <source> [<path> <destructive> ...]
#   Trailing args are recovery items as alternating pairs.
#   Pass `false`/`true` literally (no quotes) so jq --argjson can consume them.

_repo_root_for_halt() {
  git rev-parse --show-toplevel 2>/dev/null || echo "$PWD"
}

build_halt_inner() {
  if [[ $# -lt 4 ]]; then
    echo "build_halt_inner: need at least <rule> <command> <invariant> <source>" >&2
    return 1
  fi
  local rule="$1" command="$2" invariant="$3" source_loc="$4"
  shift 4

  if (( $# % 2 != 0 )); then
    echo "build_halt_inner: trailing recovery args must be (path, destructive) pairs" >&2
    return 1
  fi

  local recovery_json="[]"
  while [[ $# -ge 2 ]]; do
    recovery_json="$(jq -c --arg p "$1" --argjson d "$2" \
      '. + [{path:$p, destructive:$d}]' <<<"$recovery_json")"
    shift 2
  done

  jq -nc \
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
    }'
}

_log_halt() {
  # Append the inner payload to .claude/hook-violations.log. Non-fatal.
  local inner="$1"
  local log_violation
  log_violation="$(_repo_root_for_halt)/scripts/hooks/log-violation"
  if [[ -x "$log_violation" ]]; then
    printf '%s\n' "$inner" | "$log_violation" || true
  fi
}

emit_halt() {
  # For Claude Code PreToolUse hooks. Prints the wire envelope to stdout.
  # Caller MUST exit 0 after invoking this.
  local inner
  inner="$(build_halt_inner "$@")" || return 1
  _log_halt "$inner"
  jq -nc --arg reason "$inner" \
    '{hookSpecificOutput:
        {hookEventName: "PreToolUse",
         permissionDecision: "deny",
         permissionDecisionReason: $reason}}'
}

emit_git_halt() {
  # For git hooks. Prints a structured banner + pretty-printed inner
  # payload to stderr. Caller decides exit code (1 for pre-commit /
  # pre-push, error-ignored for post-commit).
  local inner rule
  inner="$(build_halt_inner "$@")" || return 1
  rule="$1"
  _log_halt "$inner"
  printf 'BLOCKED — git hook (%s):\n' "$rule" >&2
  printf '%s\n' "$inner" | jq . >&2
}
