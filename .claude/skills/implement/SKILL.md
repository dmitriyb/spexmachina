---
name: implement
description: Implement a beads task — write code, tests, and create a PR
disable-model-invocation: true
argument-hint: <bead-id>
---

First run `git checkout main && git pull --rebase` to ensure you are on the latest main.

Implement bead $ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific guidance.

## Context Loading

1. Run `br show $ARGUMENTS` to get the full bead details (title, description, labels, dependencies)
2. Read spec files from the bead's labels:
   - Find labels `spec_module:<module>` and `spec_component:<component>`
   - Read `spec/<module>/arch_<snake_case(component)>.md` for architecture
   - Read `spec/<module>/impl_<snake_case(component)>.md` for implementation details
   - Read `spec/<module>/flow_*.md` for data flow between components
   - Read `spec/<module>/module.json` for requirements the component implements (check `implements` field)
3. If no spec labels exist, fall back to reading any spec references in the description

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
