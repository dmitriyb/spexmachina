#!/usr/bin/env bash
# check-spex-hash-rebaseline.sh — R12 (spex-hash-bypasses-pipeline)
#
# `spex hash` re-baselines the merkle snapshot. Running it to "fix" a
# non-empty diff bypasses impact/emit/ingest and orphans bead-map
# records. Allowed only when SPEX_REBASELINE=1 is set, which the
# user sets manually for the legitimate re-baseline scenario (the
# TreeBuilder keying scheme changed; see commit b847f45).
#
# No skill check — the env var is the only override.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Match a real `spex hash` at a command boundary — tolerates any path
# prefix (bin/spex, ./bin/spex); rejects mentions in quoted args /
# heredoc bodies.
if ! cmd_matches "$command" "$SPEX_ERE_SPEX_HASH"; then
  exit 0
fi

if [[ "${SPEX_REBASELINE:-}" == "1" ]]; then
  exit 0
fi

emit_halt \
  "spex-hash-bypasses-pipeline" \
  "$command" \
  "spex hash re-baselines the snapshot and bypasses the impact/emit/ingest pipeline. Allowed only with SPEX_REBASELINE=1." \
  "feedback_spex_pipeline_not_hash" \
  "Run the proper pipeline instead: spex diff → spex impact → spex emit → adapter → spex ingest" false \
  "If a re-baseline is genuinely needed (e.g., TreeBuilder keying scheme changed), the user sets SPEX_REBASELINE=1 for this shell and re-runs" false
exit 0
