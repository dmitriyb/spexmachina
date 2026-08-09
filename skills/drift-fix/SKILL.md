---
name: drift-fix
description: "Triage drift reports filed by implementers (drifts/*.json), fix the spec through review, make the deliberate mint-or-refresh decision, and clear the reports"
argument-hint: "[drift-file ...]"
---

# /drift-fix — Triage Implementer Drift Reports

Implementer boxes never write `spec/` — the gate forbids it. When an implementer finds a spec
defect mid-task, it files a typed report under `drifts/` (schema: `schema/drift.schema.json`)
and either continues (non-blocking) or stops the epic (blocking). This skill is the other half
of that contract: the **authoring-loop** session that consumes the reports. It is deliberately
interactive — the baseline (snapshot) moves only here, only on purpose, never in a box.

Two entry modes, same procedure:

- **Mid-epic (blocking):** the epic settled with `halt_reason: blocking_drift` (the sentinel
  vocabulary ships with the faber-side config in dot; until that lands, a blocking report simply
  means "do not run the epic further"). The report reaches main through its own PR: the
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
verdict: record why in the PR description, delete the report, change nothing else.

## Step 3: Fix the spec

- Branch off main. Apply the corrections with `/spec` discipline (ids via `bin/spex hash-id`,
  links in prose, no Go in arch leaves, completeness obligations honored).
- Run `/spec-review` scoped to the touched modules. Fix its findings.
- Both gates green: `bin/spex validate` 0/0 and `bin/spex diff` with `errors: []`.

## Step 4: The deliberate baseline decision

This is the step that must never be automated. Look at what the fix changed:

- **Work is born** (a node added/removed, a contract change that owed code): run the mint —
  `diff → impact → emit → apply-br → ingest`. New beads join the epic's remainder; if the fix
  reshaped contracts of still-open beads, the mint legitimately updates or replaces them.
- **No work is born** (prose-only correction; code already complies): refresh —
  `bin/spex ingest --mode refresh --changeset <empty-v2> --receipts <empty-v1>` (both artifacts
  required and must be empty: `{"version":2,...,"ops":[]}` / `{"version":1,"status":"complete",
  "receipts":[]}`), optionally `--git-head <sha>`. Absorbs the correction into the baseline with
  journal events.
- Mixed changes: mint — the pipeline computes both sides; refresh is only for the pure-correction
  case.

State the decision and its reason in the PR description. A refresh without a stated reason is
the "bezdumny refresh" failure mode this skill exists to prevent.

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
