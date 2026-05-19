#!/usr/bin/env bash
# check-skill-commit-allowed.sh — R9 + R10
#   R9 (skill-must-not-commit) — /spec and /converge must not commit
#   R10 (inverse guard)         — /fix, /review, /cleanup, /implement
#                                  MUST be allowed to commit and push
#
# Fail-OPEN when skill is unknown: commits outside any skill context
# are the user's prerogative and the hook does not block them. See
# RFC §9.1 asymmetry table — fail-closed here would break normal
# direct-commit workflow.
#
# CLAUDE.md "## Enforcement" — and feedback_skills_no_autocommit.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"
source "$(dirname "$0")/lib/active-skill.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Strip heredoc bodies before matching.
stripped="$(strip_heredoc_bodies "$command")"

# Trigger only on `git commit` (also matches `git -c k=v commit`).
if ! printf '%s' " $stripped " | grep -qE "[[:space:]]git[[:space:]]+(-c[[:space:]]+[^[:space:]]+[[:space:]]+)*commit([[:space:]]|$)"; then
  exit 0
fi

skill="$(active_skill)"

# Fail-open: unknown skill → allow (user-driven commit).
if [[ -z "$skill" ]]; then
  exit 0
fi

# Skills explicitly forbidden from committing.
case "$skill" in
  spec|converge|propose|spec-review|spec-drift)
    emit_halt \
      "skill-must-not-commit" \
      "$command" \
      "/$skill does not commit. Commits in this skill are user-driven." \
      "skills/$skill/SKILL.md" \
      "Stage the changes and stop: git add -p (or git add <paths>); then describe what changed and let the user write the commit message" false \
      "If a commit is genuinely required from this skill, ask the user explicitly" false
    exit 0
    ;;
esac

# Skills explicitly authorised: fix, review, cleanup, implement.
# Anything else → unknown skill name → allow (forward-compatible).
exit 0
