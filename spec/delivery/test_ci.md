# CI and Spec Gate Tests

Acceptance coverage for [[1c2de3dbfe1c|CIPipeline]] and [[4153dbd38133|SpecGate]] together: the trigger tiers fire the right jobs, and the spec gate inside the fast gate holds the completeness line. These are workflow-level checks exercised against a throwaway branch and pull request on the real repository (or `act`-style local runs where noted); Go unit tests do not apply to workflow YAML.

## Setup

- A branch off main carrying one trivial Go change (touches a `.go` file) and one spec change (touches a file under `spec/`), pushed as a pull request.
- A second branch carrying only a README edit (no Go, no spec).
- The built binary available to the gate job via the workflow's own build step — the gate never downloads a spex release; it builds from the PR's tree.

## Cases — trigger tiers

- Opening the PR runs the fast gate: build, vet, test, and the spec gate job. The race detector does not run.
- Merging (push to main) runs the same gate plus the race step. Race runs once post-merge, not per PR push.
- The nightly schedule runs the fuzz workflow; its log shows fuzz targets discovered by scanning for fuzz functions in the tree, not read from a list in the workflow file. Adding a new fuzz function to any package and re-running shows the new target picked up with no workflow edit.
- The README-only branch triggers no runner on either workflow.
- Pushing a second commit to the open PR cancels the first run's in-progress jobs rather than queuing behind them.

## Cases — spec gate verdicts

The gate job builds the binary, runs `spex validate`, then runs `spex diff` capturing output to a file and preserving the exit status. Three exit outcomes, all asserted:

- A PR whose spec tree is clean: validate passes, diff exits 0, the check is green.
- A PR introducing a completeness error (e.g. a requirement added with no implementing component): diff exits 2, the captured errors are surfaced in the job log, the check is red.
- A PR breaking the tree itself (unparseable module.json): diff exits 1 with nothing on stdout; the job reports the build failure distinctly rather than a JSON parse error, and the check is red.
- With branch protection marking the gate's job name required, the red-check PRs cannot be merged from the GitHub UI or API.

## Edge cases

- A spec-only PR (no Go change) still runs the gate — the paths filter must not exclude `spec/`, since the gate exists precisely for spec changes.
- `spex validate` reporting `valid` with a non-zero finding count anywhere must fail the job — the gate asserts on the JSON verdict, not on exit status alone.
- Notes (disclosures) do not fail the gate; only entries in the errors array do. That holds for both passes: a PR whose spec declares a `derivation: pending` requirement gets the structural pass's `pending_derivation` note printed in the job log — visible on every PR without gating — and the check stays green, exactly as the completeness pass's diff notes already behave.
