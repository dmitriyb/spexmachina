---
name: implement
description: Implement a beads task — write code, tests, and create a PR
disable-model-invocation: true
argument-hint: <bead-id>
---

First run `git checkout main && git pull --rebase` to ensure you are on the latest main.

Implement bead $ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific guidance.

## Context Loading

1. Run `br show $ARGUMENTS` to get the full bead details (title, description, metadata, dependencies)
2. If the bead has `metadata.spec_id` or references a spec module, read the relevant `spec/<module>_impl.md`
3. If the bead was created by `spex apply`, the description contains spec context — use it directly

**DO NOT READ** `spec/*_arch.md`, `spec/*_plan.md` — already distilled into the bead.

## Workflow

1. Read the bead fully. Understand acceptance criteria before writing code.
2. Claim the bead: `br update $ARGUMENTS --status in_progress`
3. Create a feature branch: `git checkout -b <short-descriptive-name> origin/main`
4. Write code that traces to requirements described in the bead.
5. Follow patterns in existing codebase. No unrelated changes.
6. Write tests that verify requirements, not just coverage.
7. Run `go test ./...` and `go vet ./...` to confirm everything passes.
8. Commit and push.
9. Create a PR using `.github/pull_request_template.md`. Fill in the bead ID, spec references from the bead metadata, and changes summary.
10. Link the bead to the PR: `br update $ARGUMENTS --external-ref "PR#<number>"`, then check the box in the PR body: `gh pr edit <number> --body "$(gh pr view <number> --json body --jq '.body' | sed 's/- \[ \] Bead linked to PR/- [x] Bead linked to PR/')"`
