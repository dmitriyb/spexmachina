---
name: fix
description: Fix review comments on a pull request
disable-model-invocation: true
argument-hint: <pr-number>
---

**Commits and pushes:** yes. This skill commits each set of fixes and pushes. Enforcement hook `check-skill-commit-allowed.sh` permits `git commit` when the active skill is `fix`.

## Step 0: Declare skill identity to enforcement hooks

Before any other action, run this command verbatim so the hook layer knows the active skill (see CLAUDE.md "## Enforcement"):

```bash
mkdir -p .claude && printf '{"skill":"fix","started_at":"%s","pid":%d}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > .claude/skill-context.json
```

Read PR #$ARGUMENTS feedback from BOTH sources below, fix each item, commit and push, and provide a concise short response like "Fixed" or "Addressed", or answer in more detail if it is a question. Reply to EACH comment individually on GitHub. Do NOT post a single bulk comment summarizing all changes.

## Sources of review feedback

You MUST check both sources — they contain different feedback:

1. **Inline comments** — line-level comments on specific code:
   ```bash
   gh api repos/{owner}/{repo}/pulls/{pr_number}/comments
   ```
   Reply to each inline comment using:
   ```bash
   gh api repos/{owner}/{repo}/pulls/{pr_number}/comments/{comment_id}/replies -f body="Fixed"
   ```

2. **Review comments** — top-level review bodies (may contain items not covered by inline comments):
   ```bash
   gh api repos/{owner}/{repo}/pulls/{pr_number}/reviews
   ```
   For each review with actionable feedback in its body, post an issue comment addressing the items:
   ```bash
   gh api repos/{owner}/{repo}/issues/{pr_number}/comments -f body="Fixed — <description>"
   ```
