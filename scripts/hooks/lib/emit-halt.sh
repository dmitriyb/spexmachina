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

# strip_heredoc_bodies <command> — print the command with bash heredoc
# bodies removed. A heredoc body is literal data (commit messages, doc
# text) and must not be matched as command text.
#
# Recognises <<EOF, <<'EOF', <<"EOF", <<-EOF. Anything between the
# `<<DELIM` opener and a line equal to DELIM (whitespace-trimmed) is
# dropped. Pure bash — no awk — so it is portable across mawk/gawk
# (Debian docker images default to mawk, which lacks 3-arg match()).
strip_heredoc_bodies() {
  local cmd="$1" line out="" in_heredoc=0 delim="" trimmed
  local re='<<-?[[:space:]]*['\''"]?([A-Za-z_][A-Za-z0-9_]*)'
  while IFS= read -r line || [[ -n "$line" ]]; do
    if (( in_heredoc )); then
      trimmed="${line#"${line%%[![:space:]]*}"}"
      trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"
      [[ "$trimmed" == "$delim" ]] && in_heredoc=0
      continue
    fi
    if [[ "$line" =~ $re ]]; then
      delim="${BASH_REMATCH[1]}"
      in_heredoc=1
      out+="${line%%<<*}"$'\n'
      continue
    fi
    out+="$line"$'\n'
  done <<< "$cmd"
  printf '%s' "$out"
}

# strip_quoted_strings <text> — blank the contents of single-line
# single- and double-quoted strings. Used by hooks that match a flag
# or token that is never legitimately quoted (e.g. --no-gpg-sign): a
# real flag is unquoted; the same text inside `-m "..."` or `echo
# "..."` is data and must not match.
#
# NOT used by hooks that match a path argument (R7) — a path CAN be
# legitimately quoted (`cat ".beads/beads.db"`); those use cmd_matches
# instead, which anchors the command keyword rather than stripping.
strip_quoted_strings() {
  printf '%s' "$1" | sed 's/"[^"]*"//g' | sed "s/'[^']*'//g"
}

# Command-start boundary: line start (^ — also covers post-newline
# since grep is line-based), or immediately after a shell command
# separator (; & | ` (), THEN optionally an `env` word and/or a run
# of leading `VAR=value` environment-assignment prefixes. A real
# command begins at one of these; the same words inside a quoted
# argument do not.
#
# The env-prefix clause is load-bearing: without it,
# `GIT_DIR=.git git commit`, `GIT_SSH_COMMAND=… git push`, and
# `env FOO=1 br close x` slip past the anchor — idiomatic git usage,
# not an exotic respelling — and hollow out every cmd_matches rule.
SPEX_CMD_BOUNDARY='(^|[;&|`(])[[:space:]]*(env[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*=[^[:space:]]*[[:space:]]+)*'

# cmd_matches <full-command> <command-ere> — true if <command-ere>
# occurs at a command-start boundary, after heredoc bodies are
# stripped. This is the matcher for rules that key on a *command*
# (git commit, br close, spex hash, sqlite3 <path>, ...): it rejects
# the command word appearing inside quoted prose while still seeing it
# when it is a real command (even if a later argument is quoted).
cmd_matches() {
  strip_heredoc_bodies "$1" | grep -qE "${SPEX_CMD_BOUNDARY}$2"
}

# Shared command EREs, used as the second arg to cmd_matches.
#
# GIT_COMMIT tolerates git global options between `git` and `commit`
# (-c k=v, -C path, --git-dir=..., etc.) so `git -C /p commit` and
# `git --git-dir=/r/.git commit` cannot bypass the rule — while still
# rejecting `git checkout commit` (a non-option token before `commit`).
SPEX_ERE_GIT_COMMIT='git[[:space:]]+((-[cC][[:space:]]+[^-][^[:space:]]*|-[^[:space:]]+)[[:space:]]+)*commit([[:space:]]|$)'

# BR_CLOSE / SPEX_HASH tolerate a path prefix (`bin/br`, `/usr/bin/br`,
# `./bin/spex`) so a path-qualified invocation cannot bypass the rule.
SPEX_ERE_BR_CLOSE='([^[:space:]]*/)?br[[:space:]]+close([[:space:]]|$)'
SPEX_ERE_SPEX_HASH='([^[:space:]]*/)?spex[[:space:]]+hash([[:space:]]|$)'

# BRANCH_CREATE matches the branch-CREATION forms only — `git branch`
# with no name (a listing) is not creation and is not matched. Handles
# `switch -C` (force) and `branch -f` (force-create).
SPEX_ERE_BRANCH_CREATE='git[[:space:]]+(checkout[[:space:]]+-b[[:space:]]|switch[[:space:]]+-[cC][[:space:]]|branch[[:space:]]+(-f[[:space:]]+)?[^-[:space:]])'

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
