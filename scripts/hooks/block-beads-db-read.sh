#!/usr/bin/env bash
# block-beads-db-read.sh — R7 + R8
#   R7 (beads-db-direct-read)  — block direct reads of .beads/beads.db
#   R8 (tracker-storage-bypass) — block bypass of br/spex interfaces
#                                  to dig into their storage
#
# CLAUDE.md:62-63 — Use br commands or .beads/issues.jsonl; never
# bypass the documented interfaces.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"

case "$tool" in
  Bash)
    target="$(jq -r '.tool_input.command // empty' <<<"$input")"
    # Direct reads of .beads/beads.db
    if [[ "$target" =~ (sqlite3|cat|head|tail|less|more|xxd|hexdump|od|strings|file|python|python3|perl|ruby).*\.beads/beads\.db ]] \
       || [[ "$target" =~ \.beads/beads\.db.*\.(dump|backup) ]] \
       || [[ "$target" =~ sqlite[0-9]?[[:space:]]+.*\.beads/ ]]; then
      rule="beads-db-direct-read"
    else
      exit 0
    fi
    ;;
  Read)
    target="$(jq -r '.tool_input.file_path // empty' <<<"$input")"
    if [[ "$target" == *".beads/beads.db" ]] \
       || [[ "$target" == *".beads/.br_history" ]] \
       || [[ "$target" == *".beads/.br_recovery" ]]; then
      rule="beads-db-direct-read"
    else
      exit 0
    fi
    ;;
  *) exit 0 ;;
esac

emit_halt \
  "$rule" \
  "$target" \
  "Never read .beads/beads.db directly. Use br commands or .beads/issues.jsonl." \
  "CLAUDE.md:62" \
  "For a single bead: br show <id> --json" false \
  "For listings (note: br list filters silently — open-only, limit 50): br list --all --limit 0" false \
  "For canonical tracker state: jq -s '{issues: .}' .beads/issues.jsonl" false
exit 0
