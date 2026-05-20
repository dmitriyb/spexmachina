#!/usr/bin/env bash
# check-not-on-main.sh — R13 (editing-on-protected-branch)
#
# Fires on PreToolUse for Edit, Write, NotebookEdit, and `git commit`
# Bash calls. Blocks if HEAD's symbolic ref is `main`.
#
# CLAUDE.md:37 — main is protected.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"

# For Bash, only flag `git commit` (the mutating action). Read tool
# calls and inspection Bash commands are allowed on main.
case "$tool" in
  Edit|Write|NotebookEdit)
    target="$(jq -r '.tool_input.file_path // empty' <<<"$input")"
    [[ -z "$target" ]] && exit 0
    ;;
  Bash)
    command="$(jq -r '.tool_input.command // empty' <<<"$input")"
    if ! cmd_matches "$command" "$SPEX_ERE_GIT_COMMIT"; then
      exit 0
    fi
    target="$command"
    ;;
  *) exit 0 ;;
esac

branch="$(git symbolic-ref --short HEAD 2>/dev/null || echo '')"
if [[ "$branch" != "main" ]]; then
  exit 0
fi

emit_halt \
  "editing-on-protected-branch" \
  "$target" \
  "main is protected; create a feature branch before editing or committing." \
  "CLAUDE.md:37" \
  "Create a feature branch from origin/main: git switch -c <branch>" false \
  "If a read-only Edit was intended, use Read instead" false
exit 0
