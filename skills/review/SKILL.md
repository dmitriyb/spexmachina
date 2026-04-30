---
name: review
description: Review a pull request for correctness, spec traceability, and test quality
disable-model-invocation: true
argument-hint: <pr-number>
---

Review PR #$ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific review guidance.

## Context Loading

Read these documents:

1. The PR diff and description
2. The linked bead: run `br show <bead-id>` using the bead ID from the PR description
3. Read spec files from the bead's labels:
   - Find labels `spec_module:<module>` and `spec_component:<component>`
   - Read `spec/<module>/arch_<snake_case(component)>.md` for architecture
   - Read `spec/<module>/impl_<snake_case(component)>.md` for implementation details
   - Glob for `spec/<module>/flow_*.md` and read all matching files for data flow context
   - Read `spec/<module>/module.json` for requirements the component implements (check `implements` field)
   - Glob for `spec/<module>/test_<snake_case(component)>.md` and read all matching test spec files — these define required integration test scenarios
4. If no spec labels exist, fall back to reading any spec references in the description

## Review Flow

This skill supports an iterative cycle: `implement → [review → fix → review] → close`. A single four-step flow handles both first reviews and follow-ups. The action at the end depends on the `(mode, result)` pair from Step 2 and Step 3.

### Step 0: Resolve repo slug

Run `gh repo view --json owner,name --jq '.owner.login + "/" + .name'` to get the `{owner}/{repo}` slug. Use this resolved value in all subsequent `gh api` calls. Do NOT guess the owner from the git remote or working directory name.

### Step 1: Fetch review state

Fetch from GitHub everything needed to classify the invocation:

1. **Review-level comments**: `gh api repos/{owner}/{repo}/pulls/{number}/reviews` — look for reviews with `state` = `COMMENTED`/`CHANGES_REQUESTED` and a non-empty `body`.
2. **Inline comments**: `gh api repos/{owner}/{repo}/pulls/{number}/comments` — line-level comments on the diff.
3. **Commits**: `gh api repos/{owner}/{repo}/pulls/{number}/commits` — used to decide whether the author responded to prior review-body feedback.

### Step 2: Classify mode

Decide what this invocation is doing based on prior feedback:

- **No prior feedback from either source** → `mode = REVIEW`.
- **Prior feedback exists and the author responded** → `mode = FOLLOWUP`.
- **Prior feedback exists but the author did not respond** → **STOP. Do nothing.** Tell the user.

How to determine if feedback has been "responded to" (this is NOT the same as "fixed" — that determination happens in Step 3):

- **Inline comments**: a top-level comment (no `in_reply_to_id`) is responded to if at least one reply references its `id`.
- **Review-body comments**: responded to if at least one commit exists **after** the review's `submitted_at` timestamp.

This gate only checks whether the author **attempted** a response. Replies like "Fixed" are not evidence of an actual fix — Step 3 verifies that independently.

### Step 3: Evaluate

Produce a `result` of either `CLEAN` (nothing to flag) or `ISSUES` (one or more blockers).

**If `mode = REVIEW`:** examine the diff against the bead and spec. Every check below must pass for `result = CLEAN`.

1. **Spec traceability**: code maps to bead requirements, no unrelated changes.
2. **Spec hygiene** (blocker): the bead's spec leaves (`arch_*.md`, `impl_*.md`, `test_*.md` resolved via `spex map context`) must match the implementation that ships in this PR. Stale prose is a blocker, not a follow-up — list every offender as an inline comment and reject. Common drift to look for:
   - `impl_*.md` referencing methods or types that no longer exist (e.g. an old `Foo.Bar()` after a struct refactor).
   - `test_*.md` scenarios describing retired preconditions (e.g. "Given X is on PATH" after the subprocess path was deleted).
   - Output-shape mismatches (e.g. spec shows `[]`, code emits `{"items":[]}`).
   - `arch_*.md` describing a contract the code does not honor.
   `spex validate` passing is **not** sufficient — it checks structural validity, not whether the prose matches the code. Read each spec leaf line by line against the diff. If the spec leaf was already drifted before this PR (e.g. a piecemeal earlier touch only updated `arch_*.md`), the PR that lands the matching code is the PR that fixes the leftovers.
3. **Bead completion** (critical — this is the most important check):
   - Re-read the bead title and description line by line. For each stated requirement, find the code in the diff that implements it. If a requirement has no corresponding code, the PR is incomplete.
   - Take the bead's verbs literally: "replaces" means the old thing is gone, "adds" means the new thing exists and works, "removes" means the thing is absent. If a verb is not satisfied, flag it.
   - Search the diff for `TODO`, `FIXME`, `HACK`, `WORKAROUND`, shim functions, and compatibility wrappers that defer work the bead is supposed to deliver. These are automatic rejections — the bead's work is not done if it leaves TODOs for itself.
4. **Correctness**: error paths handled, edge cases, no resource leaks.
5. **Patterns**: follows existing conventions, idiomatic Go (see `@~/.claude/skills/go-expert/SKILL.md`).
6. **Tests**: verify requirements not implementation details, failure cases tested.
7. **Integration testing**: if the spec includes integration test scenarios (in `test_*.md` files), the PR must include tests matching those scenarios. Missing integration tests for defined scenarios is a review blocker.
8. **Cross-bead test scope** (blocker): tests in this PR's diff must trace ONLY to scenarios in this bead's test_sections. Run `bin/spex map context <bead-id>` to get the bead's `test_files`. For each test case added or modified in the diff (regardless of source-code file or language — `*_test.go`, `*_spec.rb`, `*.test.ts`, `test_*.py`, etc.), identify which `test_*.md` scenario it implements. If a test covers a scenario from a test_section NOT in this bead's `test_files`, the implementer reached outside scope — flag as a blocker. The default action is to require removal of the cross-bead tests so the corresponding sibling bead can deliver them in its own PR; ask the user to confirm before allowing such tests to ship under this bead. Note: this check is independent of file ownership — a single test file may legitimately host tests for multiple beads' test_sections; the boundary is per-test-case, mapped via scenario-to-test_section ownership.

**If `mode = FOLLOWUP`:** verify each prior feedback item against current files.

**Replies and commit messages are not evidence.** The author saying "Fixed" or a commit titled "Fix review feedback" means nothing until you verify the actual code. Only the current state of the files determines whether an item is fixed.

For each feedback item (review body + each inline comment):

1. Read the original feedback to understand what was specifically requested.
2. Read the **current file** (not the diff, not the reply) where the fix should appear.
3. Verify the fix is genuine:
   - Does the code/spec actually contain the requested change?
   - Is the change correct, not just present? (e.g., if the review asked to add error handling, is the error handling right?)
   - Did the fix introduce any new issues?
4. Classify each item as **fixed** or **not fixed** based solely on what the code shows.

`result = CLEAN` if every feedback item is fixed; otherwise `ISSUES`.

### Step 4: Act

The `(mode, result)` pair determines the action — one of four, no other paths:

- **CLEAN + REVIEW** → Post a short "LGTM" review summary (see Posting Comments), then close the bead (see Closing the bead). Tell the user the PR is ready to merge.
- **CLEAN + FOLLOWUP** → Close the bead (see Closing the bead). Do NOT post another review. Tell the user the PR is ready to merge.
- **ISSUES + REVIEW** → Post the review with inline comments describing each blocker (see Posting Comments). Do not close.
- **ISSUES + FOLLOWUP** → For each unfixed item, post a new reply on that comment thread explaining what's still wrong. Do NOT re-review already-fixed items. Do not close.

These four actions are exhaustive. Any action not listed above is forbidden without explicit user consent, regardless of working mode (auto-accept, auto, plan, regular).

Note: Do NOT attempt to resolve PR review threads via the GitHub GraphQL API — the `resolveReviewThread` mutation is not supported by fine-grained PATs. The closed bead serves as the approval signal.

#### Closing the bead

```bash
br close <bead-id> --reason "Reviewed and approved in PR#<number>. All review feedback addressed."
git add .beads/issues.jsonl
git commit -m "Close <bead-id>: <short bead title>

All PR #<number> review feedback addressed.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
git push
```

## Posting Comments

When submitting GitHub PR reviews on the user's own PRs, always use event `COMMENT` (not `APPROVE` or `REQUEST_CHANGES`), as GitHub disallows approving or requesting changes on your own PRs.

Write a JSON file and pass it via `--input`. Do NOT use `-f` flags for reviews with inline comments — the nested `comments` array cannot be constructed with `-f`. Do NOT use the `pulls/{number}/comments` endpoint for individual comments — always use the reviews endpoint below.

The `line` field must be a line number present in the PR diff, not an absolute file line number.

```bash
cat > /tmp/review.json << 'EOF'
{
  "event": "COMMENT",
  "body": "Brief summary of review findings.",
  "comments": [
    {
      "path": "src/file.go",
      "line": 42,
      "body": "Short, explicit comment with code example if needed."
    }
  ]
}
EOF
gh api repos/{owner}/{repo}/pulls/{number}/reviews --method POST --input /tmp/review.json
```

- Each comment should be short, explicit, and aligned with the code
- Include a code example if the fix isn't obvious
- The summary comment should be brief and not duplicate inline comments
