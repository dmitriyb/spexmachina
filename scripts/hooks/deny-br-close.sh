#!/usr/bin/env bash
# deny-br-close.sh — R6 (br-close-outside-review)
#
# A skill-frontmatter PreToolUse hook. Declared in the frontmatter of
# every skill EXCEPT /review. Because it is frontmatter-scoped, it only
# runs while one of those skills is active — so the script needs no
# skill-detection logic. If it runs at all, `br close` is disallowed.
# /review declares no such hook, so it is the sole skill that can close
# beads.
#
# Arg $1 (optional): the declaring skill's name, used only to populate
# the violation log's `skill` field.
#
# CLAUDE.md:49 — br close performed by /review after LGTM, never by
# any other skill.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

export SPEX_SKILL="${1:-}"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Match a real `br close` at a command boundary — tolerates a path
# prefix (bin/br, /usr/bin/br) so a path-qualified call cannot bypass;
# rejects `br close` text inside quoted args / heredoc bodies.
if cmd_matches "$command" "$SPEX_ERE_BR_CLOSE"; then
  emit_halt \
    "br-close-outside-review" \
    "$command" \
    "br close is allowed only from /review after LGTM." \
    "CLAUDE.md:49" \
    "If the PR is LGTM, run /review — it is the only skill that may close beads" false \
    "If the bead must be closed for an exceptional reason, the user invokes br close directly outside the agent" false
fi
exit 0
