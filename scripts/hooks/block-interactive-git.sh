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

# Heredoc bodies are literal text passed to commands (typically
# `git commit -m "$(cat <<'EOF' ... EOF)"`). Real interactive git
# rebase/add commands never use heredocs. To prevent false positives
# from rule discussion inside commit messages or doc strings, strip
# heredoc bodies before matching.
stripped="$(printf '%s\n' "$command" | awk '
  BEGIN { in_heredoc = 0; delim = "" }
  in_heredoc {
    if ($0 ~ "^" delim "$") { in_heredoc = 0; next }
    next
  }
  match($0, /<<-?[[:space:]]*'\''?"?([A-Za-z_][A-Za-z0-9_]*)/, m) {
    delim = m[1]; in_heredoc = 1
    print substr($0, 1, RSTART - 1)
    next
  }
  { print }
')"

# has_flag <command> <flag>  — true if command contains the flag as
# a whitespace-separated token.
has_flag() {
  local cmd=" $1 "
  printf '%s' "$cmd" | grep -qE "[[:space:]]$2[[:space:]]"
}

# is_subcmd <command> <git-subcommand>  — true if command contains
# `git <subcommand>` (handles leading `git` with optional `-c k=v` flags).
is_subcmd() {
  local cmd=" $1 "
  printf '%s' "$cmd" | grep -qE "[[:space:]]git[[:space:]]+(-c[[:space:]]+[^[:space:]]+[[:space:]]+)*$2([[:space:]]|$)"
}

# Replace command with the heredoc-stripped form for matching below.
command="$stripped"

block=false
if is_subcmd "$command" "rebase"; then
  if has_flag "$command" "-i" \
     || has_flag "$command" "--interactive" \
     || has_flag "$command" "--edit-todo"; then
    block=true
  fi
fi
if is_subcmd "$command" "add"; then
  if has_flag "$command" "-i" \
     || has_flag "$command" "-p" \
     || has_flag "$command" "--interactive" \
     || has_flag "$command" "--patch"; then
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
