---
name: review
description: Review a pull request for correctness, spec traceability, and test quality
disable-model-invocation: true
argument-hint: <pr-number>
---

Review PR #$ARGUMENTS. Use @~/.claude/skills/go-expert/SKILL.md for Go-specific review guidance.

## Context Loading

Read ONLY these documents:

1. The PR diff and description
2. The linked bead: run `bd show <bead-id> --json` using the bead ID from the PR description
3. If the bead references spec nodes, read the relevant `spec/<module>_reqs.md` and `spec/<module>_impl.md`

## Review Process

1. Read the PR description and linked bead
2. Read the full diff
3. Check spec traceability: does the code implement what the bead requires?
4. Check test quality: do tests verify requirements, not just coverage?
5. Check code quality: correctness, error handling, existing patterns followed
6. Post review with inline comments

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
