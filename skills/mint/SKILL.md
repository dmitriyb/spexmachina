---
name: mint
description: "Move the baseline for a committed spec change: decide mint-vs-refresh per node, assess absorb marks, then run diff → plan → adapter → ingest — the only place the baseline moves"
argument-hint: "[proposal-stem]"
---

# /mint — Move the Baseline

The authoring loop's last phase. `/spec` authors and gates, `/spec-review` audits, and this
skill moves the baseline (`.spex/snapshot.json`) — deliberately, interactively, never in a box.
It owns the doctrine the other skills reference: the direction axis, the absorb rules, the
pipeline mechanics, and the gate asymmetry. When another skill needs the baseline moved, it
sends the user here; when this file and another disagree, this one is the copy to trust and
the other is the drift.

## Preconditions

- The spec edits are committed — the pipeline runs against that commit's SHA, nothing dirty.
- `/spec`'s gates ran green on that state: `bin/spex validate` 0/0 and `bin/spex diff` with
  `errors: []` — subject to the one override below.
- The change was reviewed (`/spec-review` scoped to the touched modules, or the PR review).

## Step 1: The baseline decision — which side yields

The decision is one axis: does the corrected spec **change what the code must do**, or **record
what the code already provably does**? "Prose-only" and "contract-bearing" are proxies for that
axis and both misfire alone.

- **Code must yield to spec** — work is born (a node added or removed, a decided cell the code
  implements the other way, a hole whose decision has no implementation): **mint**, Step 3.
- **Spec yields to code** — no work is born; the correction records behaviour that is shipped,
  reviewed, and pinned by a test: **refresh**, Step 4.
- **Mixed** — mint, with the yielding nodes marked in an `--absorb` file (Step 2) so the run
  does not open tasks that owe nothing.

The spec-yields verdict is evidenced, never asserted: for every yielding node, name the test
that pins the corrected behaviour — in the PR description for a whole-run refresh, in the mark's
`reason` for an absorb. A pinning test that does not exist yet does not force a mint: write it
in the same PR, and it counts once green — it merges together with the text it pins.

## Step 2: Absorb assessment — before any pipeline command runs

Walk every `modified` entry in `bin/spex diff --json` and classify each node explicitly. Present
the result to the user as a table — node, verdict (mint | absorb), reason, pinning test — and
get their confirmation **before** running `spex plan`. An unclassified node mints, which is the
safe direction; the table exists so no node is absorbed by omission or minted by inattention.

`--absorb` names a git-committed JSON list of `{node, reason}`. A marked node's change is
withheld before matching, so it yields no op at all — no create, no close, no retarget, no
claimed-task refusal — and rides in the changeset's `absorbed` array instead; `ingest` mints its
`modified` event (keyed `refresh:<node>:<before>:<after>`, the same scheme a whole-run refresh
uses) and the batch's single non-task-bearing `refresh` receipt names every absorbed eid the
journal did not already carry. It is the per-node form of the Step 4 refresh. This repository
keeps the file at `.spex/runs/absorb.json`; any git-committed path works.

`spex plan` checks exactly two things about a mark: the node is a 12-character hex identity
hash, and the diff reports it as `modified` — anything else is exit 2 naming the node. Whether
the node yields is **not checked and cannot be**; these four rules are the whole of it:

1. **Declare per node what changed.** Every mark's `reason` names the sections the edit touched
   and the pinning test Step 1's evidence rule requires. A reason that restates the node name,
   or says only "prose-only", is not a declaration.
2. **Contract-bearing content yields only to a pinning test.** An Interface or Responsibilities
   section, any table stating a contract (flags, exit codes, states, field lists, vocabularies):
   if the edit changes what the code must do, no reason makes it absorbable; if it converges the
   text onto pinned behaviour, the named test is what makes it eligible. A `module.json` change
   is never absorbable in either direction — module meta is structure, not prose.
3. **The presumption is mint.** Absorption is the exception that has to be argued; silence never
   skips: an unmarked node mints.
4. **The reviewer validates the declaration**, not merely its presence. The absorb file in the
   PR diff and the reasons in the journal are the entire audit trail.

Rule 2's carve-out is deliberately narrow, and the record shows why: on 2026-08-16 a triage run
absorbed five contract-bearing leaves on a bare "owes no code" — nothing pinned the claimed
compliance, the changes did owe code, and the mint had to be reverted and re-run as eight tasks.
Nothing enforces rules 1–4: `spex plan` validates shape only, and skill prose is advisory by
construction. Making any of them binding is a change to `spex` behaviour and needs a proposal
first; until one lands, the PR review is the only check there is.

## Step 3: Mint

Registration comes first: `spex plan` resolves the epic through a `registered` event carrying
the proposal stem (or a live epic task), and fails without one. Check the journal; if neither
exists, run

```
bin/spex register spec/proposals/<stem>.md --git-head <sha7>
```

with the **same 7-character SHA** the plan invocation below uses — the spec-edits commit — and
commit the appended journal event with the run's artifacts. The Registrar validates the
template's H2 sections and refuses a malformed proposal. The event's eid `<git_head>:<stem>`
becomes the epic's idempotency label, which is why `/propose` caps the slug at 26 characters.

```
bin/spex diff --json > diff.json
<adapter export> → tasks.json                 # scripts/export-br.sh tasks.json is the reference
bin/spex plan --proposal <stem> --git-head <sha> --diff diff.json \
              --tasks tasks.json [--absorb <file>] --out changeset.json
<adapter apply> changeset.json → receipts.json   # scripts/apply-br.sh is the reference
bin/spex ingest --changeset changeset.json --receipts receipts.json
```

The export runs before **every** plan, never once per session: the task-state artifact is
required, and it describes the tracker at the moment it was written. A run that reads a stale
one retargets or closes tasks the tracker no longer holds live, or misses a claim made since.

| `spex plan` flag | Required | What it is |
|---|---|---|
| `--proposal` | yes | the proposal stem, without `.md`; the epic resolves through it — a live epic task in the journal fold first, else a `registered` event with the same stem, else the run fails |
| `--git-head` | yes | the commit carrying the spec edits — hand it a **7-character short SHA** (label budget below) |
| `--diff` | no | `spex diff --json` output; stdin by default |
| `--tasks` | yes | the task-state artifact, `{"version":1,"tasks":[{"task_id":…,"status":…}]}` with `status` ∈ {`open`, `in_progress`} (`schema/task-state.schema.json`). It lists **in-flight tasks and nothing else** — a handful mid-epic, `[]` after one, and an empty list is a legal, explicit "nothing in flight". The adapter's export half writes it from the tracker (for br: `scripts/export-br.sh tasks.json`, the listing filtered to open/in_progress and projected to two fields); spex never runs the tracker itself, and never reads the raw listing |
| `--absorb` | no | the Step 2 file |
| `--out` | no | changeset path; stdout by default |

Exit codes: `0` changeset written; `1` input validation (bad flags, malformed JSON, a diff that
still carries errors — `plan: diff contains N error(s), refusing to proceed` — or a missing,
unreadable or wrong-version `--tasks` document); `2` contract refusal (a claimed task's node
changed, an invalid absorb entry, a dep cycle, an unresolvable dep or parent); `3` not a spex project — the lifecycle pre-flight's own
code, uninitialised (naming `spex init`) or broken snapshot/journal (naming `spex doctor`). A
malformed journal never takes `1`: the pre-flight refuses before the fold reads anything.

**The lifecycle plan decides.** A pairing's task is in one of exactly three states — `open` or
`in_progress` in the artifact, or absent from it — and absence *means* finished: completion is
derived from the journal pairing plus absence, never carried as a status. Two decisions follow
(`spec/plan/arch_action_classifier.md` holds the table):

| Change | Task `open` | Task `in_progress` | Task absent |
|---|---|---|---|
| added / modified, hash unrecorded | **retarget** the task | **refuse the run** | one plain **create** |
| removed | **close** the task | **refuse the run** | **create** a cleanup task |

The create on an absent task is plain: no close of the predecessor, no old task id carried, no
`blocks` dependency onto the finished task. Generation history is the journal's event chain,
which `spex map context` surfaces — the changeset never encodes it. Closing is reserved for
live work; there is no obsolete+create path and no no-op close against a finished task.

Changeset v4 vocabulary: ops are `create`, `close`, `retarget` — `obsolete` is not in it;
`spec_node_kind` on a create is `proposal_epic`, `component`, `data_flow`, `test_section` or
`cleanup` (no `api`, no `requirement` — neither produces a task); refs are `{"ref":"task",
"task_id":…}` or `{"ref":"op","op_id":…}`, resolved at build time; `absorbed` is a top-level
array beside `ops`, never an op. Receipts are v2: every entry is keyed `task_id`. Inside the
spec corpus and these skills the word is task(s), everywhere; the tracker's own name for its
objects enters only through the reference adapter's command strings (`br create`, `br list`).

**The 50-character label budget.** br rejects any label over 50 characters, in the same
`br create` as the task, so an overflow fails the create outright mid-mint. Two labels are the
author's to size: the epic's `spex:<git_head>:<proposal_stem>` (fixed at `spex register` — the
stem budget is why `/propose` caps slugs at 26), and every op's `spex:<git_head>:op-NN` — a
40-character SHA overflows at ten ops, the 7-character abbreviation never does. Nothing enforces
these budgets until br fails partway through.

Exit 2 on a claimed task is the reason to mint a module's changes in one run rather than
dribbling them across several while tasks are in flight.

## Step 4: Refresh

For the pure spec-yields run — every node's correction pinned, no ops owed:

```
bin/spex ingest --mode refresh --changeset <empty> --receipts <empty> [--git-head <sha>]
```

Both artifacts are required and must be empty: `{"version":4,"ops":[],"absorbed":[]}` and
`{"version":2,"status":"complete","ops":[]}` — the same versions a mint's changeset and receipts
carry; ingest refuses any other (`changeset version must be 4`, `receipts version must be 2`). Refresh walks the current graph itself, absorbs
every drift entry as a `modified` event, writes one refresh receipt (stamped with `--git-head`
when given), and commits journal and snapshot under one atomic boundary. It refuses non-empty
artifacts, and added or removed `meta`/`module` leaves; every other change kind — content drift,
and structural additions and removals of any node type the resolved profile declares absorbable,
which the default profile does for all of them in both directions — is absorbed, each as its own
`added`/`removed`/`modified` event under the one refresh receipt. Refresh is the authoring loop's
deliberate override: the reason for absorbing a structural change (the tests landed in the same
change; what the removed leaf described were unit tests that stay) goes in the PR description.

**The gates bind asymmetrically.** `spex plan` refuses a red diff structurally; refresh runs
neither `spex validate` nor the completeness checker, so a diff flagged with `incomplete_change`
is still refresh-landable — an override, not an impossibility, and never a silent one. It is
legitimate only when the flagged obligation is discharged by hand — name the entry that actually
changed and why every flagged sibling owes nothing — and recorded in the PR description. The
recurring case is a `module.json` description sync: the meta sweep deliberately obliges every
component in the module, so a per-entry correction can never show a green diff and rides on the
hand-verification instead.

## Step 5: Verify and hand over

- `bin/spex diff` → `no changes`; `bin/spex validate` → 0/0.
- The journal tail carries the run's events and its receipt; the snapshot moved.
- Commit the baseline (`.spex/snapshot.json`, `.spex/history.jsonl`) — plus, for a mint, the
  changeset/receipts artifacts if the repo keeps them.
- State the baseline decision and its per-node reasons in the PR description. A refresh or an
  absorb mark without a stated reason is the failure mode this skill exists to prevent.

## Out of scope

- Authoring spec files (`/spec`) or auditing them (`/spec-review`).
- Drift triage — `/drift` decides what a drift report is worth; `/drift-workflow` comes here
  to execute the baseline move.
- Running the epic (faber's job).
