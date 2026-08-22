# Change Proposal: Requirement derivation status

## Context

`spex validate` treats an underived project requirement as a hard error. `RequirementCoverageChecker` (`c7d0282b0e05`) walks every entry in `project.json`'s `requirements[]`, scans all module requirements for a matching `preq_id`, and reports any requirement with no deriver at path `project.json` — the first link of the two-link chain that [[168ae8fde8e2|requirement coverage validation]] asks the spec to hold.

That rule is correct in a spec that is finished, and it is the reason a spec cannot be started incrementally. Coverage in this project is *relative*, not absolute: nothing forces a spec to describe a whole repository, and a single-module spec validates cleanly today. What cannot be expressed is a `project.json` that honestly names the whole system while only part of it has been decomposed. The author must either under-declare the project — omitting requirements that genuinely exist, so the map is silent about its own gaps — or declare them and stay red until every one has a module behind it. Omission is the cheaper option and the worse one, because a gap that is absent from the spec cannot be reviewed, counted or planned.

This is the first of an agreed series of eight proposals aimed at making spex adoptable by projects other than its own. It is the entry point for a project adopting spex over an existing codebase: declare the system shallowly at project level, derive only the module in flight, and let the unspecced remainder be visible as a declared state rather than hidden by omission. Proposals in this series refer to each other by stem and never refer forward; this one has no dependencies.

One constraint shapes the design. `spex validate` has no non-error output channel. [[608f8ca2e1b0|Structured error output]] is a four-key document — `valid`, `error_count`, `warning_count`, `errors` — in which `severity` is always `"error"` and no checker can emit any other value; `warning_count` is a stable part of the contract, permanently `0`, retained because gates assert on it. `spex diff` already has the channel this change needs: a `notes` array of disclosures that never gate the verdict, which `SpecGate` (`4153dbd38133`) prints for the completeness pass and not for the structural one. A declared gap is exactly such a disclosure, so the channel is extended to `validate` rather than reviving warnings, which were removed deliberately.

## Proposed change

### schema — declare the field

`Define project schema` (`f471f2764ab8`) is modified to cover an optional `derivation` property on a project requirement, whose only permitted value is `pending`. The field is non-default: a requirement without it is an ordinary requirement that must be derived. `$defs.requirement` in `schema/project.schema.json` sets `additionalProperties: false`, so the property must be declared there or every spec carrying it fails schema conformance.

The field is project-scoped only. A module requirement already carries a required `preq_id` and derives by construction, so the state has no meaning there and the module schema is untouched. `ProjectSchema` (`79946d618829`) is the component that changes.

### validator — a declared gap is a note, not an error

`Requirement coverage validation` (`168ae8fde8e2`) is modified so the project-level pass skips a requirement declaring `derivation: pending` and emits a disclosure in its place. The message names the requirement's identity hash and its quoted title, in the same shape the existing error uses. Nothing else in the checker moves: an underived requirement that has *not* declared the state is still an error with its current message, and the module-requirement-to-component link is untouched in both wording and behaviour. `RequirementCoverageChecker` (`c7d0282b0e05`) is the component that changes.

### validator — the report gains a disclosure channel

[[608f8ca2e1b0|Structured error output]] is modified so the report carries a `notes` array alongside `errors`, mirroring the disclosure channel `spex diff` already publishes. Three properties are load-bearing and belong in the arch leaves:

- Notes never affect `valid`, `error_count` or the exit code. A spec whose only finding is a declared gap validates, and `spex validate` exits 0.
- The array is omitted when empty, so a clean run emits the same four-key document it emits today and no existing consumer observes a change.
- `warning_count` stays in the contract and stays `0`. A note is not a warning, and the statement that no checker emits a severity other than `"error"` remains true — a note is not a validation entry and carries no severity.

`ErrorReporter` (`0f98ca780873`) composes and writes the report; `ValidateCommand` (`59235a75aa44`) derives its exit status from it. Both are components that change.

### merkle — state the exclusion

`arch_tree_builder.md`'s "Requirement Leaf Hashing" section enumerates the serialized fields of a project-level requirement as `depends_on`, `description`, `id`, `priority`, `title`, `type`. `derivation` is deliberately absent from that list, and the leaf must say so rather than leave the omission to be inferred, because the enumeration is what a future implementer reads.

The rationale is the point of the decision. Graduating a requirement from `pending` to derived produces no content change, therefore no diff entry, no impact and no absorb entry. The derivation itself is already visible: it arrives as added module requirements and added components, which are the nodes that carry the work. Hashing the field as well would mint a modification on the parent requirement every time a module is decomposed — noise on a node that did not change in substance, and one more absorb entry to write by hand. `TreeBuilder` (`dfe1467b7a4b`) is the component whose leaf changes; `merkle/module.json` is untouched.

### delivery — surface the disclosure in CI

`SpecGate` (`4153dbd38133`) prints the structural pass's notes as it already prints the completeness pass's, so a declared gap is visible in the job log of every PR without gating the verdict. The gate's rule that a `valid: true` report with a non-zero finding count still fails the job is unchanged; notes are not findings. `delivery/module.json` is untouched.

### What is deliberately not changed

Project requirement `Validate spec structure` (`d5a8407d38e1`) says coverage holds in both directions, "every requirement derived and implemented". Its description is not amended. Under this change the invariant still holds for every requirement that has not declared otherwise, and a declared exemption recorded in the spec is not the same as a silent gap — the project requirement states the rule, and the module requirement that owns it states the exemption.

The cost of the alternative is the reason to state this explicitly rather than leave it as an oversight. Touching that description walks the completeness rule from the project requirement to all thirteen module requirements deriving from it — three in `schema`, ten in `validator` — and then to every component implementing any of them, obliging a changed content leaf from each. That is most of two modules rewritten to record a nuance already recorded in the requirement where the rule lives.

## Impact expectation

### Components modified — one bead each

| Module | Component | Hash | Leaf |
|---|---|---|---|
| schema | ProjectSchema | `79946d618829` | `arch_project_schema.md` |
| validator | RequirementCoverageChecker | `c7d0282b0e05` | `arch_requirement_coverage_checker.md` |
| validator | ErrorReporter | `0f98ca780873` | `arch_error_reporter.md` |
| validator | ValidateCommand | `59235a75aa44` | `arch_validate_command.md` |
| merkle | TreeBuilder | `dfe1467b7a4b` | `arch_tree_builder.md` |
| delivery | SpecGate | `4153dbd38133` | `arch_spec_gate.md` |

### Test sections modified — one bead each

Every one of these names two or more components in `describes`, so each produces a bead.

| Module | Test section | Hash | Covers |
|---|---|---|---|
| validator | Name and coverage tests | `15d919047039` | the checker's new skip and its unchanged error path |
| validator | Validation pipeline tests | `dad7c5e68169` | report shape with and without notes, exit code unaffected |
| schema | Schema validation tests | `96f944302b78` | the property accepted, an unknown value rejected |
| merkle | Hashing tests | `31b096a76fd4` | a requirement's hash unchanged when the field is added or graduated |
| delivery | CI and Spec Gate Tests | `191f87d4981b` | the gate prints structural-pass notes and still passes |

The merkle test is the one that can actually fail if the exclusion is implemented wrongly: it asserts the leaf hash of a project requirement is identical with the field present, absent, and removed.

### Requirements modified

`f471f2764ab8` (schema), `168ae8fde8e2` and `608f8ca2e1b0` (validator). Each is a description change on a module requirement, which obliges a changed content leaf on every component implementing it — satisfied by the component table above: `f471f2764ab8` by ProjectSchema, `168ae8fde8e2` by RequirementCoverageChecker, `608f8ca2e1b0` by both ErrorReporter and ValidateCommand.

### Completeness rules that apply

- `schema/module.json` and `validator/module.json` both change, moving each module's `meta` leaf. The meta rule — every component in the module owes a changed leaf — is suppressed in both, because a requirement in each module also changed.
- `merkle/module.json` and `delivery/module.json` do not change. Both edits are content-leaf-only modifications and move no meta leaf, so neither module owes anything beyond the leaf named above.
- No node is renamed, so nothing is delete-plus-create and no inbound link needs rewriting.
- Nothing is retired, so no corpus sweep is owed.

### Scope

Eleven beads under one epic: six component leaves, five test sections. Implementation is a property added to one JSON Schema, a skip and a disclosure in one checker, an omitted-when-empty array on the report struct, one `print_notes` call in `scripts/spec-gate.sh`, and no change at all to `merkle/tree_builder.go` — the field allowlist in `hashRequirement` already excludes anything not named in it, which is what makes the exclusion free rather than a change to defend.
