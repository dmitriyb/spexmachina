# RFC: Machine-enforced pipeline guardrails

**Status:** Draft
**Branch:** `rfc/enforcement-guardrails`
**Author:** Dmitrii Bozhko (with Claude)
**Date:** 2026-05-19

## 1. Context

Three failure modes have surfaced repeatedly in this repo and motivate this RFC:

**1.1 Skills skip pipeline steps despite CLAUDE.md prose.** An audit of the
repo found zero hooks in `.claude/settings.json` and zero active git hooks.
Every "Never / MUST NOT" rule in `CLAUDE.md` is currently prose-only: it
holds only as long as the model reads it correctly each turn. Specifically:

- `br close` is documented as `/review`-only (CLAUDE.md:49) but nothing
  prevents `/implement`, `/fix`, or `/cleanup` from running it.
- Direct reads of `.beads/beads.db` are forbidden (CLAUDE.md:62) but
  nothing blocks `sqlite3`, `python sqlite3`, or `cat` against the file.
- Commits to `main` are forbidden (CLAUDE.md:37) but the only check lives
  in `scripts/run-pipeline.sh:87`, which runs only when the pipeline does.
  Direct `git commit` / `git push` to `main` would not be caught.
- SSH-signed commits are mandatory (CLAUDE.md:40) but only `/implement`
  performs an `ssh-add -l` pre-flight; other skills do not.

**1.2 Memory is fragile across the actual working environment.** Memory
lives at `/home/dev/.claude/projects/-workspace-spexmachina/memory/`, which
is per-user-home. The user works across multiple machines via docker
containers; that path does not survive container rebuilds or machine
switches. Memory cannot be the durable home for rules. `CLAUDE.md` and
`.claude/settings.json` travel with the repo and are the correct home for
anything load-bearing.

The `feedback_skills_no_autocommit` incident in PR#169 made this concrete:
the auto-mode classifier cited a memory entry to block a `/fix` commit
that the skill's own contract authorized. The memory entry's scope
(originally written for `/spec` and `/converge`) had quietly drifted into
authority over a skill it was never meant to govern. A memory entry should
not be able to override a checked-in skill contract.

**1.3 Auto-mode classifier behavior is opaque.** The classifier is an
Anthropic-side guardrail; we cannot tune it directly. What we *can* do is
make the rules it observes (settings.json, CLAUDE.md, skill docs)
authoritative and consistent, so that when it blocks, the block is
correct, and when it allows, the action is authorized by something
durable.

**Intended outcome.** A strict-by-default, machine-enforced ruleset that
holds even when the model errs. Genuine edge cases halt the agent and
escalate to the user. The human is the only override path.

## 2. Goals & Non-Goals

**Goals**

- Move every load-bearing "Never / MUST NOT" rule from prose into a hook
  (Claude Code hook, git hook, or skill pre-flight, in that preference
  order).
- When a rule fires, the agent halts immediately, emits a structured
  recovery message, and waits for explicit user direction. No silent
  retries, no workarounds, no model-side bypass.
- Logged violations accumulate locally so frequent firings can later
  inform rule tuning.
- All enforcement lives in the repo (`.claude/settings.json`,
  `scripts/git-hooks/`, skill files), so it travels across docker
  containers and machines.

**Non-goals**

- No relaxation of existing rules. This RFC catalogs and enforces what is
  already documented.
- No new auto-recovery or silent retries.
- No work on the auto-mode classifier itself.
- Implementation lands in follow-up PRs; this RFC ships only the design.

## 3. Rule catalog

| # | Rule | Source | Today's enforcement | Target layer | Halt message stub |
|---|------|--------|---------------------|--------------|-------------------|
| R1 | Never commit directly to `main` | CLAUDE.md:37 | Prose; `run-pipeline.sh:87` checks branch (pipeline only) | **Git pre-commit + pre-push** | `BLOCKED: no-commits-to-main` |
| R2 | All changes land via branch + PR merge | CLAUDE.md:37 | Prose | **Git pre-push** (refuse `main` ref) | `BLOCKED: no-direct-push-to-main` |
| R3 | Always `git fetch origin` before creating a branch | CLAUDE.md:38 | Prose | **Skill pre-flight** in `/implement`, `/fix`, `/cleanup` | `BLOCKED: stale-origin` |
| R4 | Always branch from `origin/main`, not current branch | CLAUDE.md:39 | Prose | **Skill pre-flight** + CC hook on `git checkout -b` (best-effort) | `BLOCKED: branch-not-from-origin-main` |
| R5 | Commits must be SSH-signed; never bypass signing | CLAUDE.md:40 | `/implement` runs `ssh-add -l` | **Git pre-commit** verifying signing config + CC hook denying `--no-gpg-sign` / `-c commit.gpgsign=false` | `BLOCKED: unsigned-commit-attempt` |
| R6 | `br close` only from `/review` after LGTM | CLAUDE.md:49 | Prose | **CC hook** (PreToolUse Bash matcher; requires `SPEX_SKILL=review` env) | `BLOCKED: br-close-outside-review` |
| R7 | Never read `.beads/beads.db` directly | CLAUDE.md:62 | Prose | **CC hook** (PreToolUse Bash matcher) | `BLOCKED: beads-db-direct-read` |
| R8 | Never bypass `br` / `spex` to dig into storage | CLAUDE.md:63 | Prose | **CC hook** (same matcher as R7, extended) | `BLOCKED: tracker-storage-bypass` |
| R9 | `/spec` and `/converge` leave changes staged, do not commit | skill docs (`feedback_skills_no_autocommit`) | Prose + memory | **Skill pre-flight** + CC hook keyed on `SPEX_SKILL` env | `BLOCKED: skill-must-not-commit` |
| R10 | `/fix`, `/review`, `/cleanup`, `/implement` *must* commit and push | skill docs | Prose | **Inverse rule** — explicitly NOT blocked; R9 hook must check the skill name | n/a (a guard against false positives) |
| R11 | Never use `git rebase -i` / `git add -i` (interactive) | (operational, not in CLAUDE.md) | Prose | **CC hook** (PreToolUse Bash matcher) | `BLOCKED: interactive-git-not-supported` |

Layer key:
- **CC hook**: `PreToolUse` matcher in `.claude/settings.json` (`Bash` tool).
- **Git hook**: script under repo-tracked `scripts/git-hooks/`, installed via `git config core.hooksPath scripts/git-hooks`.
- **Skill pre-flight**: Bash check at the top of the skill file, exits non-zero with the structured halt message before any work begins.

Layer choice rationale:
- **Git hooks** are preferred for anything involving `git commit` or
  `git push` because they run regardless of who drove the action (model,
  user, another tool). Once `core.hooksPath` is set, the hooks are active
  in every clone.
- **CC hooks** are preferred for tool-shape rules where the violation is
  visible in the Bash command string (`br close`, `sqlite3 .beads/`,
  `--no-gpg-sign`). They block the model before the syscall.
- **Skill pre-flight** is the weakest layer; reserved for rules that are
  context-dependent (e.g., "fetched origin within this skill run") and
  cannot be expressed as a syntactic Bash match.

## 4. Halt-and-recover protocol

### 4.1 Hook output format

When any enforcement layer blocks an action, the hook MUST write the
following to its stdout / stderr and exit non-zero:

```
BLOCKED: <one-line rule slug, e.g. br-close-outside-review>

STATE: <what was attempted, including the exact command and CWD>
INVARIANT: <the rule, quoted from CLAUDE.md or skill docs, with source location like "CLAUDE.md:49">
POSSIBLE RECOVERY: <one or more canonical paths back to a valid state>

The agent MUST NOT retry this command, MUST NOT attempt a workaround, and
MUST surface this block to the user verbatim before proceeding. Recovery
requires explicit user direction.
```

The `POSSIBLE RECOVERY` block is **informational only** — it names valid
paths but does not authorize them. Authorization comes from the user.

### 4.2 Agent-side contract

A new section will be added to `CLAUDE.md` codifying agent behavior on a
`BLOCKED:` line:

1. The agent halts the current tool sequence. It does not call any tool
   on the same goal-path until the user has responded.
2. The agent surfaces the full structured message to the user verbatim
   (no paraphrasing — the slug and source location are load-bearing).
3. The agent proposes a recovery path drawn from `POSSIBLE RECOVERY`, but
   does not execute it.
4. Destructive recovery steps require explicit per-step confirmation:
   - `rm`, `rm -rf`, `git reset --hard`, `git checkout -- <file>`,
     `git clean -f`, `git branch -D`, force-push, `br delete`, any DB
     wipe, any operation that overwrites unstaged work.
   - The agent never bundles a destructive step with other steps.
5. Non-destructive recovery (file edits, `git add`, `git stash`,
   `git fetch`, `br update`, additional `Read`s) may proceed once the user
   acknowledges the block and names the path.

### 4.3 Why no override env var

Earlier drafts considered an override path (env var or marker file the
user could set when an exception is genuinely intended). The user has
explicitly chosen against this: the human IS the override. Edge cases are
unique enough that a generic override knob would either (a) bit-rot to
"always on" or (b) tempt the model to set it. Forcing every exception
through an interactive conversation is the desired property, not a cost.

## 5. Logging contract

Each hook firing appends a single JSON line to `.claude/hook-violations.log`:

```json
{"ts":"2026-05-19T07:14:00Z","rule":"no-commits-to-main","branch":"main","command":"git commit -m ...","cwd":"/workspace/spexmachina","skill":"fix"}
```

Fields:

- `ts` — ISO-8601 UTC.
- `rule` — the slug from the `BLOCKED:` line. Must match across hooks.
- `branch` — `git rev-parse --abbrev-ref HEAD` at the time of the block.
- `command` — the attempted Bash command (truncated to 500 chars).
- `cwd` — working directory.
- `skill` — value of `$SPEX_SKILL` if set, else `null`.

`.claude/hook-violations.log` is added to `.gitignore` (alongside the
existing `.spex/` and `.bv/` entries). The log is local-only by design:
it captures the user's specific workflow, not a global pattern.

A small helper `scripts/hook-violations-summary` (added in a later PR)
will print rule firing counts and recent commands. The goal is to make
"rule X fires 40 times a week, is it scoped correctly?" answerable in
seconds.

## 6. Reference hook implementations

These are worked examples for the three highest-value rules. They are
NOT implemented in this RFC — the code below is illustrative. Follow-up
PRs land each one separately.

### 6.1 CC hook: block `br close` outside `/review`

Addition to `.claude/settings.json`:

```jsonc
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "scripts/hooks/block-br-close-outside-review.sh"
          }
        ]
      }
    ]
  }
}
```

`scripts/hooks/block-br-close-outside-review.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Claude Code passes tool input as JSON on stdin.
input="$(cat)"
command="$(jq -r '.tool_input.command // empty' <<<"$input")"

if [[ "$command" =~ ^[[:space:]]*br[[:space:]]+close([[:space:]]|$) ]]; then
  if [[ "${SPEX_SKILL:-}" != "review" ]]; then
    rule="br-close-outside-review"
    cat >&2 <<EOF
BLOCKED: $rule

STATE: attempted '$command' with SPEX_SKILL='${SPEX_SKILL:-<unset>}'
INVARIANT: br close is only allowed from /review after LGTM (CLAUDE.md:49)
POSSIBLE RECOVERY:
  - If the PR is LGTM and ready to close, run /review on it; that skill sets SPEX_SKILL=review and is the only authorized closer.
  - If the bead should be closed manually for an exceptional reason, the user must invoke br close directly outside the agent context.

The agent MUST NOT retry this command, MUST NOT attempt a workaround,
and MUST surface this block to the user verbatim before proceeding.
EOF
    scripts/hooks/log-violation "$rule" "$command"
    exit 2
  fi
fi
exit 0
```

`/review`'s SKILL.md gains a one-line `export SPEX_SKILL=review` at the
top of its instructions so the hook can see the skill context.

### 6.2 CC hook: block direct reads of `.beads/beads.db`

`scripts/hooks/block-beads-db-direct-read.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
command="$(jq -r '.tool_input.command // empty' <<<"$input")"

# Match: sqlite3 .beads/beads.db, cat .beads/beads.db, head/tail/less/xxd
# .beads/beads.db, python -c 'import sqlite3 ... .beads/beads.db'.
if [[ "$command" =~ (sqlite3|cat|head|tail|less|xxd|python).*\.beads/beads\.db ]]; then
  rule="beads-db-direct-read"
  cat >&2 <<EOF
BLOCKED: $rule

STATE: attempted '$command'
INVARIANT: Never read .beads/beads.db directly (CLAUDE.md:62). Use br commands or .beads/issues.jsonl.
POSSIBLE RECOVERY:
  - For a single bead: br show <id> --json
  - For listings: br list / br ready (note: br list filters silently; use jq over .beads/issues.jsonl for canonical state)
  - For raw tracker dump: jq -s '{issues: .}' .beads/issues.jsonl

The agent MUST NOT retry this command, MUST NOT attempt a workaround,
and MUST surface this block to the user verbatim before proceeding.
EOF
  scripts/hooks/log-violation "$rule" "$command"
  exit 2
fi
exit 0
```

### 6.3 Git pre-push: block push to `main`

`scripts/git-hooks/pre-push`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Git passes refs being pushed on stdin: <local_ref> <local_sha> <remote_ref> <remote_sha>
while read -r local_ref local_sha remote_ref remote_sha; do
  if [[ "$remote_ref" == "refs/heads/main" ]]; then
    rule="no-direct-push-to-main"
    cat >&2 <<EOF
BLOCKED: $rule

STATE: attempted to push $local_ref to $remote_ref
INVARIANT: main is protected; all changes land via PR merge (CLAUDE.md:37)
POSSIBLE RECOVERY:
  - Push to a feature branch instead: git push -u origin <branch-name>
  - Open a PR with gh pr create
  - If this is a genuinely exceptional case (e.g., emergency hotfix), the user must temporarily disable core.hooksPath out-of-band — the agent must not.

The agent MUST NOT retry this command, MUST NOT attempt a workaround,
and MUST surface this block to the user verbatim before proceeding.
EOF
    scripts/hooks/log-violation "$rule" "git push to main"
    exit 1
  fi
done
exit 0
```

Installation, added to a new top-level `Makefile` (or `make setup`
target):

```make
.PHONY: setup-hooks
setup-hooks:
	git config core.hooksPath scripts/git-hooks
	chmod +x scripts/git-hooks/*
```

CLAUDE.md gains a one-line note under "Git Conventions":

> Run `make setup-hooks` once per clone to activate the repo-tracked
> git hooks. CI verifies `core.hooksPath` is set correctly.

## 7. Memory migration

Memory inventory at `/home/dev/.claude/projects/-workspace-spexmachina/memory/`:

| File | Type | Disposition | Target |
|------|------|-------------|--------|
| `feedback_beads_undo_requires_db_reset.md` | feedback | **Move** | CLAUDE.md "Bead Context Resolution" |
| `feedback_br_list_filters_silently.md` | feedback | **Move** | CLAUDE.md "Issue Tracking" |
| `feedback_branch_creation_avoid_inherited_upstream.md` | feedback | **Move** | CLAUDE.md "Git Conventions" |
| `feedback_check_branch_before_editing.md` | feedback | **Enforce + delete** | Skill pre-flight in `/implement`, `/fix`, `/cleanup`; delete memory once enforced |
| `feedback_dont_paraphrase_existing_rules.md` | feedback | **Move** | New section in CLAUDE.md "When fixing skills" |
| `feedback_merkle_test_section_keying.md` | feedback | **Keep** | Domain-specific, model-discipline rule with no enforcement path |
| `feedback_read_branch_commits_not_diff_stat.md` | feedback | **Move** | New section in CLAUDE.md "Working with branches" |
| `feedback_read_main_not_current_branch.md` | feedback | **Move** | CLAUDE.md "Git Conventions" (same section as branch-creation) |
| `feedback_review_findings_block_dont_punt.md` | feedback | **Move** | `/review` SKILL.md |
| `feedback_skills_no_autocommit.md` | feedback | **Move** | Each affected skill's SKILL.md (positive contract: "this skill commits / does not commit"); delete the cross-cutting memory once skill files are authoritative |
| `feedback_spex_pipeline_errors_are_signal.md` | feedback | **Move** | CLAUDE.md "Technical Constraints" or `/converge` SKILL.md |
| `feedback_spex_pipeline_not_hash.md` | feedback | **Move** | `/converge` SKILL.md (operational rule) |
| `feedback_supersession_delete_create.md` | feedback | **Move** | `/spec` SKILL.md |
| `feedback_test_section_ownership_distinct_from_file_ownership.md` | feedback | **Move** | `/implement` and `/review` SKILL.md |

Migration test: a fresh docker container with only `git clone` of this
repo (no `~/.claude/projects/...` state) must yield the same agent
behavior as the current dev environment. If a rule fails to fire after a
fresh clone, the migration of that rule is incomplete.

## 8. Implementation roadmap

Out of scope for this RFC PR. Suggested order, riskiest gaps first:

1. **Git pre-push hook + `core.hooksPath` wiring** (one PR). Closes the
   "push to `main`" gap globally. Lowest coordination cost.
2. **Git pre-commit hook** for SSH-signing verification (one PR). Pairs
   with #1; covers the signing rule.
3. **CC hook: `br close` outside `/review`** (one PR). Requires the
   `/review` skill to export `SPEX_SKILL=review`; landing them together.
4. **CC hook: `.beads/beads.db` direct reads** (one PR). Pure block, no
   skill changes.
5. **CC hook: `--no-gpg-sign` and `-c commit.gpgsign=false`** (one PR).
   Defense-in-depth alongside #2.
6. **Memory migration** (one PR, documentation-only). Per §7 table.
7. **Skill pre-flight checks** for R3, R4, R9 (one PR per skill).
8. **`scripts/hook-violations-summary` helper** (one PR). Quality-of-life
   tool for tuning rules.

Each PR ships its own row(s) from the catalog and updates the catalog
status. The RFC itself does not change as rows land.

## 9. Open questions

- **Skill context to hooks.** Setting `SPEX_SKILL=<name>` at the top of
  each SKILL.md works if skills are invoked via Bash (the env propagates).
  Confirm during #3 implementation; fallback is to inspect process
  ancestry or read from a `.claude/current-skill` marker file the skill
  writes on entry.
- **Hook violation log rotation.** At one line per block, unlikely to
  matter for a year. Defer until log size warrants.
- **CI verification.** Should CI fail if `core.hooksPath` is not set
  correctly on a contributor's clone? Probably no — CI can't see the
  setting. Better: a `make verify-enforcement` target that fakes each
  blocked action and asserts the hook fires, run as part of `make check`.
- **Interaction with auto-mode classifier.** Once these hooks exist, the
  classifier may stop citing memory entries (because the hook IS the
  authority). Worth observing post-implementation; no design change
  needed up front.

## 10. Acceptance criteria for the RFC

This RFC is complete when the following hold:

- §3 catalog covers every "Never / MUST NOT" rule grep'd from `CLAUDE.md`
  and from `skills/*/SKILL.md`.
- §4 halt-and-recover protocol is unambiguous: a follow-up implementer
  writing a new hook needs no clarifying questions about output format
  or agent behavior.
- §5 logging schema is exact: rule slug, fields, file path.
- §6 reference implementations are runnable as-is (modulo `chmod +x`).
- §7 memory inventory matches `ls /home/dev/.claude/projects/-workspace-spexmachina/memory/`
  at the time of the PR.
