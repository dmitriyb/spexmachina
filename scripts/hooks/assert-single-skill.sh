#!/usr/bin/env bash
# assert-single-skill.sh — R14 (skill-mixing-detected)
#
# A skill-frontmatter PreToolUse hook declared in EVERY skill's
# frontmatter. Enforces "one skill per Claude Code session": the first
# skill to run in a session owns it; invoking a different skill in the
# same session is blocked.
#
# How it works: the hook stdin carries a `session_id`. The first time
# any skill's assert-single-skill.sh fires in a session, it records
# the declaring skill's name under .claude/skill-sessions/<session_id>.
# A later hook firing for the SAME session but a DIFFERENT skill name
# means two skills are being mixed — halt.
#
# Arg $1 (required): the declaring skill's name.
#
# Rationale: docs/enforcement-rfc.md §9.1 — one-skill-per-session makes
# the (undocumented) skill-frontmatter-hook lifecycle safe: a lingering
# hook can only ever affect the skill it belongs to.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

this_skill="${1:-}"
[[ -z "$this_skill" ]] && exit 0  # misconfigured frontmatter — fail open

export SPEX_SKILL="$this_skill"

input="$(cat)"
session_id="$(jq -r '.session_id // empty' <<<"$input")"
[[ -z "$session_id" ]] && exit 0  # no session id — cannot key; fail open

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")"
session_dir="$repo_root/.claude/skill-sessions"
mkdir -p "$session_dir" 2>/dev/null || exit 0

# Sweep markers older than 24h so the dir does not grow unbounded
# (one file per session). Non-fatal.
find "$session_dir" -maxdepth 1 -type f -mmin +1440 -delete 2>/dev/null || true

# Sanitise session_id for use as a filename (UUIDs are safe, but be
# defensive against unexpected characters).
safe_id="$(printf '%s' "$session_id" | tr -c 'A-Za-z0-9_.-' '_')"
marker="$session_dir/$safe_id"

# Atomic create-if-absent: `set -o noclobber` makes `>` fail when the
# file already exists, so the test-and-write is a single operation —
# no TOCTOU between checking and writing.
if ( set -o noclobber; printf '%s\n' "$this_skill" > "$marker" ) 2>/dev/null; then
  exit 0  # we created it — first (and so far only) skill in this session
fi

# Marker already existed — read the owning skill.
owner="$(cat "$marker" 2>/dev/null || echo '')"
if [[ -z "$owner" || "$owner" == "$this_skill" ]]; then
  exit 0
fi

emit_halt \
  "skill-mixing-detected" \
  "/$this_skill invoked in a session already running /$owner" \
  "Each Claude Code session runs exactly one skill. This session is already running /$owner." \
  "docs/enforcement-rfc.md §9.1 (R14)" \
  "Start a fresh Claude Code session and run /$this_skill there" false \
  "Finish or abandon the /$owner work in this session first" false
exit 0
