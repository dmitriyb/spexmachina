#!/usr/bin/env bash
# check-branch-from-origin-main.sh — R4 (branch-not-from-origin-main)
#
# Branch-creation commands must specify origin/main as the start point.
# Catches:
#   git checkout -b <name>            ← no start point, inherits current
#   git switch -c <name>              ← same
#   git checkout -b <name> <other>    ← explicit non-origin/main
#   git branch <name>                 ← no start point
#
# Allowed:
#   git checkout -b <name> origin/main
#   git switch -c <name> origin/main
#   git branch <name> origin/main
#
# CLAUDE.md:39 — Always branch from origin/main, not the current branch.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"

# Gate: only a real branch-CREATION command at a command boundary.
if ! cmd_matches "$command" "$SPEX_ERE_BRANCH_CREATE"; then
  exit 0
fi

stripped="$(strip_heredoc_bodies "$command")"

# Parse the start point. Three creation forms; `switch -C` (force) and
# `branch -f` (force-create) are handled so they cannot bypass.
rest=""
if [[ "$stripped" =~ git[[:space:]]+checkout[[:space:]]+-b[[:space:]]+([^[:space:]]+)([[:space:]]+(.*))? ]]; then
  rest="${BASH_REMATCH[3]:-}"
elif [[ "$stripped" =~ git[[:space:]]+switch[[:space:]]+-[cC][[:space:]]+([^[:space:]]+)([[:space:]]+(.*))? ]]; then
  rest="${BASH_REMATCH[3]:-}"
elif [[ "$stripped" =~ git[[:space:]]+branch[[:space:]]+(-f[[:space:]]+)?([^-][^[:space:]]*)([[:space:]]+(.*))? ]]; then
  rest="${BASH_REMATCH[4]:-}"
else
  exit 0
fi

# First whitespace token of the remainder is the start point.
start="$(awk '{print $1}' <<<"$rest")"

if [[ "$start" == "origin/main" ]]; then
  exit 0
fi

emit_halt \
  "branch-not-from-origin-main" \
  "$command" \
  "New branches must be created from origin/main." \
  "CLAUDE.md:39" \
  "Re-run with origin/main as the start point: git switch -c <branch> origin/main" false \
  "Or: git checkout -b <branch> origin/main" false \
  "On first push, use git push -u origin <branch> so upstream tracks the new branch (not origin/main)" false
exit 0
