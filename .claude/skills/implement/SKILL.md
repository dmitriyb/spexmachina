---
name: implement
description: Implement a beads task — write code, tests, and create a PR
disable-model-invocation: true
argument-hint: <bead-id>
---

First run `git checkout main && git pull --rebase` to ensure you are on the latest main.

Implement bead $ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific guidance.

## Bead JSON Structure

`br list --json` returns an array of bead objects. Spec metadata is stored in the `labels` array as `key:value` strings — there is no separate `metadata` object. Key fields: `id`, `status`, `labels`, `title`, `description`, `priority`, `issue_type`, `dependency_count`, `dependent_count`. Spec-related labels use the prefixes: `spec_module:`, `spec_component:`, `spec_impl_section:`, `spec_hash:`.

## Context Loading

1. Run `br show $ARGUMENTS` to get the full bead details (title, description, labels, dependencies)
2. Read spec files from the bead's labels:
   - Find labels `spec_module:<module>` and `spec_component:<component>`
   - Read `spec/<module>/arch_<snake_case(component)>.md` for architecture
   - Read `spec/<module>/impl_<snake_case(component)>.md` for implementation details
   - Read `spec/<module>/flow_*.md` for data flow between components
   - Read `spec/<module>/module.json` for requirements the component implements (check `implements` field)
3. If no spec labels exist, fall back to reading any spec references in the description

## Pre-flight Checks

After context loading, run ALL of the following checks. If ANY check fails, **STOP immediately**, report the failure to the user, and do NOT proceed to the workflow.

### Check 1: Bead status

The bead must be in `open` or `ready` status. If it is `in_progress`, `closed`, or `tombstone`, stop and tell the user.

### Check 2: Blocking dependencies are closed

Run `br dep list $ARGUMENTS` to get the bead's dependencies. Every dependency with type `blocks` must have status `closed`. If any blocker is not closed, stop and list the blocking beads that are still open:

> Cannot implement $ARGUMENTS — blocked by open dependencies:
> - <blocker-id>: <blocker-title> (status: <status>)

### Check 3: Component dependencies have beads

Read `spec/<module>/module.json`. Find the component matching `spec_component`. Check its `uses` list — these are component IDs that this component depends on. For each used component, verify that a bead exists with matching `spec_module` and `spec_component` labels (run `br list --json` and check labels). If any used component has no bead, stop and list what's missing:

> Cannot implement $ARGUMENTS — the spec component uses other components that have no beads:
> - <ComponentName> (component id <N> in module.json) — no bead found

### Check 4: Scope is single component

The bead must map to exactly one spec component. Only write code for that component. If the component's `uses` list references other components, those components must already be implemented (their beads must be closed per Check 2). Do NOT implement dependency components inline — that is scope creep.

## Workflow

1. Read the bead fully. Understand acceptance criteria before writing code.
2. Claim the bead: `br update $ARGUMENTS --status in_progress`
3. Create a feature branch: `git checkout -b <short-descriptive-name> origin/main`
4. Write code that traces to requirements described in the bead. Only implement the single component this bead covers.
5. Follow patterns in existing codebase. No unrelated changes.
6. Write tests that verify requirements, not just coverage.
7. Run `go test ./...` and `go vet ./...` to confirm everything passes.
8. Commit and push.
9. Create a PR using `.github/pull_request_template.md`. Fill in the bead ID, spec references from the bead metadata, and changes summary.
10. Link the bead to the PR: `br update $ARGUMENTS --external-ref "PR#<number>"`, then check the box in the PR body: `gh pr edit <number> --body "$(gh pr view <number> --json body --jq '.body' | sed 's/- \[ \] Bead linked to PR/- [x] Bead linked to PR/')"`
