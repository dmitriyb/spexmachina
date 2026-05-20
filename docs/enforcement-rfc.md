# RFC: Machine-enforced pipeline guardrails

**Status:** Draft
**Branch:** `rfc/enforcement-guardrails`
**Author:** Dmitrii Bozhko (with Claude)
**Date:** 2026-05-19 (revised 2026-05-20 — Phase 2: skill-frontmatter hooks)

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
  the skills' frontmatter `hooks:` blocks, `scripts/hooks/`,
  `scripts/git-hooks/`), so it travels across docker containers and
  machines.

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
| R6 | `br close` only from `/review` after LGTM | CLAUDE.md:49 | Prose | **CC skill hook** (`deny-br-close.sh` declared in the frontmatter of every skill except `/review`) | `scripts/hooks/deny-br-close.sh` | `br-close-outside-review` |
| R7 | Never read `.beads/beads.db` directly | CLAUDE.md:62 | Prose | **CC project hook** (PreToolUse Bash, Read tool) | `scripts/hooks/block-beads-db-read.sh` | `beads-db-direct-read` |
| R8 | Never read `br`'s internal state files (`.beads/.br_history`, `.beads/.br_recovery`) | CLAUDE.md:63 | Prose | **CC project hook** (same script as R7) | `scripts/hooks/block-beads-db-read.sh` | `beads-db-direct-read` |
| R9 | `/spec`, `/propose`, `/converge`, `/spec-review`, `/spec-drift` must not commit | `feedback_skills_no_autocommit` | Prose + memory | **CC skill hook** (`deny-commit.sh` declared in the frontmatter of each non-committing skill) | `scripts/hooks/deny-commit.sh` | `skill-must-not-commit` |
| R10 | `/fix`, `/review`, `/cleanup`, `/implement` *must* commit and push | skill docs | Prose | **No hook** — these skills simply do not declare a `deny-commit` frontmatter hook; commit is allowed by default | n/a | n/a |
| R11 | Never use `git rebase -i` / `git add -i` (interactive) | operational | Prose | **CC project hook** (PreToolUse Bash matcher) | `scripts/hooks/block-interactive-git.sh` | `interactive-git-not-supported` |
| R12 | Never run `spex hash` outside an explicit re-baseline | `feedback_spex_pipeline_not_hash` | Memory only | **CC project hook** (PreToolUse Bash on `spex hash *`; allow only if `SPEX_REBASELINE=1`) | `scripts/hooks/check-spex-hash-rebaseline.sh` | `spex-hash-bypasses-pipeline` |
| R13 | Edit/Write/commit when `HEAD == main` | `feedback_check_branch_before_editing` + CLAUDE.md:37 | Memory only | **CC project hook** (PreToolUse on Edit, Write, Bash on `git commit *`) | `scripts/hooks/check-not-on-main.sh` | `editing-on-protected-branch` |
| R14 | One skill per session — no skill-mixing | user requirement (2026-05-20) | n/a (new) | **CC skill hook** (`assert-single-skill.sh` declared in *every* skill's frontmatter) | `scripts/hooks/assert-single-skill.sh` | `skill-mixing-detected` |

Layer key:
- **CC project hook**: `PreToolUse` matcher in `.claude/settings.json`, executes a script under `scripts/hooks/`. Active for the whole session. Used for rules that apply universally.
- **CC skill hook**: `PreToolUse` matcher declared in a `SKILL.md`'s YAML frontmatter `hooks:` block, executes a script under `scripts/hooks/`. Active only while that skill is active. Used for rules that depend on *which skill is running* (R6, R9).
- **Git hook**: script under repo-tracked `scripts/git-hooks/`, installed via `git config core.hooksPath scripts/git-hooks` (one-time `scripts/setup-hooks`).

All three return the structured JSON of §4.1 and block by `permissionDecision: "deny"` (CC hooks) or non-zero exit (git hooks).

**No "skill pre-flight" layer, and no marker file.** Earlier drafts
included a "skill pre-flight" layer (rejected — see §9.3) and then a
marker-file mechanism (`.claude/skill-context.json`) so a *project*
hook could tell which skill was active. The marker file is also gone:
the **CC skill hook** layer makes skill identity intrinsic — a hook
declared in `/spec`'s frontmatter only runs while `/spec` is active, so
the hook script needs no skill-detection logic at all. See §9.1.

Why not the permissions system for R6/R9? Claude Code's `permissions`
block (`allow`/`deny`/`ask`) cannot express per-skill rules:
`deny` is absolute ("if a tool is denied at any level, no other level
can allow it"), so "deny `br close` globally, allow it for `/review`"
is structurally impossible; and skill frontmatter has no `deny` field
(only `allowed-tools`, which is positive-only and matches tool *names*,
not command patterns). Permission denials are also terse — they cannot
carry the `spex-halt/v1` recovery payload. Hooks are the only mechanism
that can do contextual, per-skill, structured-message enforcement.

Layer choice rationale:
- **Git hooks** for anything involving `git commit` or `git push` — they
  run regardless of who drove the action (model, user, CI, another
  tool). Once `core.hooksPath` is set, hooks are active in every clone.
- **CC project hooks** for tool-shape rules that apply universally
  (`sqlite3 .beads/`, `--no-gpg-sign`, Edit on main, `spex hash`).
- **CC skill hooks** for rules that are *per-skill*: R6 (`br close`
  belongs to `/review` alone) and R9 (`/spec` and the other authoring
  skills must not commit). The skill that should be restricted carries
  the restriction in its own frontmatter.

Note: only R3, R4, R12 consult runtime *state* beyond the tool input
(fetch-recency, branch name, the `SPEX_REBASELINE` env var). R6 and R9
no longer consult any state — frontmatter scoping replaces what the
marker file used to do. R7, R8, R11, R13 are pure tool-shape matches.

## 4. Halt-and-recover protocol

### 4.1 Hook output format (structured JSON)

Claude Code's hook protocol for `PreToolUse` requires the hook to exit
**0** and write a `hookSpecificOutput` envelope to **stdout**. Exit-2
with stderr is supported but only gives Claude raw stderr text — no
structured control. We use the structured-output mode (source:
[Claude Code hooks guide](https://code.claude.com/docs/en/hooks-guide.md),
"Structured JSON output" and "PreToolUse" sections).

The wire format every hook script emits:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "<spex-halt/v1 payload, serialised as a JSON string>"
  }
}
```

The model sees the `permissionDecisionReason` string. We pack the
`spex-halt/v1` schema into that string as JSON, so the model can parse
it deterministically rather than reading prose.

Inner `spex-halt/v1` schema:

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

Field contract (inner `spex-halt/v1`):

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

A reference helper `scripts/hooks/lib/emit-halt.sh` builds the inner
`spex-halt/v1` payload and the outer `hookSpecificOutput` envelope in
one call. Every hook script calls it so the wire format can evolve
without rewriting every script:

```bash
# scripts/hooks/lib/emit-halt.sh
# Usage: emit_halt <rule> <command> <invariant> <source> [<path> <destructive> ...]
#   Trailing args are recovery items in alternating (text, bool) pairs.
#   Pass `false`/`true` literally (no quotes) so jq --argjson can consume them.
emit_halt() {
  local rule="$1" command="$2" invariant="$3" source_loc="$4"
  shift 4

  local recovery_json="[]"
  while [[ $# -ge 2 ]]; do
    recovery_json="$(jq --arg p "$1" --argjson d "$2" \
      '. + [{path:$p, destructive:$d}]' <<<"$recovery_json")"
    shift 2
  done

  local inner
  inner="$(jq -nc \
    --arg rule "$rule" \
    --arg command "$command" \
    --arg invariant "$invariant" \
    --arg source_loc "$source_loc" \
    --arg cwd "$PWD" \
    --arg head "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '')" \
    --arg skill "${SPEX_SKILL:-}" \
    --argjson recovery "$recovery_json" \
    '{
      protocol: "spex-halt/v1",
      rule: $rule,
      command: $command,
      cwd: $cwd,
      head: $head,
      skill: (if $skill == "" then null else $skill end),
      invariant: $invariant,
      source: $source_loc,
      recovery: $recovery,
      directive: "halt"
    }')"

  # Log the inner payload (canonical form) before wrapping for the wire.
  printf '%s\n' "$inner" | "$(dirname "${BASH_SOURCE[0]}")/../log-violation"

  # Wrap for the Claude Code PreToolUse wire protocol.
  jq -nc --arg reason "$inner" \
    '{hookSpecificOutput:
        {hookEventName: "PreToolUse",
         permissionDecision: "deny",
         permissionDecisionReason: $reason}}'
}
```

The function prints the outer envelope to stdout (the caller pipes it
directly to its own stdout) and side-effects the log line. Caller exits
0 — Claude Code interprets the `permissionDecision: "deny"` as the block.

### 4.2 Agent-side contract

When a `PreToolUse` hook denies a tool call, Claude Code surfaces the
`permissionDecisionReason` field to the agent as the block reason. In
our protocol that field is a JSON string conforming to `spex-halt/v1`.
CLAUDE.md (added in Commit 1) codifies the contract:

1. **Recognise the protocol.** If a tool block's reason parses as JSON
   with `"protocol": "spex-halt/v1"`, follow this contract. Otherwise
   the block is a non-spex hook or a different mechanism — handle
   normally.
2. **Halt the current tool sequence.** No further tools on the same
   goal-path until the user has responded.
3. **Render the payload to the user.** The rendering MUST include
   verbatim: the `rule` slug, the `invariant`, the `source`, and every
   entry in `recovery`. The agent MAY add a one-line plain English
   summary on top.
4. **Propose, do not execute.** The agent proposes a recovery path
   drawn from `recovery` but does not execute it.
5. **Destructive recovery needs per-step confirmation.** Entries with
   `destructive: true` require explicit user authorisation each.
   The agent never bundles destructive steps. CLAUDE.md enumerates the
   syntactic backstop list: `rm`, `rm -rf`, `git reset --hard`,
   `git checkout -- <file>`, `git clean -f`, `git branch -D`,
   force-push, `br delete`, any DB wipe, any operation that overwrites
   unstaged work.
6. **Non-destructive recovery** (file edits, `git add`, `git stash`,
   `git fetch`, `br update`, additional `Read`s) may proceed once the
   user acknowledges the block and names the path.

### 4.3 Override env vars (when allowed)

The default rule is **no generic override**: the human is the override.
A general "shut up the hooks" env var would either bit-rot to "always
on" or tempt the model to set it. Recovery is always through the user.

A rule MAY introduce its own narrow env-var override only if all of:

1. The override is **rule-specific** (not cross-cutting).
2. There is a **legitimate, recurring scenario** that hits the rule and
   the override is the right response to that scenario — not a way to
   keep working past the rule.
3. The override is **scoped**: an env var, not a marker file, not a
   global config flip. It only affects one shell invocation.
4. The rule documents **who is expected to set it** (always the user,
   never the agent). The agent must not set the override; if it
   believes the override is warranted, it asks the user.

Currently two rules qualify under this carve-out:

- **R12 `SPEX_REBASELINE=1`** — the re-baseline operation is legitimate
  when the TreeBuilder keying scheme changes (see commit
  `b847f45 spec: regenerate snapshot baseline against current
  TreeBuilder keying`). Without the marker, `spex hash` is blocked.
- **R3 `SPEX_OFFLINE=1`** — in offline contexts (no network), the
  fetch-recency check is meaningless. The user sets this when working
  intentionally offline. Without it, the hook treats stale
  `FETCH_HEAD` as a stale-origin block (which is the desired default).

Future rules adding their own override must update this section and
justify the addition. Two overrides today, period.

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

Worked examples for the highest-value rules. Code is illustrative and
uses helpers from `scripts/hooks/lib/emit-halt.sh`:

- `emit_halt` — builds the §4.1 JSON payload, logs it, prints the
  deny envelope.
- `strip_heredoc_bodies` — removes heredoc bodies (literal data, not
  command text). Pure bash, portable across mawk/gawk.
- `strip_quoted_strings` — blanks single-line quoted-string contents.
  Used by hooks matching a flag/token never legitimately quoted.
- `cmd_matches <cmd> <ere>` — true if `<ere>` occurs at a
  command-start boundary (line start, or after `; & | \` (`),
  optionally past an `env` word and/or a run of leading `VAR=value`
  environment-assignment prefixes, after heredoc stripping. The
  matcher for rules keyed on a *command*: it rejects the command word
  inside quoted prose while still matching a real command whose later
  argument is quoted — and a real command behind a `GIT_DIR=… ` /
  `env FOO=1 ` prefix. Shared EREs (`SPEX_ERE_GIT_COMMIT`,
  `SPEX_ERE_BR_CLOSE`, `SPEX_ERE_SPEX_HASH`, `SPEX_ERE_BRANCH_CREATE`)
  tolerate path prefixes and git global options so a rule cannot be
  bypassed by trivial respelling.

### 6.1 CC skill hook: block `br close` outside `/review`

R6 is enforced by a **skill-frontmatter hook**, not a project hook. The
hook script is declared in the frontmatter of every skill *except*
`/review`. Because the hook only runs while one of those skills is
active, the script needs no skill-detection logic.

`SKILL.md` frontmatter (every skill except `/review`):

```yaml
---
name: implement
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: scripts/hooks/deny-br-close.sh
---
```

`scripts/hooks/deny-br-close.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"
export SPEX_SKILL="${1:-}"   # declaring skill name, for the log only

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0
command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# SPEX_ERE_BR_CLOSE tolerates a path prefix (bin/br, /usr/bin/br);
# cmd_matches anchors it to a command boundary so `br close` text
# inside quoted prose / a heredoc body does not trip.
if cmd_matches "$command" "$SPEX_ERE_BR_CLOSE"; then
  emit_halt \
    "br-close-outside-review" \
    "$command" \
    "br close is allowed only from /review after LGTM." \
    "CLAUDE.md:49" \
    "If the PR is LGTM, run /review — it is the only skill that may close beads" false \
    "If the bead must be closed for an exceptional reason, the user invokes br close directly outside the agent" false
fi
exit 0
```

`emit_halt` writes the deny envelope to stdout and the caller exits 0
(per §4.1). `/review` declares no such hook, so it can close beads.

### 6.2 CC hook: block direct reads of `.beads/beads.db`

`scripts/hooks/block-beads-db-read.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"

case "$tool" in
  Bash)
    command="$(jq -r '.tool_input.command // empty' <<<"$input")"
    # A reading/copying tool, boundary-anchored, with a .beads/beads.db
    # (or br internal-state) argument bound to the SAME command.
    # [^;&|`] keeps the path from a later command in a chain out of
    # the match. The path itself may be quoted — cmd_matches anchors
    # the keyword, not the path.
    if cmd_matches "$command" '(sqlite3?|cat|head|tail|less|more|xxd|hexdump|od|strings|file|python3?|perl|ruby|cp|dd|mv|install)[^;&|`]*\.beads/(beads\.db|\.br_)'; then
      target="$command"
    else
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

emit_halt \
  "beads-db-direct-read" \
  "$target" \
  "Never read .beads/beads.db directly. Use br commands or .beads/issues.jsonl." \
  "CLAUDE.md:62" \
  "For a single bead: br show <id> --json" false \
  "For listings: br list --all --limit 0 (the bare br list filters silently — open-only, limit 50)" false \
  "For raw tracker dump: jq -s '{issues: .}' .beads/issues.jsonl" false
exit 0
```

`emit_halt` (per §4.1) builds the inner payload, appends it to the
violation log, and prints the `permissionDecision: "deny"` envelope to
stdout. The hook always `exit 0` — the envelope, not the exit code, is
what blocks. This hook matches both `Bash` (to catch
`sqlite3 .beads/beads.db`) and `Read` (the model reading the file
directly); `.claude/settings.json` enumerates both matchers.

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

Two plain scripts under `scripts/` (the project has no Makefile and
does not use `make`; `make` is not a Claude Code dependency):

```bash
# scripts/setup-hooks
#!/usr/bin/env bash
set -euo pipefail
git config core.hooksPath scripts/git-hooks
chmod +x scripts/git-hooks/* scripts/hooks/*.sh scripts/hooks/log-violation \
         scripts/hooks/test/*.sh
echo "git hooks active at scripts/git-hooks; CC hooks read by .claude/settings.json"
```

```bash
# scripts/verify-enforcement
#!/usr/bin/env bash
exec scripts/hooks/test/run-all.sh
```

`CLAUDE.md` gains an "Enforcement" section pointing at this RFC and
codifying:
- Run `scripts/setup-hooks` once per clone.
- On any hook response containing `"protocol": "spex-halt/v1"`,
  follow the §4.2 agent-side contract.
- Run `scripts/verify-enforcement` before opening a PR that touches
  `.claude/settings.json`, `scripts/hooks/`, or `scripts/git-hooks/`.

(Phase 1 shipped these as `Makefile` targets; Phase 2 Commit 12
converts them to plain scripts — see §8.)

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
  → **Enforce as R13 (CC PreToolUse on Edit/Write/Bash-git-commit that
  asserts HEAD ≠ main), then delete.** Once the hook exists the prose
  is sufficient.

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

The work lands in two phases. **Phase 1** (Commits 1–8) shipped the
13-rule catalog with the marker-file mechanism for skill identity.
**Phase 2** (Commits 9+) refactors R6 and R9 onto CC skill-frontmatter
hooks per the §9.1 decision, deleting the marker file and the skill
"Step 0" boilerplate, and strips the now-redundant prose the hooks
made unnecessary. Phase 2 exists because a review of Phase 1 found the
marker file was still model-dependent and bloated every SKILL.md.

**Ordering rule:** the CLAUDE.md "Enforcement" section (which contains
the agent-side contract for `spex-halt/v1` payloads, per §4.2) MUST
land *before* any hook that can block. Otherwise the model sees JSON
output from a hook and has no contract telling it what to do. Hence
Commit 1 ships the agent contract in CLAUDE.md alongside the hook
infrastructure — not in a late commit.

### Phase 1 — catalog with marker-file skill identity (Commits 1–8, shipped)

### Commit 1 — Foundation + agent contract

- `scripts/hooks/lib/emit-halt.sh` — the `emit_halt` helper. Single
  source of truth for the §4.1 JSON schema.
- `scripts/hooks/log-violation` — one-line script: read JSON from
  stdin, prepend `ts` via `jq`, append to `.claude/hook-violations.log`.
- `scripts/hooks/test/run-all.sh` — test harness skeleton.
- `Makefile` with `setup-hooks` and `verify-enforcement` targets.
- `.gitignore` adds `.claude/hook-violations.log`.
- **CLAUDE.md "## Enforcement" section** — agent-side contract for
  `spex-halt/v1` payloads (per §4.2), `make setup-hooks` note, pointer
  to this RFC.
- **`.claude/settings.json`** — created (currently only
  `.claude/settings.local.json` exists). Hook matcher entries added
  empty here; later commits append.

Outcome: agent contract is in CLAUDE.md, infrastructure exists, no
rules enforced yet.

### Commit 2 — Git hooks: protect main + verify signing

- `scripts/git-hooks/pre-commit` — R1 + R5a pre-commit half.
- `scripts/git-hooks/post-commit` — R5a post-commit half.
- `scripts/git-hooks/pre-push` — R2 + R5a per-ref re-verification.
- Fixtures: commit on main, push to main, `--no-gpg-sign` commit.

Outcome: R1, R2, R5a enforced.

### Commit 3 — CC hooks: syntactic blocks (no skill context needed)

- `scripts/hooks/block-signing-bypass.sh` — R5b.
- `scripts/hooks/block-beads-db-read.sh` — R7 + R8 (single script;
  catalog lists them separately for source-of-rule reasons but they
  share enforcement).
- `scripts/hooks/block-interactive-git.sh` — R11.
- `scripts/hooks/check-not-on-main.sh` — R13.
- `scripts/hooks/check-fetched-recent.sh` — R3 (honors
  `SPEX_OFFLINE=1`).
- `scripts/hooks/check-branch-from-origin-main.sh` — R4.
- `.claude/settings.json` matchers wired.
- Fixtures for each.

Outcome: R3, R4, R5b, R7, R8, R11, R13 enforced.

### Commit 4 — Skill-context propagation (marker file) — *superseded by Phase 2*

Shipped `scripts/hooks/lib/active-skill.sh` and a "Step 0" marker-write
block in all 9 SKILL.md files. Phase 2 (Commit 9) deletes all of it.

### Commit 5 — CC hooks: skill-aware blocks (marker-based) — *refactored in Phase 2*

Shipped `check-br-close-skill.sh` and `check-skill-commit-allowed.sh`
(marker-consuming) plus `check-spex-hash-rebaseline.sh` (R12, env-var,
unaffected by Phase 2). Phase 2 replaces the first two with
context-free frontmatter-hook scripts.

### Commit 6 — Memory migration

- Delete §7.1 + §7.2 memory entries.
- Move §7.3 entries into target SKILL.md files.
- Move §7.4 entries into CLAUDE.md as one-liners under existing
  sections (Git Conventions, Issue Tracking, Bead Context Resolution).
- Verify §7.5 entries handled per disposition.

Outcome: memory holds only ephemeral observations.

### Commit 7 — Helper: `hook-violations-summary`

- `scripts/hook-violations-summary` — `jq`-based rollup.

### Commit 8 — Heredoc-strip backfill

- Factored `strip_heredoc_bodies` into `emit-halt.sh`; applied to every
  Bash-matching hook. Caught a false-positive where rule-discussion
  text inside a `gh pr edit` heredoc tripped R5b.

Outcome of Phase 1: all 13 rows machine-enforced via project hooks +
git hooks + the marker file.

### Phase 2 — refactor skill identity onto frontmatter hooks (Commits 9+)

#### Commit 9 — CC skill-frontmatter hooks for R6 and R9

- `scripts/hooks/deny-commit.sh` — context-free: deny if the Bash
  command is a `git commit`, else allow. (No skill detection — the
  frontmatter scoping does that.)
- `scripts/hooks/deny-br-close.sh` — context-free: deny if the Bash
  command is a `br close`, else allow.
- Each non-committing skill (`spec`, `propose`, `converge`,
  `spec-review`, `spec-drift`) gets a `hooks:` block in its YAML
  frontmatter referencing `deny-commit.sh`.
- Each non-`/review` skill (8 of them) gets a `hooks:` block
  referencing `deny-br-close.sh`.
- The frontmatter passes the skill's own name as a hook-command
  argument (e.g. `command: scripts/hooks/deny-commit.sh spec`); the
  script sets `SPEX_SKILL="$1"` before calling `emit_halt`, so the
  violation log's `skill` field stays populated for tuning. This is
  the only thing the script does with the skill name — it is NOT used
  for a decision (the decision is "am I running at all", answered by
  frontmatter scoping).
- `.claude/settings.json` loses the `check-br-close-skill.sh` and
  `check-skill-commit-allowed.sh` matchers.
- Delete `scripts/hooks/check-br-close-skill.sh`,
  `scripts/hooks/check-skill-commit-allowed.sh`,
  `scripts/hooks/lib/active-skill.sh`, and its test.
- Delete the "Step 0" marker-write block from all 9 SKILL.md files.
- Delete the `.claude/skill-context.json` line from `.gitignore`.
- Remove the marker-file mechanism from CLAUDE.md "## Enforcement".
- Fixtures: `deny-commit.sh` and `deny-br-close.sh` deny their target
  command and allow everything else.

#### Commit 10 — R14: one-skill-per-session guard

`scripts/hooks/assert-single-skill.sh` — declared in *every* skill's
frontmatter. On each tool call it reads `session_id` from the hook
stdin, looks up `.claude/skill-sessions/<session_id>`:
- file absent → write the declaring skill's name, allow.
- file names the same skill → allow.
- file names a different skill → halt (`skill-mixing-detected`).

Per §9.1 this makes "one skill per session" an enforced invariant and
neutralises the frontmatter-hook lifecycle concern. `.gitignore` adds
`.claude/skill-sessions/`. Fixture: same-skill repeat allowed,
different-skill denied, fresh `session_id` starts clean.

#### Commit 11 — Strip redundant skill prose

The Phase 1 memory migration (Commit 6) *added* to SKILL.md files. With
hooks now enforcing the rules, some of that prose is redundant and
should be deleted (the review feedback that prompted Phase 2):

- Remove the hook-narration half of each "Commits and pushes" tagline
  (keep the plain intent statement, drop "Enforcement hook X permits…").
- Remove `/implement`'s "This skill does NOT close beads" sentence —
  R6 enforces it.
- Audit `First run git checkout main && git pull --rebase` in
  `/implement` and `/cleanup`: keep the *workflow instruction* (get to
  a clean base) but do not restate the branch-hygiene *rules* that
  R3/R4/R13 now enforce.
- Keep genuine authoring guidance that was never mechanically
  enforceable (review-findings-no-punt, supersession, pipeline-errors).

#### Commit 12 — Makefile → plain scripts

Replace the `Makefile` with `scripts/setup-hooks` and
`scripts/verify-enforcement`. The project never had a Makefile; `make`
is not used by Claude Code and adds a build-tool dependency for two
convenience targets. Plain scripts match the existing `scripts/`
directory. Update CLAUDE.md and this RFC's command references.

### Acceptance gate (before merge)

- `scripts/verify-enforcement` passes locally (all fixture suites).
- Reduced live check: confirm a frontmatter hook is active for the
  *duration* of its skill (deactivation no longer matters — R14
  enforces one-skill-per-session). Invoke `/spec`, attempt a
  `git commit`, confirm it is denied.
- Fresh-container test (§7.6): clone, `scripts/setup-hooks`, run a
  representative skill; agent picks up rules without any memory
  directory present.
- Every catalog row in §3 has a fixture that asserts the hook fires.
- CI check for `core.hooksPath` is a documented post-merge follow-up
  (no CI configured as of this PR).

## 9. Open questions and risks

### 9.1 Skill-scoped enforcement: from marker file to skill-frontmatter hooks

R6 and R9 depend on knowing which skill is active. This section records
the design evolution and the chosen mechanism.

**Rejected — `SPEX_SKILL` env var.** A skill exports `SPEX_SKILL=<name>`
at its top; a project hook reads it. Fails because a SKILL.md is
markdown the model reads — the export only happens if the model runs
it. Model-dependent, same weakness the RFC exists to remove.

**Rejected — marker file (`.claude/skill-context.json`).** A skill
writes a JSON marker as its first action; a project hook reads it with
a TTL. Implemented in the first pass of this PR (Commits 4–5) and then
removed. Same model-dependency as the env var — the skill has to run
the write command — plus it required a "Step 0" boilerplate block in
all 9 SKILL.md files, which is exactly the skill bloat the user
objected to in review.

**Rejected — the permissions system.** See §3 "Why not the permissions
system" — `deny` is absolute and cannot be exempted per-skill, and
skill frontmatter has no `deny` field.

**Chosen — CC skill-frontmatter hooks.** Claude Code lets a `SKILL.md`
declare hooks in its YAML frontmatter:

```yaml
---
name: spec
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: scripts/hooks/deny-commit.sh
---
```

A frontmatter hook is active **only while that skill is active**. This
makes skill identity *intrinsic*: the hook script does not detect the
skill — its mere presence in `/spec`'s frontmatter means "this runs
during `/spec`." `deny-commit.sh` becomes a trivial context-free
script: "if the command is a `git commit`, deny; else allow." No
marker, no `active-skill.sh`, no Step 0.

- **R9**: `deny-commit.sh` is declared in the frontmatter of the 5
  non-committing skills (`spec`, `propose`, `converge`, `spec-review`,
  `spec-drift`). `/fix`, `/review`, `/cleanup`, `/implement` declare
  nothing — commit allowed by default (R10).
- **R6**: `deny-br-close.sh` is declared in the frontmatter of the 8
  skills that are *not* `/review`. `/review` declares nothing — it is
  the sole skill that can `br close`.

**Eliminated by this design:** the marker file, `active-skill.sh` and
its TTL logic, all 9 "Step 0" blocks, and the two context-branching
scripts (`check-skill-commit-allowed.sh`, `check-br-close-skill.sh`).
The fail-mode asymmetry table from the marker-file design no longer
applies — there is no "skill unknown" state, because the hook only
exists in contexts where the skill *is* known.

**Behavioral note — ad-hoc actions.** With R6/R9 as frontmatter hooks,
an agent action taken outside *any* skill (e.g. the user directly says
"commit this" or "close bead X" in a bare conversation) is not blocked
by R6/R9. This is acceptable and arguably correct: it is an explicit
user instruction, not a skill running unsupervised, and CC hooks only
ever gate the agent — the user can always run the command themselves.
The git hooks (R1, R2, R5a) still apply to every commit/push
regardless of skill.

**Skill-hook lifecycle — resolved by the one-skill-per-session rule
(R14).** The Claude Code docs say frontmatter hooks are "scoped to
component lifecycle" but do not define when a skill's lifecycle ends.
Two failure shapes were originally a concern:

1. *Hook deactivates too early* — the restriction lifts before the
   skill's work is done.
2. *Hook lingers too long* — the restriction outlives the skill, so a
   stale `deny-commit` could block an unrelated later action.

The project's actual workflow eliminates failure shape 2: **each skill
runs in its own Claude Code session.** A session never reaches a
second skill, so a lingering hook can only ever affect the *same*
skill it belongs to — which is correct behaviour, not a false block.
The user has explicitly accepted that frontmatter hooks need not
deactivate.

To make "one skill per session" an enforced invariant rather than a
convention, **R14** adds `assert-single-skill.sh` to every skill's
frontmatter. It records which skill owns the current session (keyed by
the `session_id` field in the hook's stdin, under
`.claude/skill-sessions/<session_id>`). If a *different* skill's hook
fires in a session that already has an owner, it halts with
`skill-mixing-detected` — recovery: "start a fresh session for the
second skill." With R14 in place, skill-mixing cannot happen silently,
so failure shape 2 cannot cause an incorrect block.

Failure shape 1 (deactivate too early) remains the only thing worth a
live check: confirm a frontmatter hook is active for the *duration* of
its skill. This is the reduced acceptance check in §8 — no marker-file
fallback is retained, because the marker file was itself
model-dependent and the one-skill-per-session model is the cleaner
guarantee.

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

### 9.4 R3 TTL choice

The catalog formulates R3 as "fetched within last 15 min" rather than
"always fetch on every branch creation," to avoid forcing an SSH-SK
tap on a fast iteration loop. Open: the TTL value itself. 15 min is a
guess. The real harm is working against months-stale `origin/main`,
not minutes-stale. An hour might be fine. Pick when the hook lands and
tune from the violation log.

Offline contexts are handled by the `SPEX_OFFLINE=1` env-var override
declared in §4.3, not as an open question.

### 9.5 Hook violation log rotation

At one line per block, log size is unlikely to matter for a year. Defer
until the log warrants it. If/when it does, add a `logrotate`-style
trim to `scripts/hook-violations-summary`.

### 9.6 Verification

`scripts/verify-enforcement` runs every fixture under
`scripts/hooks/test/`; each fixture simulates a blocked action and
asserts the hook fires with the correct slug. Run it before any PR
that touches the enforcement files. The skill-frontmatter-hook
lifecycle (§9.1) cannot be fixture-tested — it needs the live check
described in §8 Commit 10.

### 9.7 Interaction with the auto-mode classifier

Once these hooks exist, the classifier may stop citing memory entries
because the hook is now the authority for the rules it covers. Worth
observing post-implementation; no design change is needed up front. If
the classifier continues to cite memory after a rule has migrated to a
hook, that's a signal the migration of that specific rule is
incomplete — the prose still exists somewhere it shouldn't.

## 10. Acceptance criteria for the RFC

This RFC is complete when the following hold:

- §3 catalog covers every "Never / MUST NOT" rule in `CLAUDE.md` and
  `skills/*/SKILL.md` **that is expressible as a tool-shape or git
  rule.** Two documented rules are deliberately *not* in the catalog
  because they cannot be mechanically enforced by a hook:
  - CLAUDE.md "Do NOT use markdown TODOs" — there is no tool-call
    shape that distinguishes a TODO marker from ordinary text.
  - CLAUDE.md:63's broad principle "never bypass the documented
    `br`/`spex` interfaces" — R8 mechanically enforces the concrete,
    catchable case (reading `br`'s internal state files); the broad
    principle remains prose because "bypass" has no finite syntactic
    form. R8's catalog text is scoped to what the hook actually does.
- §4 halt-and-recover protocol is unambiguous: a follow-up implementer
  writing a new hook needs no clarifying questions about output format
  or agent behavior.
- §5 logging schema is exact: rule slug, fields, file path.
- §6 reference implementations are runnable as-is (modulo `chmod +x`).
- §7 memory inventory matches `ls /home/dev/.claude/projects/-workspace-spexmachina/memory/`
  at the time of the PR.
