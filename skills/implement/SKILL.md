---
name: implement
description: Implement a beads task — write code, tests, and create a PR
disable-model-invocation: true
argument-hint: <bead-id>
---

**Commits and pushes:** yes. This skill commits, pushes, opens a PR, and links the bead. Enforcement hook `check-skill-commit-allowed.sh` permits `git commit` when the active skill is `implement`. This skill does NOT close beads — `br close` is the `/review` skill's responsibility (R6 enforcement).

## Step 0: Declare skill identity to enforcement hooks

Before any other action, run this command verbatim so the hook layer knows the active skill (see CLAUDE.md "## Enforcement"):

```bash
mkdir -p .claude && printf '{"skill":"implement","started_at":"%s","pid":%d}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > .claude/skill-context.json
```

First run `git checkout main && git pull --rebase` to ensure you are on the latest main.

Implement bead $ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific guidance.

## Dispatch Check

Before anything else, run `br show $ARGUMENTS --json` and look at the labels. If the bead carries a `spex:cleanup` label, this is a cleanup bead — use `/cleanup` instead. Stop here and tell the user. The workflows differ (removal vs implementation) and using `/implement` for a cleanup bead will fail the context-loading step because cleanup beads have no `spex:<record-id>` label or bead-map record.

## Context Loading

1. Run `br show $ARGUMENTS` to get the full bead details (title, description, labels, dependencies)
2. Extract the `spex:<record_id>` label from the bead's labels array (e.g. `spex:22` → record ID 22)
3. Run `bin/spex map context <record_id>` to get all spec file paths deterministically
4. Read all files returned in the context result: `arch_file`, all `impl_files`, all `test_files`, all `flow_files`, and `module_file`
5. If no `spex:` label exists and the bead is NOT a cleanup bead (checked above), fall back to reading any spec references in the description

## Pre-flight Checks

After context loading, run ALL of the following checks. If ANY check fails, **STOP immediately**, report the failure to the user, and do NOT proceed to the workflow.

### Check 1: Bead status

The bead must be in `open` or `ready` status. If it is `in_progress`, `closed`, or `tombstone`, stop and tell the user.

### Check 2: Git signing key available

Run `ssh-add -l` and verify at least one key is loaded. If no keys are found (or the agent is not running), stop and tell the user:

> Cannot implement $ARGUMENTS — no SSH signing key available in the agent. Run `ssh-add` to load your key before proceeding.

This ensures all commits will be signed. Do NOT bypass signing with `--no-gpg-sign` or `-c commit.gpgsign=false`.

## Workflow (TDD)

1. Read the bead fully. Understand acceptance criteria before writing code.
2. Claim the bead: `br update $ARGUMENTS --status in_progress`
3. Create a feature branch: `git checkout -b <short-descriptive-name> origin/main`
4. **Fix deferred breakage.** Search the codebase for `TODO(bead:<this-bead-id>)` markers left by other beads. Fix these first — they represent compilation breakage from upstream changes that was deferred to this bead.
5. **Write integration tests first.** Read the `test_*.md` spec files loaded during context loading. Write tests that cover every scenario defined there. These tests should compile but fail — they exercise behavior that does not exist yet. Run `go test ./...` to confirm they fail for the right reasons (missing functions, wrong output, etc. — not compilation errors).

   **Test-section scope guard**: the `test_files` array returned by `bin/spex map context <record_id>` IS the bead's test-section ownership boundary. Write test cases ONLY for scenarios described in those files. The same source test file in the codebase (regardless of language: `*_test.go`, `*_spec.rb`, `*.test.ts`, `test_*.py`, etc.) may legitimately host test cases owned by multiple beads — only write the ones whose scenarios trace to YOUR bead's `test_files`. If a test would cover a scenario from any other `test_*.md` (one not in your `test_files`), STOP — that test belongs to that test_section's bead. This is a separate axis from the file-ownership rule below: file-ownership says "don't edit files owned by another component"; test-section-ownership says "don't write tests for scenarios owned by another bead, even if they would naturally live in your component's test file."
6. **Write unit tests.** Based on the impl spec and architecture, write unit tests for internal functions and edge cases. These also fail initially.
7. **Write the implementation.** Write code that traces to requirements described in the bead. Only implement the single component this bead covers. Follow patterns in existing codebase. No unrelated changes.

**Scope boundary**: Only modify logic and tests for the component this bead covers. If your changes cause compilation errors in files owned by a different component, comment out the broken code with `// TODO(bead:<other-bead-id>): fix after <your-bead-id> changed <what>`. Look up the correct bead ID from `.bead-map.json` for that component. Do NOT rewrite the other component's logic or tests — that work belongs to their bead.
8. **Run tests.** Run `go test ./...` and `go vet ./...`. All tests from steps 5-6 must now pass. If any test still fails, fix the implementation — do not weaken or delete the test.
9. **Completion gate** (see below). Do NOT proceed until every item passes.
10. Commit and push.
11. Create a PR using `.github/pull_request_template.md`. Fill in the bead ID, spec references from the bead metadata, and changes summary.
12. Link the bead to the PR: `br update $ARGUMENTS --external-ref "PR#<number>"`, then commit `.beads/issues.jsonl` and push so the bead state is tracked in git. Then check the box in the PR body: `gh pr edit <number> --body "$(gh pr view <number> --json body --jq '.body' | sed 's/- \[ \] Bead linked to PR/- [x] Bead linked to PR/')"

## Completion Gate

Before committing, re-read the bead description and verify **every** claim is met. This is mandatory — do not skip it.

1. **Requirements satisfied**: Re-read the bead title and description line by line. For each stated requirement or behavior, identify the code that implements it. If you cannot point to concrete code for a requirement, it is not done.
2. **No deferred work**: Search your changes for `TODO`, `FIXME`, `HACK`, `WORKAROUND`, shim functions, and compatibility wrappers. If any of these exist for work that the bead is supposed to deliver, the implementation is incomplete. Either finish the work or stop and tell the user you cannot complete the bead as scoped.
3. **Verbs are true**: If the bead says "replaces", the old thing must be gone. If it says "adds", the new thing must exist and work. If it says "removes", the thing must not be present. Do not reinterpret the bead's language — take it literally.
4. **Tests cover requirements**: Each requirement from the bead must have at least one test that would fail if the requirement were not implemented. Tests that only assert happy-path output are insufficient if the bead specifies error behavior or edge cases.`
