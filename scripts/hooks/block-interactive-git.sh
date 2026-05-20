#!/usr/bin/env bash
# block-interactive-git.sh — R11 (interactive-git-not-supported)
#
# Blocks Bash commands that invoke interactive git modes which the
# agent cannot drive (no TTY for editor / picker interaction):
#   git rebase -i / --interactive
#   git rebase --edit-todo
#   git add -i / --interactive / -p / --patch
#
# Operational rule (not in CLAUDE.md as of this PR).

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Strip heredoc bodies AND quoted strings: a real interactive flag is
# unquoted; the same text in a commit message, an `--exec '<cmd>'`
# argument, or doc prose must not trip the hook.
stripped="$(strip_quoted_strings "$(strip_heredoc_bodies "$command")")"

# has_flag <text> <flag> — true if <flag> appears as a whitespace-
# separated token.
has_flag() {
  printf '%s' " $1 " | grep -qE "[[:space:]]$2[[:space:]]"
}

block=false
# `git rebase` / `git add` must be a real command (boundary-anchored),
# not the words appearing inside an argument.
if cmd_matches "$command" 'git[[:space:]]+rebase([[:space:]]|$)'; then
  if has_flag "$stripped" "-i" \
     || has_flag "$stripped" "--interactive" \
     || has_flag "$stripped" "--edit-todo"; then
    block=true
  fi
fi
if cmd_matches "$command" 'git[[:space:]]+add([[:space:]]|$)'; then
  if has_flag "$stripped" "-i" \
     || has_flag "$stripped" "-p" \
     || has_flag "$stripped" "--interactive" \
     || has_flag "$stripped" "--patch"; then
    block=true
  fi
fi

if [[ "$block" == true ]]; then
  emit_halt \
    "interactive-git-not-supported" \
    "$command" \
    "Interactive git modes (-i, --interactive, -p, --patch) require a TTY that the agent cannot drive." \
    "scripts/hooks/block-interactive-git.sh" \
    "Use a non-interactive equivalent: git rebase --exec '<cmd>' <base>, git add <specific-paths>, git restore --staged <paths>" false \
    "If interactive editing is genuinely required, the user runs it directly outside the agent" false
fi
exit 0
