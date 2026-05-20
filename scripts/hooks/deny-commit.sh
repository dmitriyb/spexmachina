#!/usr/bin/env bash
# deny-commit.sh — R9 (skill-must-not-commit)
#
# A skill-frontmatter PreToolUse hook. Declared in the frontmatter of
# every skill that must NOT commit (spec, propose, converge,
# spec-review, spec-drift). Because it is frontmatter-scoped, it only
# runs while one of those skills is active — so the script needs no
# skill-detection logic. If it runs at all, committing is disallowed.
#
# Arg $1 (optional): the declaring skill's name, used only to populate
# the violation log's `skill` field. Not used for any decision.
#
# CLAUDE.md "## Enforcement" + feedback_skills_no_autocommit.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

export SPEX_SKILL="${1:-}"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

stripped="$(strip_heredoc_bodies "$command")"

# Match `git commit` (also `git -c k=v commit`).
if printf '%s' " $stripped " | grep -qE "[[:space:]]git[[:space:]]+(-c[[:space:]]+[^[:space:]]+[[:space:]]+)*commit([[:space:]]|$)"; then
  emit_halt \
    "skill-must-not-commit" \
    "$command" \
    "This skill does not commit. Commits in an authoring/pipeline skill are user-driven." \
    "skills/${SPEX_SKILL:-<skill>}/SKILL.md" \
    "Stage the changes and stop: git add -p (or git add <paths>); describe what changed and let the user write the commit" false \
    "If a commit is genuinely required from this skill, ask the user explicitly" false
fi
exit 0
