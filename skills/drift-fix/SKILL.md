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
  diff whose `errors` are non-empty); the refresh path does not — `/mint`'s Step 4 names the
  one narrow override.

## Step 4: The deliberate baseline decision

This is the step that must never be automated. The doctrine and the mechanics — the direction
axis, the evidence rule, the absorb assessment and its four rules, the plan/adapter/ingest
commands, the label budgets, the refresh artifacts, and the gate-asymmetry override — are
`/mint`'s, the one skill that moves the baseline. Decide here, execute there; if this file and
`/mint` ever disagree on that doctrine, `/mint` is the copy to trust.

What triage adds on top of `/mint`'s axis is the defaults per entry mode:

- **A blocking triage mints** in all but the rare case where the fix only clarifies wording and
  the reopened bead's obligations are unchanged — the fix settles an open bead's contract, and
  the mint is what updates or replaces its pairing. New beads join the epic's remainder; if the
  fix reshaped contracts of still-open beads, the mint legitimately updates or replaces them.
- **A post-epic batch of non-blocking reports usually refreshes** — the code merged and its
  tests pin the behaviour the spec is corrected toward — but each node earns its direction
  individually: a resolution that changes the contract mints regardless of the batch around it.

State the decision and its per-node reasons in the PR description (`/mint` Step 5). A refresh
without a stated reason — or an absorb mark without one — is the "bezdumny refresh" failure
mode this skill exists to prevent.

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
