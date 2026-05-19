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
| R12 | Never run `spex hash` to "fix" a non-empty diff | `feedback_spex_pipeline_not_hash` memory | Memory only | **CC hook** (PreToolUse Bash matcher; allow only if `SPEX_REBASELINE=1` marker is set, which the user sets manually when the TreeBuilder keying scheme changed) | `BLOCKED: spex-hash-bypasses-pipeline` |
| R13 | Pre-flight check at the top of every commit-producing skill: HEAD is not `main` | `feedback_check_branch_before_editing` memory + CLAUDE.md:37 | Memory only | **CC hook** (PreToolUse on Edit/Write/Bash-git-commit; reads `git rev-parse --abbrev-ref HEAD`) | `BLOCKED: editing-on-protected-branch` |

Layer key:
- **CC hook**: `PreToolUse` matcher in `.claude/settings.json` (`Bash` tool).
- **Git hook**: script under repo-tracked `scripts/git-hooks/`, installed via `git config core.hooksPath scripts/git-hooks`.
- **Skill pre-flight**: a check that runs at the *start* of a skill invocation. **Important:** plain text at the top of a SKILL.md file is *not* enforcement — it's the same strength as prose, since the model must read and act on it. A row labelled "Skill pre-flight" only counts as enforced once the check is wired as a real hook (UserPromptSubmit matching the slash-command, or PreToolUse wrapping the skill's first tool call). See §9.3.

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

Inventory of `/home/dev/.claude/projects/-workspace-spexmachina/memory/` as
of 2026-05-19. Each entry includes a one-line content summary, what's
already covered elsewhere, and a disposition. Not every memory belongs in
CLAUDE.md — some are domain knowledge, some are operational recovery
procedures, some are already redundant with current SKILL.md text.

### 7.1 Already redundant — delete

These memories state in their own body that they're already codified in
SKILL.md or are duplicated by existing prose:

- **`feedback_test_section_ownership_distinct_from_file_ownership.md`** —
  rule: test-section ownership is distinct from file ownership; bead
  scope is per `test_section`, not per source-file. The memory's own
  closing line says: *"This is now codified in
  `skills/implement/SKILL.md` (test-section scope guard within step 5)
  and `skills/review/SKILL.md` (cross-bead test scope as Step 3 check
  #8)."* → **Delete.** Verify the SKILL.md text is still present and
  authoritative, then drop the memory.

### 7.2 Already substantively covered by CLAUDE.md (delete or trim)

- **`feedback_check_branch_before_editing.md`** — rule: check current
  branch before any Edit/Write; branch from origin/main if on main.
  CLAUDE.md L37 already says *"main is protected: never commit directly
  to it. All changes land via a dedicated branch + PR merge, even
  one-line data fixes."* The memory adds the operational "check before
  the first edit" framing, which is what the incident actually needed.
  → **Enforce as R4 hook (CC PreToolUse on Edit/Write/Bash-git-commit
  that asserts HEAD ≠ main), then delete.** Once the hook exists the
  prose is sufficient.

### 7.3 Skill-scoped — move into individual SKILL.md files

Each of these is operational guidance for one specific skill. Belongs in
that skill's file, not in cross-cutting memory or CLAUDE.md:

- **`feedback_review_findings_block_dont_punt.md`** — rule: every real
  finding goes into the review as a blocker; never "non-blocker for
  follow-up." If a finding is named, verdict is `ISSUES`. → **Move into
  `/review` SKILL.md**, ideally as a numbered step in the verdict-mapping
  section.
- **`feedback_skills_no_autocommit.md`** — rule: `/spec` and `/converge`
  must not auto-commit; `/fix`, `/review`, `/cleanup`, `/implement` MUST
  commit and push (per their explicit contracts). → **Move per-skill: a
  one-line "Commits and pushes: yes / no" line at the top of each
  SKILL.md.** Once each skill is self-describing, the cross-cutting memory
  is redundant. The carve-out paragraph in the memory exists *because* the
  memory was overly broad; per-skill statements eliminate the need for it.
- **`feedback_spex_pipeline_not_hash.md`** — rule: never run `spex hash`
  to "fix" a non-empty diff; it bypasses impact/emit/ingest and orphans
  bead-map records. → **Move into `/converge` SKILL.md and `/review`
  SKILL.md** (the closing-bead context). Also enforceable as a CC hook
  blocking `spex hash` outside an explicit re-baseline marker — captured
  as R12 below.
- **`feedback_spex_pipeline_errors_are_signal.md`** — rule: spex pipeline
  errors are correctness signals; never bypass with manual workarounds
  (especially manual `br close`). → **Move into `/converge` SKILL.md.**
  Tie to R6 (br close outside /review) — manual br close to escape
  pipeline friction is exactly what R6 blocks.
- **`feedback_supersession_delete_create.md`** — rule: when a proposal
  splits/merges/reshapes modules, delete the old module entirely and
  create fresh nodes; don't rename or reuse IDs. → **Move into `/spec`
  SKILL.md** (mode-detection / restructure section). No enforcement path
  (semantic decision, not a syntactic command pattern).

### 7.4 General git/tracker operations — move into CLAUDE.md

These are short operational facts about `git` and `br` that any
contributor (model or human, on any machine) should know:

- **`feedback_branch_creation_avoid_inherited_upstream.md`** — gotcha:
  `git checkout -b <name> origin/main` sets origin/main as upstream and
  breaks bare `git push`. Use `git push -u origin <name>` on first push.
  → **Move into CLAUDE.md "Git Conventions"** as a one-line note under
  the existing "branch from origin/main" rule.
- **`feedback_br_list_filters_silently.md`** — gotcha: `br list --json`
  silently filters to open-only with `--limit 50`. For canonical state,
  use `jq -s '{issues: .}' .beads/issues.jsonl`. CLAUDE.md already says
  *"Use `br` commands or `.beads/issues.jsonl`"* (L62) but doesn't warn
  about the silent filter. → **Move into CLAUDE.md "Issue Tracking"** as
  a one-line note.
- **`feedback_beads_undo_requires_db_reset.md`** — recovery procedure:
  to undo a `br` mutation, checkout jsonl + delete db + re-import (db is
  source-of-truth, jsonl is a reflection). → **Move into CLAUDE.md
  "Bead Context Resolution"** under a new "Recovery procedures"
  subsection. This is the kind of thing that has to be discoverable
  without a session-bound memory.
- **`feedback_read_main_not_current_branch.md`** — rule: when answering
  "current state" questions, verify `HEAD` first; read from
  `origin/main` explicitly if not on main. → **Move into CLAUDE.md
  "Git Conventions"** as a short paragraph. Not enforceable mechanically
  (the model has to recognize the question shape) but should be a
  repo-visible rule.
- **`feedback_read_branch_commits_not_diff_stat.md`** — rule: for rebase
  risk, read `git log base..branch --oneline` and per-commit `git show
  --stat`; do not judge by `git diff base..branch --stat`. → **Move into
  CLAUDE.md "Git Conventions"** as a one-liner pointing to the right
  command.

### 7.5 Keep in memory — genuine domain knowledge or meta-guidance

These don't fit CLAUDE.md (too specialized, too meta, or no clean
enforcement path):

- **`feedback_merkle_test_section_keying.md`** — domain knowledge: a
  test_section's leaf hash covers only its markdown content;
  `describes`/`id`/`name` changes show up only in module meta. This is a
  property of `merkle/tree_builder.go:155-164`, valuable when reviewing
  describes-only PRs whose `spex diff` looks misleading. → **Keep in
  memory** *or* fold into `spec/merkle/arch_tree_builder.md` (the spec
  leaf where this property is implemented). The latter is better
  long-term: the rule lives next to the code it describes.
- **`feedback_dont_paraphrase_existing_rules.md`** — meta-rule: when
  asked to fix a recurring failure, look for the workflow gap, not the
  wording. Adding more prose to an already-existing rule is decorative.
  Hard to enforce mechanically. → **Keep in memory** for now. This RFC
  is in part an embodiment of this rule — it's the workflow-gap
  enforcement layer the memory advocates for. Once the RFC's hooks land,
  consider promoting this rule into a short "When to add a hook vs. a
  paragraph" note in CLAUDE.md.

### 7.6 Acceptance test for the migration

A fresh docker container with only `git clone` of this repo and no
`~/.claude/projects/...` state must produce the same agent behavior as
the current dev environment for every rule in §7.1–§7.4. Items in §7.5
explicitly do not need to survive the test; they are the residual
machine-local knowledge.

To verify: spin up a fresh container, run a representative task
(`/implement <bead>` or `/fix <pr>`) and check that the agent picks up
the rules now in CLAUDE.md / SKILL.md without consulting memory.
Anything that requires a memory lookup post-migration is a migration
gap.

## 8. Implementation roadmap

Out of scope for this RFC PR. Suggested order, riskiest gaps first:

1. **Git pre-push hook + `core.hooksPath` wiring** (R1/R2). One PR.
   Closes the "push to `main`" gap globally. Lowest coordination cost,
   no skill-context dependency.
2. **Git pre-commit + post-commit hooks for signing** (R5). One PR.
   Pre-commit verifies config; post-commit verifies the just-created
   commit has a `gpgsig` block (§9.2).
3. **CC hook: `.beads/beads.db` direct reads** (R7/R8). One PR. Pure
   syntactic block, no skill-context dependency.
4. **CC hook: deny `--no-gpg-sign` and `-c commit.gpgsign=false` flags**
   (R5 complement). One PR. Defense-in-depth alongside #2.
5. **CC hook: editing on protected branch** (R13). One PR. Reads
   `git rev-parse --abbrev-ref HEAD`; no skill-context dependency. This
   is the §7.2 "delete memory after enforcement" pickup.
6. **Resolve skill-context propagation** (§9.1). One PR with a small
   spike: investigate harness-native skill identity, fall back to the
   marker-file pattern if needed. This unblocks #7/#8/#9.
7. **CC hook: `br close` outside `/review`** (R6). Depends on #6.
8. **CC hook: skill-aware commit rules** (R9/R10). Depends on #6.
9. **CC hook: `spex hash` requires re-baseline marker** (R12). Depends
   on #6 (or stands alone if the marker is purely env-var-based, with
   no skill check needed — see implementation note in §9.1).
10. **Memory migration** (one PR, documentation-only). Per §7 table.
    Can land in parallel with hook work since it's text-only.
11. **`scripts/hook-violations-summary` helper** (one PR). Quality of
    life for tuning rules.

Each PR ships its own row(s) from the catalog, updates the row's
"Today's enforcement" column, and adds a `make verify-enforcement` case
for its rule. The RFC itself does not change as rows land — it stays a
design reference. The catalog can be promoted to its own
`docs/enforcement-catalog.md` once it has more than a few rows in
"enforced" state, with the RFC pointing to it.

## 9. Open questions and risks

### 9.1 Skill-context propagation (primary risk)

The catalog includes rules that depend on knowing which skill is active:
R6 (`br close` only from `/review`), R9/R10 (`/spec` and `/converge` do
not commit, others do), R12 (`spex hash` requires explicit re-baseline
marker). All three §6 reference implementations use
`SPEX_SKILL=<name>`, set by the skill itself, to disambiguate.

**The fragility:** a skill file is a markdown instruction to the model,
not a process boundary. "/review exports `SPEX_SKILL=review` at the top"
only works if the model runs that Bash command. If the model skips it,
the hook sees `SPEX_SKILL` unset, the rule falls back to "block all" (R6
case) or "allow all" (R9 case), and we have either a false-positive
block or — worse — a silent failure that lets the wrong skill close a
bead. This is the same model-dependency that motivates the RFC in the
first place.

**Options to investigate during implementation, in preference order:**

1. **Harness-native skill identity.** Investigate whether Claude Code
   exposes the active skill name to hooks (env var, JSON field on
   `tool_input`, etc.). If yes, use it; the harness is the authority and
   model compliance is not required.
2. **Marker file with TTL.** `/review` writes `.claude/skill-context.json`
   with `{skill: "review", started_at: <iso>, pid: <self>}` on entry.
   Hooks read it and refuse the operation if the file is missing, older
   than N minutes, or names a different skill. Forgetful failure becomes
   a hard block rather than a silent pass.
3. **Defense in depth.** Combine (2) with a fail-closed default: if the
   hook can't determine the skill context, it blocks (not allows). R6
   becomes "block `br close` unless we can positively verify
   skill=review." This is the safest default; the cost is more spurious
   blocks until the marker mechanism is reliable.

The current RFC text under §6 picks the env-var approach for
readability; the implementation PR for R6 must answer this question
before landing.

### 9.2 `--no-gpg-sign` flag bypass (R5 gap)

A git `pre-commit` hook can verify `commit.gpgsign=true` in config but
cannot catch `git commit --no-gpg-sign` or `git -c commit.gpgsign=false
commit`, because those produce an unsigned commit object that the hook
sees as "valid commit, signing config is fine globally." Two
complementary mitigations:

1. **CC hook denying the flags** in the Bash command string. Catches
   model-driven attempts but not direct user invocation.
2. **Post-commit hook** verifying the just-created commit has a `gpgsig`
   block via `git cat-file -p HEAD`. If absent, the hook errors
   loudly — the commit is already made, but the user has the slug to
   amend before pushing. Pair this with a pre-push hook that re-checks
   signing on every ref being pushed (last-line defense).

### 9.3 Skill pre-flight is not a real enforcement layer

The catalog has §6/§4 calling "skill pre-flight" a layer. As written
(text at the top of SKILL.md), it's the same enforcement strength as
prose: the model has to read it and act. To count as enforcement,
pre-flight checks must run as hooks (UserPromptSubmit on slash-command
invocation, or PreToolUse wrapping the skill's first tool call) — not
as markdown the model reads.

The implementation roadmap should treat "skill pre-flight" rows as
"unenforced until wired as a real hook." R3 and R4 should probably be
demoted to skill-pre-flight-as-hook in the catalog, with the explicit
note that they require harness wiring (see §9.1).

### 9.4 R3 (always git fetch) is overweight

Requiring a network fetch + SSH-SK tap before every branch creation is
painful. Two softer formulations to consider:

- **TTL fetch**: hook passes if `.git/FETCH_HEAD` mtime is within last N
  minutes (default 15). Avoids the tap on a fast iteration loop.
- **Skip on no-network**: hook passes if the network check itself fails
  fast (offline contexts).

Either is honest about what the rule is protecting against (working
against months-stale origin/main), without the punitive enforcement on
every branch.

### 9.5 Hook violation log rotation

At one line per block, log size is unlikely to matter for a year. Defer
until the log warrants it. If/when it does, add a `logrotate`-style
trim to `scripts/hook-violations-summary`.

### 9.6 Verification target

A `make verify-enforcement` target should simulate each blocked action
and assert the hook fires with the correct slug. Lives in the
implementation PR for each hook, not the RFC. Should be wired into
`make check` so regressions are caught locally before push.

### 9.7 Interaction with the auto-mode classifier

Once these hooks exist, the classifier may stop citing memory entries
because the hook is now the authority for the rules it covers. Worth
observing post-implementation; no design change is needed up front. If
the classifier continues to cite memory after a rule has migrated to a
hook, that's a signal the migration of that specific rule is
incomplete — the prose still exists somewhere it shouldn't.

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
