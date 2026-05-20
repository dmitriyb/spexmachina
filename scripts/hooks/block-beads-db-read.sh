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
    command="$(jq -r '.tool_input.command // empty' <<<"$input")"
    # A reading/copying tool, at a command boundary, with a
    # .beads/beads.db (or br internal-state file) argument on the SAME
    # command — [^;&|`] keeps the path bound to this command, not a
    # later one in a `&&` chain. The path may itself be quoted
    # (`cat ".beads/beads.db"`); cmd_matches anchors the keyword, so
    # quoting the path does not bypass and prose mentioning the
    # keyword inside a quoted arg does not false-trigger.
    if cmd_matches "$command" '(sqlite3?|cat|head|tail|less|more|xxd|hexdump|od|strings|file|python3?|perl|ruby|cp|dd|mv|install)[^;&|`]*\.beads/(beads\.db|\.br_)'; then
      rule="beads-db-direct-read"
      target="$command"
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
