---
name: fix
description: Fix review comments on a pull request
disable-model-invocation: true
argument-hint: <pr-number>
---

Read PR #$ARGUMENTS comments, fix each comment, commit and push it, and provide a concise short response like "Fixed" or "Addressed", or answer in more detail if it is a question. Reply to EACH comment individually on GitHub. Do NOT post a single bulk comment summarizing all changes.

Reply to each review comment using this endpoint (note: PR number is required in the path):

```bash
gh api repos/{owner}/{repo}/pulls/{pr_number}/comments/{comment_id}/replies -f body="Fixed"
```
