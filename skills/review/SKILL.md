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
4. If no spec labels exist, fall back to reading any spec references in the description

## Review Flow

This skill supports an iterative cycle: `implement → [review → fix → review] → close`.

### Step 0: Resolve repo slug

Run `gh repo view --json owner,name --jq '.owner.login + "/" + .name'` to get the `{owner}/{repo}` slug. Use this resolved value in all subsequent `gh api` calls. Do NOT guess the owner from the git remote or working directory name.

### Step 1: Check for prior reviews

Check **both** sources of review feedback:

1. **Review-level comments**: `gh api repos/{owner}/{repo}/pulls/{number}/reviews` — look for reviews with `state` = `COMMENTED`/`CHANGES_REQUESTED` and a non-empty `body`.
2. **Inline comments**: `gh api repos/{owner}/{repo}/pulls/{number}/comments` — line-level comments on the diff.

A prior review exists if **either** source has feedback.

- **No prior feedback from either source** → first review. Proceed to Step 2.
- **Prior feedback exists but unaddressed** → **Stop. Do nothing.** Tell the user.
- **Prior feedback exists and addressed** → proceed to Step 3 (Follow-up Review).

How to determine if feedback is "addressed":

- **Inline comments**: a top-level comment (no `in_reply_to_id`) is addressed if at least one reply references its `id`.
- **Review-body comments**: addressed if at least one commit exists **after** the review's `submitted_at` timestamp. Check with `gh api repos/{owner}/{repo}/pulls/{number}/commits` and compare dates.

### Step 2: First Review

1. Read the PR description and linked bead
2. Read the full diff
3. Check spec traceability: does the code implement what the bead requires?
4. Check test quality: do tests verify requirements, not just coverage?
5. Check code quality: correctness, error handling, existing patterns followed
6. Post review with inline comments (see Posting Comments below)

### Step 3: Follow-up Review

Collect all feedback items from both sources (review bodies and inline comments). For each item:

1. Read the original feedback to understand what was requested
2. For inline comments: read the reply. For review-body items: identify the post-review commit(s).
3. Read the **current code** (not just the diff) to verify the fix
4. Classify each item as **fixed** or **not fixed**

Then decide:

- **All fixed** → close the linked bead (`br close <id>`), commit the bead closure, and tell the user the PR is ready to merge.
- **Some not fixed** → for each unfixed item, post a new reply on that comment thread explaining what's still wrong. Do NOT re-review already-fixed items.

Note: Do NOT attempt to resolve PR review threads via the GitHub GraphQL API — the `resolveReviewThread` mutation is not supported by fine-grained PATs. The closed bead serves as the approval signal.

#### Closing the bead and committing

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

## What to Check

- **Spec traceability**: code maps to bead requirements, no unrelated changes
- **Correctness**: error paths handled, edge cases, no resource leaks
- **Patterns**: follows existing conventions, idiomatic Go
- **Tests**: verify requirements not implementation details, failure cases tested
