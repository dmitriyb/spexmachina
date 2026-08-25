---
name: drift-fix
description: "Triage drift reports filed by implementers (drifts/drift-*.json), fix the spec through review, make the deliberate mint-or-refresh decision, and clear the reports"
argument-hint: "[drift-file ...]"
---

# /drift-fix — Triage Implementer Drift Reports

Implementer boxes never write `spec/` — the gate forbids it. When an implementer finds a spec
defect mid-task, it files a typed report under `drifts/` (schema: `schema/drift.schema.json`)
and either continues (non-blocking) or stops the epic (blocking). This skill is the other half
of that contract: the **authoring-loop** session that consumes the reports. It is deliberately
interactive — the baseline (snapshot) moves only here, only on purpose, never in a box.

Two entry modes, same procedure:

- **Mid-epic (blocking):** the epic settled with `halt_reason: blocking_drift` (the sentinel is
  emitted by the faber-side epic hook configured in dot). The report reaches main through its own PR: the
  implementer commits ONLY `drifts/drift-<bead>.json` plus the bead's return to `open` (undoing
  the claim) in the same commit, and that PR is reviewed and merged like any other — the reviewer
  is validating the drift claim itself. After THIS skill's fix-PR merges, the epic resumes with a
  plain `faber run epic` — no `faber resume`, nothing failed, the reopened (or mint-replaced)
  bead is simply ready again.
- **Post-epic (batch):** the epic finished; `drifts/` holds non-blocking reports. Triage them all
  in one pass.

## Step 1: Collect and validate

- `$ARGUMENTS` names specific files, else every `drifts/drift-*.json`.
- Validate each against `schema/drift.schema.json` (no embedded validator exists — use any JSON
  Schema checker at hand, or field-by-field judgment against the schema). A malformed report is
  itself a finding — fix-or-reject it explicitly, never silently skip it.
- For each report, load the full spec context of its node: `bin/spex map context <node>`, the
  cited files, and the leaf of every node the citations touch.

## Step 2: Classify each report

Use the four-class defect taxonomy; the class determines the shape of the fix:

| Class | Signature | Typical fix |
|---|---|---|
| 1 Migration shadow | a sibling field/leaf still speaks a retired vocabulary while neighbors were updated | sync the stale text; extend the retired-vocabulary sweep if a term registry missed it |
| 2 Cell collision | two sections each valid alone, contradictory on one input/mode intersection | decide the cell explicitly; add the intersection to the leaf (table row or sentence) |
| 3 Hole | a reachable case the contract does not decide | decide it; add the case AND a test-section scenario covering it |
| 4 Impossible seam | the leaf relies on a dependency contract the provider's leaf does not promise | fix whichever side is wrong: promise the seam in the provider or reroute the consumer |

A report may also be **wrong** — the implementer misread the spec. Rejecting it is a valid
verdict: record why in the PR description, delete the report, change nothing else. A rejected
report can still surface a code defect: if the spec was right and the shipped code disagrees
with it, that is a bug in the code — file a tracker bead directly and touch neither `spec/` nor
the baseline; with no spec diff there is nothing to mint or refresh. A report may also be
**overtaken** — the defect it describes was resolved by work that landed after it was filed;
verify the resolution, record it in the PR description, delete the report.

## Step 3: Fix the spec

- Branch off main. Apply the corrections with `/spec` discipline (ids via `bin/spex hash-id`,
  links in prose, no Go in arch leaves, completeness obligations honored).
- Run `/spec-review` scoped to the touched modules. Fix its findings.
- Both gates green: `bin/spex validate` 0/0 and `bin/spex diff` with `errors: []`. Know where
  this rule is enforced and where it is prose: the mint path enforces it (`spex plan` refuses a
  diff whose `errors` are non-empty); the refresh path does not — step 4 names the one narrow
  override.

## Step 4: The deliberate baseline decision

This is the step that must never be automated. The decision is one axis — **which side yields**:
does the corrected spec change what the code must do, or record what the code already provably
does? "Prose-only" and "contract-bearing" are both proxies for that axis, and both misfire alone.
The spec-yields verdict is evidenced, never asserted: for every corrected node, name the test
that pins the corrected behaviour — in the PR description for a whole-run refresh, in the mark's
`reason` for an absorb. A pinning test that does not exist yet does not force a mint: write it in
the same PR, and it counts once green — it merges together with the text it pins. Look at what
the fix changed:

- **Code must yield to spec** — work is born (a node added/removed, a decided cell the code
  implements the other way, a hole whose decision has no implementation): run the mint —
  `diff → plan → <adapter> → ingest`, the adapter being `scripts/apply-br.sh` or another
  implementation of the adapter contract. `spex plan` takes `--proposal`, `--git-head` and
  `--beads`: the SHA is the commit carrying the spec edits, so the corrections are committed
  before the mint runs against them, and the tracker listing (`br list --json`) is what the
  cleanup gate and the retarget split read live status from — omitting it defaults the cleanup gate
  closed and sends each *matched* modified node (one the journal already pairs with a task, whose
  new hash the journal does not already record) down the obsolete+create path.
  Hand `--git-head` a **7-character short SHA**: every node-bearing create's idempotency label is
  `spex:<git_head>:op-NN`, br rejects any label over 50 characters (`Error: Validation failed:
  label: exceeds 50 characters`, exit 4), and the label rides in the same `br create` as the bead,
  so a 40-character SHA fails the create outright on any batch of ten ops or more. The epic's own
  label is not plan's to fix — it is `spex:<git_head>:<proposal_stem>` from the `registered` event,
  settled back at `spex register`. New beads join the epic's remainder; if the fix reshaped contracts of still-open beads, the
  mint legitimately updates or replaces them.
- **Spec yields to code** — no work is born; every corrected statement has its pinning test:
  refresh —
  `bin/spex ingest --mode refresh --changeset <empty-v3> --receipts <empty-v1>` (both artifacts
  required and must be empty: `{"version":3,...,"ops":[],"absorbed":[]}` /
  `{"version":1,"status":"complete","ops":[]}`), optionally `--git-head <sha>`. Absorbs the
  correction into the baseline with journal events. Both artifacts key their array `ops` —
  receipts included, per `adapters/types.go` — and an unknown key is silently ignored rather
  than rejected, so a misspelling reads as an empty array and passes the empty case while losing
  every entry in a non-empty one.
  The gates bind asymmetrically here: refresh runs neither `spex validate` nor the completeness
  checker, so a diff that `spex diff` flags with `incomplete_change` is still refresh-landable —
  an override, not an impossibility, and never a silent one. It is legitimate only when the
  flagged obligation is discharged by hand — name the entry that actually changed and why every
  flagged sibling owes nothing — and recorded in the PR description. The recurring case is a
  `module.json` description sync: the meta sweep deliberately obliges every component in the
  module (arch_completeness_checker.md), so a per-entry correction can never show a green diff
  and rides on the hand-verification instead.
- Mixed changes: mint — the pipeline computes both sides; refresh is only for the all-nodes-yield
  case. Mark the yielding nodes in an `--absorb` file so the mint does not open beads that owe
  nothing.

A blocking triage mints in all but the rare case where the fix only clarifies wording and the
reopened bead's obligations are unchanged — the fix settles an open bead's contract, and the mint
is what updates or replaces its pairing. A post-epic batch of non-blocking reports usually
refreshes, but each node earns its direction individually: a resolution that changes the contract
mints regardless of the batch around it.

### What may be absorbed

`--absorb` takes a git-committed JSON list of `{node, reason}`. A marked node's change is
withheld before matching, so it yields no op at all — no retarget, no obsolete+create, no
claimed-task refusal, whatever the state of the task tracking it — and rides in the changeset's
`absorbed` array instead; ingest mints its `modified` event — keyed by the same
`refresh:<node>:<before>:<after>` scheme a whole-run refresh uses — and the batch's single
non-task-bearing `refresh` receipt names every absorbed eid the journal did not already carry (on an
idempotent re-run that carries them all, no receipt is written). It is the per-node form of the refresh
decision above.

`spex plan` checks exactly two things about a mark: that the node is a 12-character hex identity
hash, and that the diff reports it as `modified` — a mark on an `added` or `removed` node, or on
one absent from the diff, is exit 2 naming the node. **Whether the node's edit yields work is
not checked and cannot be**: it is decided here, under these four rules, and nothing downstream
will catch a wrong mark.

1. **Declare per node what changed.** Every mark carries a `reason` naming the sections the edit
   touched and the pinning test the step-4 evidence rule requires. A reason that restates the
   node name, or says only "prose-only", is not a declaration.
2. **Contract-bearing content yields only to a pinning test.** An Interface or Responsibilities
   section, any table stating a contract (flags, exit codes, states, field lists, vocabularies):
   if the edit changes what the code must do, no reason makes it absorbable; if it converges the
   text onto pinned behaviour, the named test is what makes it eligible. A `module.json` change
   is never absorbable in either direction — module meta is structure, not prose.
3. **The presumption is mint.** Absorption is the exception that has to be argued; silence never
   skips: an unmarked node mints, which is the safe direction.
4. **The reviewer validates the declaration**, not merely its presence — the same posture a
   blocking drift report gets. The absorb file in the PR diff and the reasons in the journal are
   the entire audit trail.

Rule 2's carve-out is deliberately narrow, and the record shows why: on 2026-08-16 a run of this
skill absorbed five contract-bearing leaves (MappingStore, ChangesetBuilder, the plan flow,
ActionClassifier, the mapping store tests) on a bare "owes no code" — nothing pinned the claimed
compliance, the changes did owe code, and the mint had to be reverted and re-run as eight beads.
Nothing enforces rules 1–4: `spex plan` validates shape only, "git-committed and reviewed" is
doctrine rather than a gate, and skill prose is advisory by construction. Making any of them
binding is a change to `spex` behaviour and needs a proposal first; until one lands, the PR
review is the only check there is.

State the decision and its reason in the PR description. A refresh without a stated reason — or
an absorb mark without one — is the "bezdumny refresh" failure mode this skill exists to prevent.

## Step 5: Clear and hand over

- Delete every triaged report in the same branch — the deletion is the completion marker,
  tracked in git.
- Commit(s) per the repo's PR flow; hand the user a PR title and one-paragraph description
  covering: which reports, class per report, fix per report, the baseline decision and why,
  any rejected reports and why.
- The user opens and merges the PR. If this was a blocking triage, the epic continues with
  `faber run epic` afterwards.

## Out of scope

- Writing drift reports (that is the implementer's protocol, in the box skills).
- Running the epic (faber's job).
- Any spec change beyond what the triaged reports justify — unrelated findings go to
  `/spec-review`'s own flow.
