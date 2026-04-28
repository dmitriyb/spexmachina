---
name: converge
description: "Drive a /spec'd proposal through the deterministic spex pipeline (validate → diff → impact → emit → adapter → ingest). Halt and diagnose any stage failure; never auto-fix the spec or the binary."
argument-hint: "[proposal-stem]"
---

# /converge — Drive the Spex Pipeline

Take a proposal that has already been processed by `/spec`, and execute the
structural pipeline that turns its spec changes into bead changes:
`spex diff → spex impact → spex emit → adapter → spex ingest`.

This skill is a **harness**, not an editor:

- It calls `scripts/run-pipeline.sh` for the deterministic plumbing.
- It diagnoses stage failures and reports actionable next steps.
- It does NOT auto-fix the spec (that's `/spec`'s job). It does NOT touch the
  spex binary or its source. It does NOT commit anything — the user reviews
  and commits after the run.

The only mechanical things `/converge` may resolve automatically: missing
`bin/spex` (build it), and a stale `.beads/.sync.lock` from a crashed previous
run (clear it after confirming no live writer holds it).

## Step 1: Resolve proposal

- If `$ARGUMENTS` is a path or filename stem of a file in `spec/proposals/`,
  use it.
- If `$ARGUMENTS` is empty, list `spec/proposals/*.md` and ask the user to
  pick.

The "stem" is the filename without `.md` (e.g.
`2026-04-27-pipeline-cleanup-and-refresh-mode`). Confirm with the user before
invoking the pipeline.

## Step 2: Confirm preconditions

Before invoking the script:

- Current branch is not `main` (the script also checks, but failing fast is
  friendlier).
- `/spec` has been run on this proposal already. Heuristic: `bin/spex diff
  --json | jq '.changes | length'` returns > 0 and `.errors | length` returns
  0. If either fails, stop and recommend `/spec`.
- The user understands the run will mutate the bead tracker. Do not bypass
  the review pause in step 4.

## Step 3: Run emit phase

Invoke the script:

```bash
scripts/run-pipeline.sh --phase emit --proposal <stem>
```

Capture the script's final stdout line `RUN_DIR=<path>` — every stage
artifact (validate.json, diff.json, impact.json, changeset.json, log) lives
under that directory. Exit code mapping is in step 6.

If the script exits 0, proceed to step 4.

## Step 4: Produce a human-readable review

This step is mandatory. Do not skip it even in auto mode — the next step
mutates the tracker (creates and closes beads), which is a shared-system
write.

`changeset.json` is a machine artifact (op_ids, three-shape ref encoding,
forward references). The user does NOT read it. The skill reads it plus
`<run-dir>/beads.json` (the canonical tracker state derived from
`.beads/issues.jsonl` during impact — full bead set, all statuses, no
pagination) and synthesizes a review surface in plain English.

Print, in this shape:

```
Pipeline run for proposal: <stem>
git_head: <short-sha>  branch: <branch>

CREATE: <N> new beads
  Proposal epic: "<title>"  (label: spex:<id>, priority <p>)
  ├─ <kind>: "<title>"  (spex:<id>, priority <p>)
  ├─ <kind>: "<title>"  (spex:<id>, priority <p>)
  ...

  Cleanup beads (often no real work; user closes on review):
  ├─ "<title>"  (spex:<id>)
  ...

CLOSE: <M> existing beads
  - spexmachina-<id>  "<title from beads.json>"  — <reason>
  ...

Replacement pairs (close-modified → fresh create carrying lineage):
  - spexmachina-<old>  →  spex:<new-label>   "<title>"
  ...

Total tracker mutations: <N+M> ops.
```

Then ask: **"Apply this changeset to the tracker now? (yes / no)"**. Do not
proceed without an explicit yes.

Conventions for the synthesis:

- Use `op.title` from the changeset for create titles (already
  human-readable).
- Resolve close-op `target.bead_id` against `beads.json` to surface the bead
  title and current status. If the bead is missing from `beads.json`, mark
  it `<not in tracker>`.
- Group cleanup creates separately. A create is a cleanup if its title
  begins with `cleanup:` or `Code cleanup:`, or its `spec_node_kind` is
  `cleanup`. Cleanups frequently need no implementation — flagging them lets
  the user spot them at a glance.
- Pair `close (Spec node modified)` with the corresponding create that
  carries `deps: [{ref:bead, bead_id:<old>, type:blocks}]`. Show as
  replacement pairs so the user sees the lineage clearly rather than
  unrelated close + create entries.
- If `ops` is empty (no-op proposal), say so plainly and still ask for
  confirmation — applying a no-op is harmless but the user should know.

Heuristics worth surfacing as warnings *above* the tree (not blockers, just
heads-ups):

- Very large batch (> 20 ops): is the proposal scope correct?
- Cleanup creates outnumber real-work creates: is this mostly bookkeeping?
- A `close (Spec node removed)` whose target bead is `closed` in
  `beads.json`: the close is a no-op label add, not a state transition —
  worth noting because the user might assume "close" means "did work."
- A modified node with a close but no paired replacement create: that's
  unusual — `arch_impl` modifications normally produce both ops.

## Step 5: Run apply phase

If the user confirms:

```bash
scripts/run-pipeline.sh --phase apply --run-dir <run-dir>
```

Capture stdout/stderr. Exit code mapping is in step 6.

If the script exits 0, proceed to step 7.

## Step 6: Diagnose stage failures

The script's exit codes map to a stage. For each failure, **halt** —
do not attempt to retry or auto-fix. Read the relevant artifact and give the
user an actionable next step:

| Exit | Stage              | Diagnosis target                | Next step |
|------|--------------------|---------------------------------|-----------|
| 1    | pre-flight         | stderr from the script          | Bad args, on main, missing binary, missing proposal — show stderr verbatim. |
| 10   | validate or diff   | `validate.json` / `diff.json`   | Spec is invalid (validate) or has completeness errors (diff). **Halt.** Recommend `/spec` — its loop-and-escape gate is the right tool. Do NOT auto-fix completeness errors here. |
| 11   | impact             | `impact.json` (if written) and `log` | Likely orphan map record or a tracker bead the impact module expected to find. Read `.beads/issues.jsonl` to identify the mismatch. Suggest `/cleanup` if cleanup beads are involved, or manual reconcile for orphans. |
| 12   | emit               | `log` and `impact.json`         | Usually a cycle in batch deps (a spec graph bug `/spec`'s validate should have caught) or a missing proposal frontmatter field. Halt; report. |
| 13   | adapter            | `receipts.json` (whatever got written) and `log` | Tracker-side: network, auth, or partial run. The receipts file shows what made it through. After fixing the underlying issue, the user can re-run `/converge` end-to-end (idempotency labels protect against duplicates) or manually re-run `--phase apply --run-dir <dir>`. |
| 14   | ingest             | `log` and the ingest stderr     | Invariant violation or input mismatch. **Never auto-retry** — re-running may compound the problem. Bead-map reflects whatever Reconciler committed before the violation; snapshot was NOT written, so the next pipeline run starts from the previous baseline. The user investigates. |

For all halts: report the run-dir path so the user can open the artifacts. A
short, named-error one-liner beats a wall of stderr.

## Step 7: Report success

If both phases completed, summarize cleanly:

- **Proposal**: `<stem>`
- **Run dir**: `<path>`
- **Beads**: created N, closed M (from `ingest_summary.json` or
  `receipts.json`).
- **Snapshot**: saved (`spec/.snapshot.json` updated).
- **Bead-map**: updated (`.bead-map.json` updated).
- **Run status**: `complete` or `partial` (from receipts top-level status).
  If partial, mention that the next `/converge` run will pick up unfinished
  ops via the idempotency path.

End with: "Review the diff and commit `spec/.snapshot.json` and
`.bead-map.json`. `/converge` does not auto-commit."

## Re-run semantics

If the previous `/converge` run was interrupted between adapter and ingest,
or the adapter wrote partial receipts, simply re-run `/converge` against the
same proposal. The script re-does diff/impact/emit (deterministic — same
inputs produce byte-identical changesets), the adapter sees existing
idempotency labels and writes `was_existing=true` receipts for already-applied
ops, and ingest reconciles. No duplicate beads.

If the user wants to skip emit and only retry the apply phase against an
already-produced changeset (e.g., emit was expensive and the failure was
transient), they can invoke the script directly:

```bash
scripts/run-pipeline.sh --phase apply --run-dir <existing-run-dir>
```

`/converge` itself always does the emit phase first to keep the deterministic
"diff matches spec" property fresh.

## Out of scope

- Spec authoring or correction — `/spec`, `/spec-review`, `/spec-drift`.
- Bead lifecycle hygiene (closing cleanup beads) — `/cleanup`.
- Source code review — `/review`.
- Source code implementation — `/implement`.
- Committing — the user does that after reviewing what `/converge` produced.
