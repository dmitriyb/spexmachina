---
name: drift
description: "Triage drift reports filed by implementers (drifts/drift-*.json): validate each report, verdict it, and apply the accepted spec corrections — the baseline decision and the pipeline are /mint's, the audit /spec-review's"
argument-hint: "[drift-report-files]"
---

# /drift — Triage Implementer Drift Reports

Implementer boxes never write `spec/` — the gate denies it. When an implementer finds a spec
defect mid-task, it files a typed report under `drifts/` (schema: `schema/drift.schema.json`)
and either continues (non-blocking) or stops the epic (blocking). This skill is the
authoring-loop consumer of those reports: collect, validate, verdict, and apply what is
accepted. It is one step of `/drift-workflow`; the deliberate baseline decision and every
pipeline command belong to `/mint`, and the audit of the resulting spec to `/spec-review`.

Two entry modes, same procedure:

- **Mid-epic (blocking):** the epic settled with `halt_reason: blocking_drift`. The report
  reached main as its own PR — the drift file plus the bead's return to `open` in one commit,
  the PR review validating the claim itself. After the triage PR merges, the epic resumes with
  a plain `faber run epic` — the reopened or replaced bead is simply ready again.
- **Post-epic (batch):** the epic finished; `drifts/` holds non-blocking reports. Triage them
  all in one pass.

## Step 1: Collect and verify

- Arguments name specific files, else every `drifts/drift-*.json` is in scope.
- Validate each against `schema/drift.schema.json` (field-by-field judgment — no embedded
  validator exists). A malformed report is itself a finding: fix or reject it explicitly.
- For each report, load the node's full context — `bin/spex map context <node>`, the cited
  files, the leaf of every node the citations touch — and verify the report's evidence against
  the current spec and code before any verdict.

## Step 2: Verdict each report

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

Present the verdicts — one row per report: class, fix shape, or the rejection/overtake ground —
and pause for discussion before touching `spec/`.

## Step 3: Fix the spec

- Apply the agreed corrections with `/spec` discipline: ids via `bin/spex hash-id`, links in
  prose, no Go in arch leaves, completeness obligations honored.
- Scope guard: the smallest edit that decides each accepted report. Every changed node beyond
  a report's justification widens the downstream task count; findings the reports do not
  justify go to `/spec-review`'s own flow.
- Both gates green: `bin/spex validate` 0/0 and `bin/spex diff` with `errors: []`.

## Step 4: Hand back to the workflow

- Delete every triaged report — the deletion is the completion marker, tracked in git.
- For each changed node, state what changed and whether it records shipped, test-pinned
  behaviour or births work — this feeds `/drift-workflow`'s pre-commit assessment and `/mint`'s
  absorb table.
- Everything after — the audit, the commits behind their pauses, the baseline decision and the
  pipeline — is the workflow's following steps.

## Out of scope

- Writing drift reports (the implementer's protocol, in the box skills).
- The baseline move and the pipeline (`/mint`), the audit (`/spec-review`), running the epic
  (faber).
