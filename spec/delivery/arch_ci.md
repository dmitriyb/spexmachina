# CI Pipeline

Trigger-tiered GitHub Actions workflows, satisfying [[68f38bb4cc74|Tiered CI by trigger]]: every tier exists because its trigger justifies its cost, and each tier lives in its own workflow file so a trigger change in one never risks the others.

## Tiers

| Trigger | Jobs | Rationale |
|---|---|---|
| pull request | build, vet, test, and the spec gate | the fast feedback loop; everything a merge decision needs |
| push to main | the same, plus the race detector | race detection roughly doubles wall time; paying it once post-merge gives merged commits the guarantee without slowing every PR push |
| nightly schedule (plus manual dispatch) | fuzz run over discovered targets | fuzzing beyond what the per-push test step covers, off the top of the hour |

The pull-request and main tiers skip runners for changes touching neither Go code nor the spec — but the paths filter must never exclude `spec/`, because the spec gate exists precisely to judge spec changes. Each workflow declares a per-ref concurrency group with cancellation, so a superseded run is cancelled rather than queued behind.

## The spec gate seat

The fast gate delegates spec judgement to [[4153dbd38133|SpecGate]], which runs as a job inside the pull-request tier: the gate builds the binary from the PR's own tree and asserts the spec's structural and completeness verdicts. CI provides the seat — checkout, Go toolchain, the job name that branch protection marks required; the gate's verdict logic is SpecGate's contract, not this component's.

## Fuzz target discovery

The nightly tier discovers fuzz targets by scanning the tree for fuzz functions at run time. The rejected alternative — a hardcoded list of function names in workflow YAML — goes stale the moment a target is added or renamed, and its failure mode is silent (a listed name matching nothing fuzzes nothing and stays green). Discovery makes an added target picked up with no workflow edit, and an empty discovery is a loud failure.
