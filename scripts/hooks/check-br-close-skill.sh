#!/usr/bin/env bash
# check-br-close-skill.sh — R6 (br-close-outside-review)
#
# `br close` is allowed only when the active skill is /review.
# Fail-CLOSED: if the marker is missing/stale, the hook blocks. This
# is the safe asymmetry described in RFC §9.1 — a false-block surfaces
# to the user; a false-allow would wrongly close beads.
#
# CLAUDE.md:49 — br close performed by /review after LGTM, never by
# any other skill or context.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"
source "$(dirname "$0")/lib/active-skill.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Match `br close ...` as a real command, not appearance in a string.
stripped="$(strip_heredoc_bodies "$command")"

if ! printf '%s' " $stripped " | grep -qE "[[:space:]]br[[:space:]]+close([[:space:]]|$)"; then
  exit 0
fi

skill="$(active_skill)"
if [[ "$skill" == "review" ]]; then
  exit 0
fi

emit_halt \
  "br-close-outside-review" \
  "$command" \
  "br close is allowed only from /review after LGTM." \
  "CLAUDE.md:49" \
  "If the PR is LGTM and ready to close, invoke /review on it; that skill writes the marker the hook checks" false \
  "If the bead should be closed for an exceptional reason, the user invokes br close directly outside the agent context" false
exit 0
