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
stripped="$(strip_heredoc_bodies "$command")"

# Capture form + remaining args. Three branch-creation forms:
form=""
rest=""
if [[ "$stripped" =~ git[[:space:]]+checkout[[:space:]]+-b[[:space:]]+([^[:space:]]+)([[:space:]]+(.*))? ]]; then
  form="checkout-b"
  rest="${BASH_REMATCH[3]:-}"
elif [[ "$stripped" =~ git[[:space:]]+switch[[:space:]]+-c[[:space:]]+([^[:space:]]+)([[:space:]]+(.*))? ]]; then
  form="switch-c"
  rest="${BASH_REMATCH[3]:-}"
elif [[ "$stripped" =~ git[[:space:]]+branch[[:space:]]+([^-][^[:space:]]*)([[:space:]]+(.*))? ]]; then
  # `git branch <name> [<start>]` — first arg must not begin with `-`
  form="branch"
  rest="${BASH_REMATCH[3]:-}"
else
  exit 0
fi

# Strip trailing flags / unrelated noise; take the first whitespace
# token of rest as the start point.
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
