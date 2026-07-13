# Enforcement migration

This repo previously shipped machine-enforced rules via **Claude Code hooks** (`.claude/settings.json` + `scripts/hooks/`) and **git hooks** (`scripts/git-hooks/`), designed in the former `enforcement-rfc.md`. That model was procedural (it gated each tool call inside one harness, mid-flight) and coupled to interactive Claude Code sessions.

Enforcement is now **external and result-based**: **portitor** (a git gateway) verifies the *result* of a push — signature → signer role → branch/ancestry/content rules — before anything reaches GitHub, and holds the only GitHub credential. **faber** orchestrates the autonomous implement/review/fix boxes. Interactive human sessions are the trusted design/authoring entrypoint and push to GitHub directly; the gate constrains the autonomous agents.

All in-repo hooks and the execution lifecycle skills (`/implement`, `/review`, `/fix`, `/cleanup`) have been removed. The authoring skills (`/propose`, `/spec`, `/spec-review`) remain, without hook frontmatter.

## Rule disposition

| Rule (old) | Was | Now |
|---|---|---|
| R1 — no direct commits to `main` | git pre-commit | portitor `no-push-to-default` |
| R2 — changes land via branch + PR | prose | portitor forwards accepted feature branches + auto-opens a PR |
| R3 — `origin/main` fetched < 15 min before branching | CC hook | retired (procedural; only served interactive sessions) |
| R4 — branch from `origin/main` | CC hook | retired; portitor `require_up_to_date_with_default` covers stale bases |
| R5a — commits SSH-signed | git post-commit / pre-push | portitor `unsigned-or-untrusted-commit` (per-role signing keys; identity is cryptographic) |
| R5b — deny `--no-gpg-sign` / `commit.gpgsign=false` | CC hook | retired (moot — portitor rejects any unsigned/untrusted commit regardless of how it was produced) |
| R6 — `br close` only after review | skill-scoped CC hook | portitor content rule on `.beads/issues.jsonl` (adding `"status":"closed"` requires the `reviewer`/`owner` role) + the gateway's compiled action policy (merge/close = `merger`/`owner`) |
| R7 / R8 — never read `.beads/beads.db` or `br` state files | CC hook | norm in `CLAUDE.md` (the db is a rebuildable reflection of `issues.jsonl`; a working-tree read is not visible to a push gate) |
| R9 — authoring skills must not commit | skill-scoped CC hook | retired with the manual skill lifecycle |
| R10 — execution skills must commit | prose | retired with the manual skill lifecycle |
| R11 — no interactive `git rebase -i` / `git add -i` | CC hook | retired (boxes are headless) |
| R13 — no Edit/Write/commit when `HEAD == main` | CC hook | retired; portitor rejects any push to `main` |
| R14 — one skill per session | skill-scoped CC hook | retired; a faber box runs exactly one skill by construction |
| R15 — commit messages ≤ two sentences | git commit-msg | soft norm in `CLAUDE.md` / box prompt (portitor content rules inspect path diffs, not commit messages) |

The two rules portitor cannot express as content rules today are R15 (commit-message shape — rules see path diffs, not the message) and a hard block on `.beads/beads.db` (binary; the file is untracked and rebuildable anyway). Both are handled as norms.
