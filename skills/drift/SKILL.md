---
name: drift
description: "Triage drift reports filed by implementers (drifts/drift-*.json): validate each report, verdict it, and apply the accepted spec corrections"
argument-hint: "[drift-report-files]"
---

# /drift — Triage Implementer Drift Reports

Implementers never write `spec/`. A spec defect found mid-task arrives as a typed report under
`drifts/` (schema: `schema/drift.schema.json`) — a non-blocking report riding in its task's own
PR, a blocking one as its own PR with the task returned to `open`. This skill consumes the
reports in the authoring loop: collect, validate, verdict each, and apply the accepted
corrections. It ends with the spec corrected and the reports deleted; the baseline stays
untouched — moving it is `/mint`'s, the one skill that does.

## Step 1: Collect and verify

- Arguments name specific files, else every `drifts/drift-*.json` is in scope.
- Validate each against `schema/drift.schema.json` (field-by-field judgment — no embedded
  validator exists). A malformed report is itself a finding: fix or reject it explicitly.
- For each report, load the node's full context — `bin/spex map context <node>`, the cited
  files, the leaf of every node the citations touch — and verify the report's evidence against
  the current spec and code before any verdict.

## Step 2: Verdict each report, present, and stop

- **Accepted** — the defect is real. Classify it, because the class shapes the fix:

  | Class | Signature | Typical fix |
  |---|---|---|
  | 1 Migration shadow | a sibling field/leaf still speaks a retired vocabulary | sync the stale text; extend the retired-vocabulary list if a term registry missed it |
  | 2 Cell collision | two sections each valid alone, contradictory on one intersection | decide the cell explicitly; add the intersection to the leaf |
  | 3 Hole | a reachable case the contract does not decide | decide it; add the case and a test-section scenario covering it |
  | 4 Impossible seam | the leaf relies on a promise the provider's leaf does not make | fix whichever side is wrong: promise the seam or reroute the consumer |

- **Rejected** — the implementer misread the spec. Record why in the PR description, delete
  the report, change nothing else. If the spec was right and the shipped code disagrees with
  it, that is a code bug: file a tracker task directly and touch neither `spec/` nor the
  baseline.
- **Overtaken** — later work resolved the defect. Verify the resolution, record it in the PR
  description, delete the report.

Present the verdicts — one row per report: class and fix shape, or the rejection/overtake
ground — then stop and wait for the user's explicit go. Fixing is a second, separately
authorized act.

## Step 3: Fix the spec (after the go)

- Work on a branch off main — or, for a blocking report, the branch its PR delivered.
- Apply the agreed corrections with `/spec` discipline: ids via `bin/spex hash-id`, links in
  prose, no Go in arch leaves, completeness obligations honored.
- Scope guard: the smallest edit that decides each accepted report. Every changed node beyond
  a report's justification widens the downstream task count.
- Both gates green: `bin/spex validate` 0/0 and `bin/spex diff` with `errors: []`.

## Step 4: Report

- Delete every triaged report in the same change — the deletion is the completion marker,
  tracked in git.
- Tell the user: the verdict per report, and per changed node what changed and whether it
  records shipped, test-pinned behaviour or births work — advisory only; the classification
  itself is `/mint`'s Step 2, made against the committed diff.
- Remind them to review and commit on the working branch. The mint runs against that commit.

## Out of scope

- Writing drift reports (the implementer's protocol).
- Moving the baseline and running the pipeline (`/mint`); auditing the corrected spec
  (`/spec-review`).
- Any spec change beyond what the triaged reports justify — other findings go to
  `/spec-review`'s own flow.
