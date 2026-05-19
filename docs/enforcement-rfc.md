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

- Move every load-bearing "Never / MUST NOT" rule from prose into a
  machine-enforced hook — Claude Code hook (PreToolUse running a script)
  or git hook (pre-commit, post-commit, pre-push). No "skill pre-flight"
  layer; markdown at the top of a SKILL.md file is prose, not
  enforcement (see §9.3).
- When a rule fires, the hook returns a structured JSON payload per
  §4.1 and the agent halts immediately. The agent does not retry, does
  not work around the block, and surfaces the structured payload to the
  user. Recovery requires explicit user direction.
- Logged violations accumulate locally (`.claude/hook-violations.log`,
  gitignored) using the same JSON schema so `jq` is the only tool
  needed to mine them.
- All enforcement lives in the repo (`.claude/settings.json`,
  `scripts/hooks/`, `scripts/git-hooks/`, `Makefile`), so it travels
  across docker containers and machines.

**Non-goals**

- No relaxation of existing rules. This RFC catalogs and enforces what
  is already documented.
- No new auto-recovery or silent retries. The recovery path is always
  through the user.
- No work on the auto-mode classifier itself (opaque to us).
- The RFC itself ships only design. The implementation lands as a
  single PR with structured commits per §8 — not fragmented across
  many PRs, since the rules are interdependent and half-installed
  enforcement is worse than uninstalled.

## 3. Rule catalog

| # | Rule | Source | Today's enforcement | Target layer | Hook script | Rule slug |
|---|------|--------|---------------------|--------------|-------------|-----------|
| R1 | Never commit directly to `main` | CLAUDE.md:37 | Prose; `run-pipeline.sh:87` checks branch (pipeline only) | **Git pre-commit** | `scripts/git-hooks/pre-commit` | `no-commits-to-main` |
| R2 | All changes land via branch + PR merge | CLAUDE.md:37 | Prose | **Git pre-push** | `scripts/git-hooks/pre-push` | `no-direct-push-to-main` |
| R3 | `origin/main` must be fetched within last 15 min before branch creation | CLAUDE.md:38 | Prose | **CC hook** (PreToolUse Bash on `git checkout -b *` / `git switch -c *`) | `scripts/hooks/check-fetched-recent.sh` | `stale-origin` |
| R4 | New branches must be created from `origin/main` | CLAUDE.md:39 | Prose | **CC hook** (PreToolUse Bash on `git checkout -b *` / `git switch -c *`) | `scripts/hooks/check-branch-from-origin-main.sh` | `branch-not-from-origin-main` |
| R5a | Commits must have `gpgsig` block (verify post-commit) | CLAUDE.md:40 | `/implement` runs `ssh-add -l` | **Git post-commit + pre-push** | `scripts/git-hooks/post-commit`, `pre-push` | `unsigned-commit-detected` |
| R5b | Deny `--no-gpg-sign` and `-c commit.gpgsign=false` flags | CLAUDE.md:40 | Prose | **CC hook** (PreToolUse Bash matcher) | `scripts/hooks/block-signing-bypass.sh` | `signing-flag-denied` |
| R6 | `br close` only from `/review` after LGTM | CLAUDE.md:49 | Prose | **CC hook** (PreToolUse Bash on `br close *`) | `scripts/hooks/check-br-close-skill.sh` | `br-close-outside-review` |
| R7 | Never read `.beads/beads.db` directly | CLAUDE.md:62 | Prose | **CC hook** (PreToolUse Bash, Read tool) | `scripts/hooks/block-beads-db-read.sh` | `beads-db-direct-read` |
| R8 | Never bypass `br` / `spex` to dig into storage | CLAUDE.md:63 | Prose | **CC hook** (same matcher as R7, extended) | `scripts/hooks/block-tracker-bypass.sh` | `tracker-storage-bypass` |
| R9 | `/spec` and `/converge` must not commit | `feedback_skills_no_autocommit` | Prose + memory | **CC hook** (PreToolUse Bash on `git commit *`) | `scripts/hooks/check-skill-commit-allowed.sh` | `skill-must-not-commit` |
| R10 | `/fix`, `/review`, `/cleanup`, `/implement` *must* commit and push | skill docs | Prose | **Inverse guard** inside R9's script — never block these skills | (same as R9) | n/a |
| R11 | Never use `git rebase -i` / `git add -i` (interactive) | operational | Prose | **CC hook** (PreToolUse Bash matcher) | `scripts/hooks/block-interactive-git.sh` | `interactive-git-not-supported` |
| R12 | Never run `spex hash` outside an explicit re-baseline | `feedback_spex_pipeline_not_hash` | Memory only | **CC hook** (PreToolUse Bash on `spex hash *`; allow only if `SPEX_REBASELINE=1`) | `scripts/hooks/check-spex-hash-rebaseline.sh` | `spex-hash-bypasses-pipeline` |
| R13 | Edit/Write/commit when `HEAD == main` | `feedback_check_branch_before_editing` + CLAUDE.md:37 | Memory only | **CC hook** (PreToolUse on Edit, Write, Bash on `git commit *`) | `scripts/hooks/check-not-on-main.sh` | `editing-on-protected-branch` |

Layer key:
- **CC hook**: `PreToolUse` matcher in `.claude/settings.json`, executes a script under `scripts/hooks/`. The script returns structured JSON per §4.1 and exits non-zero to block.
- **Git hook**: script under repo-tracked `scripts/git-hooks/`, installed via `git config core.hooksPath scripts/git-hooks` (one-time `make setup-hooks`).

**No "skill pre-flight" layer.** An earlier draft included it for rules
like R3, R4, R9. We removed the layer entirely: a check that's only text
at the top of a SKILL.md file is prose, not enforcement. Every row in
the catalog now maps to a CC hook or git hook that runs without
model cooperation. See §9.3 for the reasoning trail.

Layer choice rationale:
- **Git hooks** for anything involving `git commit` or `git push` — they
  run regardless of who drove the action (model, user, CI, another
  tool). Once `core.hooksPath` is set, hooks are active in every clone.
- **CC hooks** for tool-shape rules where the violation is visible in
  the Bash command string or tool input (`br close`, `sqlite3 .beads/`,
  `--no-gpg-sign`, Edit on main). They block the model before the
  syscall.

Note: rules R3, R4, R6, R9, R12, R13 are all CC hooks that consult some
form of *context* (recent fetch time, branch name, active skill,
re-baseline marker, HEAD). The "active skill" piece is the only piece
that depends on the model setting state correctly — see §9.1 for the
implementation options. Everything else reads state directly from the
filesystem or git, with no model-mediated input.

## 4. Halt-and-recover protocol

### 4.1 Hook output format (structured JSON)

When any enforcement layer blocks an action, the hook script MUST write
a single line of JSON conforming to the `spex-halt/v1` schema below to
**stdout**, then exit non-zero. No free text, no section-titled prose:
the protocol is machine-readable so the agent doesn't have to parse
sentences to know what to do.

Schema (`spex-halt/v1`):

```json
{
  "protocol": "spex-halt/v1",
  "rule": "br-close-outside-review",
  "command": "br close spexmachina-1dj1",
  "cwd": "/workspace/spexmachina",
  "head": "feat/phb-4-snapshot-load-empty-tree",
  "skill": null,
  "invariant": "br close is only allowed from /review after LGTM",
  "source": "CLAUDE.md:49",
  "recovery": [
    {"path": "Run /review on the PR; that skill is the only authorized closer", "destructive": false},
    {"path": "User invokes br close directly outside the agent context", "destructive": false}
  ],
  "directive": "halt"
}
```

Field contract:

- `protocol` — exact string `spex-halt/v1`. Agents detect the halt by
  this marker. Any future schema bump uses a new version string and
  both must be supported during a transition window.
- `rule` — slug from the catalog "Rule slug" column. Stable identifier
  for logging and tooling.
- `command` — the attempted shell command (or for Edit/Write hooks, the
  target path). Truncated to 500 chars.
- `cwd` — `pwd` at the time of the block.
- `head` — `git rev-parse --abbrev-ref HEAD` at the time of the block.
- `skill` — the active skill name if known (see §9.1); else `null`.
- `invariant` — single-sentence statement of the rule the agent
  violated. Plain prose for human reading.
- `source` — file path + line where the rule is defined. Load-bearing
  for verification (the agent or user can read the cited line).
- `recovery` — array of `{path: string, destructive: bool}` objects.
  Each entry is one canonical way back to a valid state.
  `destructive: true` flags steps that overwrite unstaged work, force
  pushes, deletes, db wipes — those require per-step user
  confirmation per §4.2.
- `directive` — currently always `halt`. Future protocol versions may
  introduce other directives (warn, ask, etc.) but v1 is halt-only.

The `recovery` array is **informational only**. It names valid paths,
not authorization. Authorization comes from the user.

A reference helper `scripts/hooks/lib/emit-halt.sh` MUST be used by
every hook script to produce a conforming payload, so the schema can
evolve without rewriting every script:

```bash
# scripts/hooks/lib/emit-halt.sh
emit_halt() {
  local rule="$1" command="$2" invariant="$3" source="$4"; shift 4
  # remaining args are recovery items, alternating: "<text>" "<destructive>"
  jq -n \
    --arg protocol "spex-halt/v1" \
    --arg rule "$rule" \
    --arg command "$command" \
    --arg cwd "$PWD" \
    --arg head "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '')" \
    --arg skill "${SPEX_SKILL:-}" \
    --arg invariant "$invariant" \
    --arg source "$source" \
    --argjson recovery "$(build_recovery_array "$@")" \
    --arg directive "halt" \
    '{protocol:$protocol,rule:$rule,command:$command,cwd:$cwd,head:$head,
      skill:(if $skill=="" then null else $skill end),
      invariant:$invariant,source:$source,recovery:$recovery,directive:$directive}'
}
```

### 4.2 Agent-side contract

A new section is added to CLAUDE.md codifying agent behavior on a hook
response containing `"protocol": "spex-halt/v1"`:

1. The agent halts the current tool sequence. No further tools on the
   same goal-path until the user has responded.
2. The agent renders the payload to the user. The rendering MUST
   include verbatim: the `rule` slug, the `invariant`, the `source`,
   and every entry in `recovery`. The agent MAY add a one-line plain
   English summary on top.
3. The agent proposes a recovery path drawn from `recovery`, but does
   not execute it.
4. Destructive recovery steps (entries where `destructive: true`)
   require explicit per-step user confirmation. The agent never bundles
   a destructive step with other steps. The destructive list at the
   syntactic level is also enumerated in CLAUDE.md as a backstop:
   `rm`, `rm -rf`, `git reset --hard`, `git checkout -- <file>`,
   `git clean -f`, `git branch -D`, force-push, `br delete`, any DB
   wipe, any operation that overwrites unstaged work.
5. Non-destructive recovery (file edits, `git add`, `git stash`,
   `git fetch`, `br update`, additional `Read`s) may proceed once the
   user acknowledges the block and names the path.

### 4.3 Why no override env var

Earlier drafts considered an override env var or marker file. The user
has explicitly chosen against it: the human IS the override. A generic
override knob would either (a) bit-rot to "always on" or (b) tempt the
model to set it. Forcing every exception through an interactive
conversation is the desired property, not a cost.

The one exception is R12's `SPEX_REBASELINE=1` marker for `spex hash`,
which is narrow (single rule, single command) and exists because the
re-baseline operation is *legitimate* in specific circumstances (the
TreeBuilder keying scheme changed). It is not a general bypass.

## 5. Logging contract

Each hook firing appends a single JSON line to
`.claude/hook-violations.log`. The log line is the §4.1 halt payload
plus a `ts` field — same schema, no duplication, no separate format to
maintain:

```json
{"ts":"2026-05-19T07:14:00Z","protocol":"spex-halt/v1","rule":"no-commits-to-main","command":"git commit -m ...","cwd":"/workspace/spexmachina","head":"main","skill":"fix","invariant":"...","source":"CLAUDE.md:37","recovery":[...],"directive":"halt"}
```

`.claude/hook-violations.log` is added to `.gitignore`. The log is
local-only by design: it captures the user's specific workflow, not a
global pattern.

`scripts/hook-violations-summary` (delivered in the implementation PR)
prints rule firing counts and recent commands, with `jq` doing all the
filtering since every line is a well-formed JSON object. Goal: "rule X
fires 40 times a week, is it scoped correctly?" is answerable in
seconds.

## 6. Reference hook implementations

Worked examples for the three highest-value rules. Code is illustrative
and uses the §4.1 JSON halt format via the `emit_halt` helper from
`scripts/hooks/lib/emit-halt.sh`. The implementation PR (§8) lands all
the scripts together.

### 6.1 CC hook: block `br close` outside `/review`

`.claude/settings.json`:

```jsonc
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash",
        "hooks": [{ "type": "command",
                    "command": "scripts/hooks/check-br-close-skill.sh" }] }
    ]
  }
}
```

`scripts/hooks/check-br-close-skill.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
command="$(jq -r '.tool_input.command // empty' <<<"$input")"

if [[ "$command" =~ ^[[:space:]]*br[[:space:]]+close([[:space:]]|$) ]]; then
  if [[ "${SPEX_SKILL:-}" != "review" ]]; then
    payload="$(emit_halt \
      "br-close-outside-review" \
      "$command" \
      "br close is only allowed from /review after LGTM" \
      "CLAUDE.md:49" \
      "Run /review on the PR; that skill is the only authorized closer" false \
      "User invokes br close directly outside the agent context" false)"
    printf '%s\n' "$payload"
    printf '%s\n' "$payload" | scripts/hooks/log-violation
    exit 2
  fi
fi
exit 0
```

`/review` declares `SPEX_SKILL=review` via the skill-context propagation
mechanism chosen in §9.1.

### 6.2 CC hook: block direct reads of `.beads/beads.db`

`scripts/hooks/block-beads-db-read.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"

case "$tool" in
  Bash)
    target="$(jq -r '.tool_input.command // empty' <<<"$input")"
    if [[ ! "$target" =~ (sqlite3|cat|head|tail|less|xxd|python).*\.beads/beads\.db ]]; then
      exit 0
    fi
    ;;
  Read)
    target="$(jq -r '.tool_input.file_path // empty' <<<"$input")"
    if [[ "$target" != *".beads/beads.db" ]]; then
      exit 0
    fi
    ;;
  *) exit 0 ;;
esac

payload="$(emit_halt \
  "beads-db-direct-read" \
  "$target" \
  "Never read .beads/beads.db directly. Use br commands or .beads/issues.jsonl." \
  "CLAUDE.md:62" \
  "For a single bead: br show <id> --json" false \
  "For listings: br list --all --limit 0 (the bare br list filters silently — open-only, limit 50)" false \
  "For raw tracker dump: jq -s '{issues: .}' .beads/issues.jsonl" false)"
printf '%s\n' "$payload"
printf '%s\n' "$payload" | scripts/hooks/log-violation
exit 2
```

Note: this hook matches both `Bash` (to catch `sqlite3 .beads/beads.db`)
and `Read` (to catch the model trying to read the file directly via the
Read tool). The CC matcher list in `.claude/settings.json` enumerates
both.

### 6.3 Git pre-push: block push to `main`

`scripts/git-hooks/pre-push`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(git rev-parse --show-toplevel)/scripts/hooks/lib/emit-halt.sh"

# stdin: <local_ref> <local_sha> <remote_ref> <remote_sha>
while read -r local_ref local_sha remote_ref remote_sha; do
  if [[ "$remote_ref" == "refs/heads/main" ]]; then
    payload="$(emit_halt \
      "no-direct-push-to-main" \
      "git push: $local_ref -> $remote_ref" \
      "main is protected; all changes land via PR merge" \
      "CLAUDE.md:37" \
      "Push to a feature branch: git push -u origin <branch-name>" false \
      "Open a PR: gh pr create" false \
      "If genuinely exceptional (emergency hotfix), user disables core.hooksPath out-of-band" true)"
    printf '%s\n' "$payload"
    printf '%s\n' "$payload" | "$(git rev-parse --show-toplevel)/scripts/hooks/log-violation"
    exit 1
  fi
done
exit 0
```

### 6.4 Installation and the agent contract

`Makefile`:

```make
.PHONY: setup-hooks verify-enforcement
setup-hooks:
	git config core.hooksPath scripts/git-hooks
	chmod +x scripts/git-hooks/* scripts/hooks/*.sh
verify-enforcement:
	scripts/hooks/test/run-all.sh
```

`CLAUDE.md` gains a "Enforcement" section pointing at this RFC and
codifying:
- Run `make setup-hooks` once per clone.
- On any hook response containing `"protocol": "spex-halt/v1"`,
  follow the §4.2 agent-side contract.
- Run `make verify-enforcement` before opening a PR that touches
  `.claude/settings.json`, `scripts/hooks/`, or `scripts/git-hooks/`.

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

**Single PR, structured commits.** This is interdependent work — half-installed
enforcement is worse than uninstalled (some rules fire, others silently pass,
and neither model nor user can reason about which is which). The §7.6 fresh-container
acceptance test also only works if the migration is atomic. One review cycle,
one merge, one verification.

The PR ships the work in this commit order. Each commit is independently
buildable and tested. The commit boundaries are essential checkpoints,
not arbitrary slices:

### Commit 1 — Foundation: halt protocol + violation log + Makefile

- `scripts/hooks/lib/emit-halt.sh` — the `emit_halt` helper that every
  hook calls. Implements the §4.1 JSON schema. Single source of truth.
- `scripts/hooks/log-violation` — stdin → append-to-log helper.
- `scripts/hooks/test/run-all.sh` — test harness skeleton (one
  fixture per rule will be added in later commits).
- `Makefile` with `setup-hooks` and `verify-enforcement` targets.
- `.gitignore` adds `.claude/hook-violations.log`.

Outcome: infrastructure exists, no rules enforced yet.

### Commit 2 — Git hooks: protect main + verify signing

- `scripts/git-hooks/pre-commit` — R1 (block commit on main) + R5a
  pre-commit half (verify `commit.gpgsign=true`).
- `scripts/git-hooks/post-commit` — R5a post-commit half (verify
  `gpgsig` block present in HEAD).
- `scripts/git-hooks/pre-push` — R2 (refuse push to `main`) +
  re-verification of R5a on each ref being pushed.
- `scripts/hooks/test/test-git-hooks.sh` — fixtures: commit on main,
  push to main, `--no-gpg-sign` commit, all expected to block.

Outcome: R1, R2, R5a enforced.

### Commit 3 — CC hooks: syntactic blocks (no skill context needed)

- `scripts/hooks/block-signing-bypass.sh` — R5b (deny `--no-gpg-sign`,
  `-c commit.gpgsign=false`).
- `scripts/hooks/block-beads-db-read.sh` — R7 + R8 (block `sqlite3`,
  `cat`, `Read` of `.beads/beads.db`).
- `scripts/hooks/block-interactive-git.sh` — R11 (block `git rebase -i`,
  `git add -i`).
- `scripts/hooks/check-not-on-main.sh` — R13 (block Edit/Write/git-commit
  when HEAD = main).
- `scripts/hooks/check-fetched-recent.sh` — R3 (`origin/main` fetched
  within 15 min before branch creation).
- `scripts/hooks/check-branch-from-origin-main.sh` — R4 (new branches
  must come from `origin/main`).
- `.claude/settings.json` wires all PreToolUse matchers.
- Test fixtures for each.

Outcome: R3, R4, R5b, R7, R8, R11, R13 enforced.

### Commit 4 — Resolve skill-context propagation (§9.1 spike)

- Investigate harness-native skill identity. Land whichever of the §9.1
  options is feasible:
  1. If Claude Code exposes the active skill to hooks (env var, JSON
     field), use it.
  2. Else, implement the marker-file pattern:
     `.claude/skill-context.json` written by each skill on entry, read
     by hooks with TTL check.
- `scripts/hooks/lib/active-skill.sh` — single helper every
  skill-aware hook calls. Returns the skill name or `""`.
- Update each skill's SKILL.md to wire its identity to the helper (one
  line per skill, depending on option chosen).

Outcome: skill identity is observable to hooks. No rules enforced *yet*
that depend on it.

### Commit 5 — CC hooks: skill-aware blocks

- `scripts/hooks/check-br-close-skill.sh` — R6 (depends on Commit 4).
- `scripts/hooks/check-skill-commit-allowed.sh` — R9 + R10 (block
  `/spec`, `/converge` from committing; allow `/fix`, `/review`,
  `/cleanup`, `/implement`).
- `scripts/hooks/check-spex-hash-rebaseline.sh` — R12 (block `spex hash`
  unless `SPEX_REBASELINE=1`). No skill check needed; included here
  because it's the last CC hook.
- `.claude/settings.json` adds the new matchers.
- Test fixtures: `br close` from each skill, `git commit` from each
  skill, `spex hash` with/without marker.

Outcome: R6, R9, R10, R12 enforced. **All 13 rows of the catalog are
now machine-enforced.**

### Commit 6 — Memory migration

- Delete memory entries in §7.1 and §7.2.
- Move §7.3 entries into their target SKILL.md files (one paragraph
  each).
- Move §7.4 entries into CLAUDE.md as one-liners under the right
  section.
- Verify §7.5 entries are either retained in memory or moved into
  `spec/merkle/arch_tree_builder.md` per the disposition.

Outcome: memory now holds only ephemeral observations. CLAUDE.md and
SKILL.md files are self-sufficient.

### Commit 7 — CLAUDE.md enforcement section + agent contract

- New "## Enforcement" section in CLAUDE.md:
  - Pointer to this RFC.
  - `make setup-hooks` instruction (one-time per clone).
  - The §4.2 agent-side contract for `spex-halt/v1` payloads (the
    canonical place the rule lives — not in memory).
- Pointer in CLAUDE.md "Git Conventions" to §7.4-migrated rules.

Outcome: CLAUDE.md is the durable home for the agent contract.

### Commit 8 — Helper: `hook-violations-summary`

- `scripts/hook-violations-summary` — `jq`-based rollup of the log.
  Usage: `scripts/hook-violations-summary [--rule <slug>] [--since <date>]`.

Outcome: rule tuning is one command away.

### Acceptance gate (before merge)

- `make verify-enforcement` passes locally.
- Fresh docker container test (§7.6): clone, `make setup-hooks`, run
  `/implement <a-bead>` or similar; agent picks up rules without any
  memory directory present.
- Every catalog row in §3 has a corresponding fixture under
  `scripts/hooks/test/` that asserts the hook fires.
- A CI check confirms `core.hooksPath` is `scripts/git-hooks` in the
  repo's checked-in config (via `.git/config` template or a setup
  script run in CI).

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

### 9.3 Why "skill pre-flight" was removed as a layer

An earlier draft listed "Skill pre-flight" as one of three target
enforcement layers, applied to R3, R4, R9, and (implicitly) R13. The
intent was: a Bash check at the top of each SKILL.md that runs before
any work. The problem: a SKILL.md file is markdown read by the model.
"At the top of the skill" is not a process boundary or a hook — it's
prose the model must execute. Same enforcement strength as the rule it
was supposed to enforce. The model that skips the rule will also skip
the pre-flight check that's supposed to catch it.

The catalog now maps every row to either a CC hook (PreToolUse running
a script) or a git hook (pre-commit, post-commit, pre-push). Both run
without model cooperation, both return structured JSON per §4.1.

The cost: rules that genuinely depend on skill context (R6, R9, R12)
need a way to know which skill is active. That's §9.1 — a real spike
with a tractable resolution path, not a "model is supposed to remember"
hope.

### 9.4 R3 TTL choice and offline contexts

The catalog formulates R3 as "fetched within last 15 min" rather than
"always fetch on every branch creation," to avoid forcing an SSH-SK
tap on a fast iteration loop. Open sub-questions:

- **TTL value.** 15 min is a guess. Could be longer (an hour) since
  the real harm is working against months-stale `origin/main`, not
  minutes-stale. Pick a value when the hook lands and tune from the
  violation log.
- **Offline contexts.** The hook check is `mtime(.git/FETCH_HEAD) < 15 min`.
  In an offline session the file is just stale; the hook will block.
  Probably acceptable (the user knows they're offline), but worth a
  flag like `SPEX_OFFLINE=1` to bypass. That's a narrow override —
  same shape as `SPEX_REBASELINE=1` for R12.

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
