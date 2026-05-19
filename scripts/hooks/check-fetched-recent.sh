#!/usr/bin/env bash
# check-fetched-recent.sh — R3 (stale-origin)
#
# Before creating a new branch, require that `git fetch origin` ran
# within the last 15 minutes. Reads .git/FETCH_HEAD mtime.
#
# Honors SPEX_OFFLINE=1 — set when working intentionally offline.
#
# CLAUDE.md:38 — Always `git fetch origin` before creating a new branch.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

# TTL in seconds (default 15 min; tune via SPEX_FETCH_TTL).
ttl="${SPEX_FETCH_TTL:-900}"

[[ "${SPEX_OFFLINE:-}" == "1" ]] && exit 0

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
stripped="$(strip_heredoc_bodies "$command")"
# Match: git checkout -b <name>, git switch -c <name>, git branch <name> <start>
if [[ ! "$stripped" =~ git[[:space:]]+(checkout[[:space:]]+-b|switch[[:space:]]+-c|branch)([[:space:]]|$) ]]; then
  exit 0
fi

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || echo '')"
fetch_head="$repo_root/.git/FETCH_HEAD"

ok=0
if [[ -f "$fetch_head" ]]; then
  age=$(( $(date +%s) - $(stat -c %Y "$fetch_head" 2>/dev/null || stat -f %m "$fetch_head" 2>/dev/null || echo 0) ))
  if (( age <= ttl )); then ok=1; fi
fi

if (( ok == 1 )); then exit 0; fi

emit_halt \
  "stale-origin" \
  "$command" \
  "origin/main must be fetched within the last $((ttl / 60)) minutes before creating a branch." \
  "CLAUDE.md:38" \
  "Run: git fetch origin" false \
  "Then re-run the branch-creation command" false \
  "Working offline? Set SPEX_OFFLINE=1 for this shell to skip the check" false
exit 0
