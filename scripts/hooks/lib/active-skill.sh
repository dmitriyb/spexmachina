#!/usr/bin/env bash
# active-skill.sh — resolve the currently active skill.
#
# Reads /workspace/spexmachina/.claude/skill-context.json (project-local,
# gitignored). The file is written by each skill as its first action;
# see CLAUDE.md "Enforcement" section for the shape and protocol.
#
# Returns:
#   - the skill name (string) on stdout if the marker is fresh
#     (started_at within last SPEX_SKILL_TTL seconds, default 3600)
#   - empty string if the marker is missing, malformed, or stale
#
# Exit code is always 0 — callers branch on the printed value.
#
# Source via:  source "$(dirname "$0")/lib/active-skill.sh"; skill="$(active_skill)"

active_skill() {
  local ttl="${SPEX_SKILL_TTL:-3600}"
  local repo_root marker
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")"
  marker="$repo_root/.claude/skill-context.json"

  [[ -f "$marker" ]] || { echo ""; return 0; }

  # Parse skill + started_at; bail to empty on any jq error.
  local skill started
  skill="$(jq -r '.skill // empty' "$marker" 2>/dev/null)"
  started="$(jq -r '.started_at // empty' "$marker" 2>/dev/null)"
  if [[ -z "$skill" || -z "$started" ]]; then
    echo ""
    return 0
  fi

  # Convert ISO-8601 → epoch. `date -d` works on GNU date; macOS would
  # need `date -j -f`. The project targets Linux (docker/CI), so GNU
  # date is the only path supported here. Fall back to empty on parse
  # failure rather than block.
  local started_epoch now age
  started_epoch="$(date -d "$started" +%s 2>/dev/null || echo 0)"
  if [[ "$started_epoch" == "0" ]]; then
    echo ""
    return 0
  fi
  now="$(date +%s)"
  age=$(( now - started_epoch ))
  if (( age < 0 || age >= ttl )); then
    echo ""  # stale (>=ttl) or in the future (clock skew)
    return 0
  fi

  echo "$skill"
}
