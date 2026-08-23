# Change Proposal: Test-section deps from describes

## Context

`br ready` asserted something false during epic `spexmachina-uiei`: the task for `Lifecycle command tests` — a test section describing `InitCommand` and `DoctorCommand` — appeared ready with no blockers while neither component existed. The task was not actionable: a test importing a package that has not been written does not compile, so the first implementer to pick it off the ready list would have stalled. The epic was mitigated by hand (`br dep add`, commit `6455f4e`), which is precisely the kind of dependency this system exists to hold structurally rather than by memory.

The gap is in plan's dependency vocabulary, and it is a gap rather than a bug: [[e3b2b4e914fc|Emit DepSpecNodeIDs per create action]] collects component `uses` edges (direct) and `requires_module` edges (transitive), and [[3ec0a433e476|the data-flow contract gating]] adds component-on-data_flow deps — and that is the complete list. A test section's one edge, `describes`, is never read for deps, so a multi-component test task is minted dep-free. `ActionClassifier` (`8aa1ab5ac102`) implements the collection faithfully; the tracker state faithfully reflected the changeset. The rule is simply missing.

Why it never surfaced before: every earlier epic's multi-component test beads described components that already existed on main — tests could be written and compiled immediately, so the absence of deps was harmless. `uiei` was the first epic to mint a test section together with the brand-new components it describes.

This does not fit any open proposal in the series. The closest, `2026-08-20-declarative-profile`, reaches into the classifier but declares itself behaviour-preserving ("the default profile reproduces current behaviour exactly") and vocabulary-only ("not a place to put behaviour") — a new dep rule would break its own acceptance criterion. The change stands alone and can land in any order relative to the series; landing before declarative-profile is marginally cleaner, since that epic will read the classifier it amends.

## Proposed change

### One new collection rule, no new machinery

[[e3b2b4e914fc|Emit DepSpecNodeIDs per create action]] is amended: for a create or retarget action whose node is a test_section, the section's `describes` array is collected into `DepSpecNodeIDs`, unconditionally — alongside the existing `uses` and transitive `requires_module` collection. `ActionClassifier`'s leaf gains the rule as a fourth entry in its DepSpecNodeIDs Collection section.

Collection is deliberately unconditional rather than batch-aware, for two reasons already written into the module. First, the classifier's division of labour: `test_classification.md` D3 states "filtering already-satisfied deps is the Resolver's responsibility, not the classifier's" — no journal lookup and no bead-status filtering happens at collection time. Second, [[e9a3b1b85953|Resolver]]'s existing precedence already produces the right answer in every case with **no change to the Resolver at all**:

- described component is a create in the same batch → `ref:op` — the uiei case, the test task now waits for its components;
- described component has an open fold pairing → `ref:bead` — the test task waits for in-flight component work, which is equally correct;
- described component's pairing is closed → the dep is dropped — the steady state, where tests against existing code are writable immediately and today's behaviour is preserved;
- described component has no pairing at all → the existing plan error naming the spec_node_id. This is a disclosed consequence, not a regression to smooth over: a component no journal event has ever tracked is exactly what the error exists to surface, and component `uses` deps carry the identical exposure today.

The retarget path follows for free: [[7d45c20bd0f7|retarget ops carry freshly recomputed DepSpecNodeIDs, applied add-only]], so a retargeted test task whose described component is being re-minted in the same batch gains a `ref:op` dep on the successor through the same recomputation.

Nothing else moves. The `describes >= 2` bead gate, the fold-back rule for sections dropping to one component, the TopologicalSorter's tier order (which already emits multi-component test tasks after features — the new deps make the ordering load-bearing instead of advisory), the ref vocabulary, and the changeset schema are all unchanged. No new node types, no new ops, no version bump: a v3 consumer already resolves `ref:op`/`ref:bead` deps on any create.

### Affected nodes

| Node | Type | Change |
|---|---|---|
| Emit DepSpecNodeIDs per create action (`e3b2b4e914fc`) | module requirement, plan | description amended: describes collection added |
| ActionClassifier (`8aa1ab5ac102`) | component, plan | description and `arch_action_classifier.md` amended: fourth collection rule |
| Classification tests (`f3c4a2c35344`) | test_section, plan | `test_classification.md` gains scenarios for the four cases above |

`Resolver` (`e9a3b1b85953`), its leaf, `flow_plan.md`, and every other module are untouched.

## Impact expectation

Small epic: three creates, two closes.

- **Epic** for this proposal.
- **ActionClassifier** — its closed task is obsoleted and a successor minted (one close, one create). `arch_action_classifier.md` changes discharge the completeness obligation the `e3b2b4e914fc` description change creates; ActionClassifier is that requirement's only implementor, so nothing else is obliged.
- **Classification tests** — `describes` names two components (`ActionClassifier`, `Resolver`), so the modified test leaf mints its own task (one close, one create). The new scenarios are the checks that can actually fail: a test_section create whose described components are in-batch creates carries `ref:op` deps on them; described components with closed pairings yield no deps and no error; a retargeted test section gains the dep add-only.

The `plan/module.json` meta leaf moves (requirement description plus ActionClassifier description), suppressed by the same-module requirement change — only the implementor's leaf is obliged. Nothing is added, removed, or renamed: no coverage obligations grow, no sweep is owed, no links need repointing. `spex diff` on the end state: three modified leaves (`e3b2b4e914fc`, ActionClassifier's arch leaf, the classification test leaf) plus `meta/<plan module hash>`, exit 0 with `errors: []`.

Work outside the spec graph riding the epic: none — this is a plan-module code and test change only.
