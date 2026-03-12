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
   - Glob for `spec/<module>/flow_*.md` and read all matching files for data flow context
   - Read `spec/<module>/module.json` for requirements the component implements (check `implements` field)
   - Glob for `spec/<module>/test_<snake_case(component)>.md` and read all matching test spec files — these define required integration test scenarios
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

### Check 4: Git signing key available

Run `ssh-add -l` and verify at least one key is loaded. If no keys are found (or the agent is not running), stop and tell the user:

> Cannot implement $ARGUMENTS — no SSH signing key available in the agent. Run `ssh-add` to load your key before proceeding.

This ensures all commits will be signed. Do NOT bypass signing with `--no-gpg-sign` or `-c commit.gpgsign=false`.

### Check 5: Scope is single component

The bead must map to exactly one spec component. Only write code for that component. If the component's `uses` list references other components, those components must already be implemented (their beads must be closed per Check 2). Do NOT implement dependency components inline — that is scope creep.

## Workflow (TDD)

1. Read the bead fully. Understand acceptance criteria before writing code.
2. Claim the bead: `br update $ARGUMENTS --status in_progress`
3. Create a feature branch: `git checkout -b <short-descriptive-name> origin/main`
4. **Write integration tests first.** Read the `test_*.md` spec files loaded during context loading. Write tests that cover every scenario defined there. These tests should compile but fail — they exercise behavior that does not exist yet. Run `go test ./...` to confirm they fail for the right reasons (missing functions, wrong output, etc. — not compilation errors).
5. **Write unit tests.** Based on the impl spec and architecture, write unit tests for internal functions and edge cases. These also fail initially.
6. **Write the implementation.** Write code that traces to requirements described in the bead. Only implement the single component this bead covers. Follow patterns in existing codebase. No unrelated changes.
7. **Run tests.** Run `go test ./...` and `go vet ./...`. All tests from steps 4-5 must now pass. If any test still fails, fix the implementation — do not weaken or delete the test.
8. **Completion gate** (see below). Do NOT proceed until every item passes.
9. Commit and push.
10. Create a PR using `.github/pull_request_template.md`. Fill in the bead ID, spec references from the bead metadata, and changes summary.
11. Link the bead to the PR: `br update $ARGUMENTS --external-ref "PR#<number>"`, then commit `.beads/issues.jsonl` and push so the bead state is tracked in git. Then check the box in the PR body: `gh pr edit <number> --body "$(gh pr view <number> --json body --jq '.body' | sed 's/- \[ \] Bead linked to PR/- [x] Bead linked to PR/')"

## Completion Gate

Before committing, re-read the bead description and verify **every** claim is met. This is mandatory — do not skip it.

1. **Requirements satisfied**: Re-read the bead title and description line by line. For each stated requirement or behavior, identify the code that implements it. If you cannot point to concrete code for a requirement, it is not done.
2. **No deferred work**: Search your changes for `TODO`, `FIXME`, `HACK`, `WORKAROUND`, shim functions, and compatibility wrappers. If any of these exist for work that the bead is supposed to deliver, the implementation is incomplete. Either finish the work or stop and tell the user you cannot complete the bead as scoped.
3. **Verbs are true**: If the bead says "replaces", the old thing must be gone. If it says "adds", the new thing must exist and work. If it says "removes", the thing must not be present. Do not reinterpret the bead's language — take it literally.
4. **Tests cover requirements**: Each requirement from the bead must have at least one test that would fail if the requirement were not implemented. Tests that only assert happy-path output are insufficient if the bead specifies error behavior or edge cases.`
