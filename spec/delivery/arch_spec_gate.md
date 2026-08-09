# Spec Gate

The CI job that makes spex self-hosting rather than merely tested, satisfying [[07bbd73df14f|Spec gate on every PR]]. On every pull request, after building the binary from the PR's tree, the gate runs the structural pass and then the completeness pass — and the second is the one that matters: the structural pass succeeds on a tree whose spec edit silently broke the graph it claims to describe, because incomplete-change findings surface only in the diff's errors, never in validation. Without the diff assertion the job is decorative. This closes the defect the declarative-spec-contracts migration identified as "nothing runs the checks" — the check was withdrawn from that proposal and specified here, its only home.

## Verdict contract

The gate asserts on both passes:

- The structural pass must report a valid tree with zero findings — the gate reads the JSON verdict, not the exit status alone.
- The completeness pass is judged by the diff exit-code contract: 0 means the tree built and the errors array is empty (green); 2 means completeness errors — each one surfaced into the job log verbatim (red); 1 means the tree did not build at all, and is reported as a build failure, distinctly (red).

Two implementation constraints, both learned the hard way:

- **Never pipe the diff into a JSON filter.** The pipe discards the diff's own exit status, and on a build failure the diff writes nothing to stdout — the filter then dies with an opaque parse error naming no file, while a hypothetical clean-but-broken run would pass. The gate captures output to a file, preserves the status, and branches on all three cases.
- **"Required for merge" is a branch-protection rule, not something the workflow can declare.** The status check marked required is the gate's job name; without the rule, a red gate is advisory.

Diff notes are disclosures, not violations: they never gate the verdict and are left visible in the log.
