# Change Proposal: spex check

## Context

`scripts/` holds fifteen non-test shell scripts. Four of them are not checks: `apply-br.sh` and `export-br.sh` are the two halves of the reference adapter, `check-lib.sh` is a sourced library, and `spec-gate.sh` is the CI seat that runs `spex validate` and `spex diff` and asserts on their JSON verdicts. The other **eleven are mechanical spec checks**, and only two of them run in CI: `.github/workflows/pr.yml` and `main.yml` invoke `spec-gate.sh`, `lens-test-shape.sh` and `lens-requirement-links.sh`, and nothing else. The remaining nine run because a skill's prose asks an agent to run them, and one of them, `no-rename-check.sh`, is named by no skill, no document and no workflow at all — a check that exists and is never run.

That works in the repository that grew the tool, where one person absorbs the discipline personally. It does not survive contact with anyone else: a check that would have caught their mistake silently does not run, they ship the mistake, and the tool gets the blame for not catching it. This project's own position is that a gate binds and prose is advisory — [[07bbd73df14f|the spec gate on every PR]] exists precisely because a documented intention is not an enforced one. Nine checks living outside that gate is the doctrine's own violation, and it is the last one of any size. The two that did reach CI reached it as separate workflow steps, each with its own exit convention, which is how the number of gate steps grows by one every time a check is written.

The scripts are also written in bash, which has shaped how they are thought about. Being written in bash is not evidence that a check needs a shell: `grep`, `jq` and `awk` are how a shell script reads files and matches strings, and a binary does both natively. Every one of the eleven is computable in Go over the same inputs.

Two of them compare a leaf against its previous version. That is the only place an outside tool appears, and the scripts already resolved it: `check-lib.sh`'s `base_resolve` accepts **a directory or a git ref**, and `heading-check.sh` states that "out-of-repo trees are first class … needs no git repository anywhere. Only the default base is a git ref and only that needs one." The baseline was always a directory; git is merely the convenient way to produce one. So nothing here reaches for git, no dependency is added to [[96c6c15ecc3e|the declared stack]], and [[58ea35f52b86|no runtime subprocesses]] is untouched — the baseline arrives as a caller-supplied path, exactly as `diff.json` and `--tasks` already do.

This proposal depends on `2026-08-20-declarative-profile`, which has landed. Half these checks walk the graph by type and edge — which types own content leaves, which edges a node declares — and they read the resolved profile for it, so folding them into the binary bakes no ontology in. It is second in the remaining series, after `2026-09-14-authoring-commands`: the authoring commands are tested for parity against the bash scripts while those still exist, and the checkers here re-pin that parity in Go.

## Proposed change

### One command, eleven checks and one new one, all in the binary

A new api `spex check`, provided by a new component `CheckCommand` in the `validator` module. It runs every mechanical check and emits one report.

`validator` owns it because these are spec-directory validation, which is what the module is for. Its description broadens from schema-and-graph conformance to include the corpus and baseline-relative checks, which is an honest widening rather than a stretch: `RequirementCoverageChecker` and `TestCoverageChecker` already live there and are graph-walking rather than schema-checking.

`spex check` stays separate from `spex validate` rather than absorbing it. They have different input contracts: validate needs only the spec directory and is the structural pass every other command depends on; check takes a diff, a journal and optionally a baseline, and is a review-time pass. Merging them would give `spex validate` inputs it does not need in order to answer a question nobody asked it.

### The parity matrix

Each script becomes one component, following the module's established one-checker-one-component pattern:

| Script | Catches | Becomes | Inputs beyond the spec |
|---|---|---|---|
| `link-check.sh` | a touched leaf that does not link its node's declared edges | `LinkChecker` | the diff |
| `no-rename-check.sh` | a surviving id whose `name` changed | `RenameChecker` | the journal |
| `lens-dissolved-modules.sh` | references to modules that no longer exist | `DissolvedModuleChecker` | the journal, judgement records |
| `lens-lexicon.sh` | retired vocabulary still live in the corpus | `LexiconChecker` | proposals, judgement records |
| `lens-usage-strings.sh` | a `--flag` in a JSON description absent from the owning arch leaf | `UsageStringChecker` | — |
| `lens-counts.sh` | written-out counts beside the graph's actual numbers | `CountChecker` | — |
| `lens-test-shape.sh` | a unit-shaped scenario in a test leaf — one that calls a Go identifier and invokes no `spex` subcommand | `TestShapeChecker` | judgement records |
| `lens-requirement-links.sh` | a module requirement that no implementing component's arch leaf links by id | `RequirementLinkChecker` | — |
| `lens-requirements.sh` | the worksheet pairing each requirement with the leaves that carry it, for a reviewer to judge | `RequirementPairChecker` | — |
| `heading-check.sh` | a leaf's `##` heading list shrinking | `HeadingChecker` | a baseline directory |
| `link-spread.sh` | links appended to a leaf whose prose did not change | `LinkSpreadChecker` | a baseline directory |

`RenameChecker` needs no baseline at all: the journal records `name`, `node_type` and `module` on every node event, so "an id that survives keeps its name" is answerable from spex's own durable state. Only the last two rows depend on a supplied baseline.

`check-lib.sh` becomes nothing — it is a bash library whose helpers (the leaf scanner, the prose view, the fail-closed traps) exist to make shell scripts safe and have no counterpart in a binary.

### One check that has no script today

`schema/drift.schema.json` is the declared shape of an implementer's drift report, and `CLAUDE.md` instructs implementers to file reports against it. The file is not in the `//go:embed` list beside the other schema documents, no Go code reads it, and the `schema` module declares neither a requirement nor a component for it. A schema nothing loads is a document that asks, not a gate that binds — the same failure this proposal exists to close, in a file that sits beside the spec rather than inside it.

So the twelfth checker is `DriftReportChecker`: every `drifts/*.json` must conform to the drift schema, and a malformed report is an error like any other. The schema itself gains an owner, `DriftSchema` in the `schema` module, embedded beside the project, module, journal-line and task-state schemas; that is what turns the document into a gate.

### What varies per project is configuration, not who runs it

There is deliberately no plug-in lens mechanism in this proposal. An external check a project forgets to declare fails in exactly the way a script nobody runs fails, and re-introducing that failure mode would defeat the point.

What genuinely varies is parameters, and those belong in the profile as data: which checks are enabled, which are report-only, which heading declares retired vocabulary, and what flag syntax `UsageStringChecker` recognises. The built-in default profile carries this project's values, so a project without a `spec/profile.json` gets exactly the checks this repository runs. A project that needs a rule spex cannot compute is a real future case, but it is speculative today and adding an extension point for it now would cost the invariant this proposal exists to establish.

### Report-only findings reuse the notes channel

`spex check` emits the report shape [[608f8ca2e1b0|structured error output]] already defines — `valid`, `error_count`, `warning_count`, `errors`, and the `notes` array `2026-08-20-derivation-status` added — through the same `ErrorReporter` (`0f98ca780873`), so a gate that already parses one verdict parses both.

`CountChecker` and `RequirementPairChecker` are report-only by design: their scripts always exit 0 and print a worksheet for a reviewer to judge. They emit **notes**, not errors — findings that inform without gating. That is the same distinction `SpecGate` (`4153dbd38133`) already draws when it prints the completeness pass's notes without letting them decide the verdict.

### Judgement records key on node ids

Three files record hits a reviewer accepted as correct: `scripts/lens-dissolved-modules.allow`, `scripts/lens-lexicon.allow` and `scripts/lens-test-shape.allow`. All three share the format `<path><TAB><substring>` and the same header promise that "both must match, so a new stale reference in an already-listed file still fires" — a good property, built on a bad key. Half that key is a file path, so every recorded judgement silently expires the moment a leaf moves, and it expires by passing rather than by failing.

Identity hashes exist so a node survives a file move. Judgements move to one `spec/judgements.json` keyed on node id plus the matched term — for `TestShapeChecker`, the scenario heading — carrying the reviewer's reason, sitting beside `project.json` because a person writes it. A judgement then survives a rename of the file and correctly dies when the node itself dies.

### Explicitly out of scope

No corpus index, no semantic analysis, nothing needing a model. The known limitation stays: `LexiconChecker` fires on a deliberate negative mention — "there is no `--map` flag" — and a reviewer judges it. No index resolves negation; only a model does, and spex is not one. The sweep's job is to refuse to let a stale term hide, not to decide what the sentence means.

## Impact expectation

### New nodes — validator module

| Node | Type | Task |
|---|---|---|
| `spex check` | api | no |
| Run every mechanical check | module requirement | no |
| Check graph-derived corpus rules | module requirement | no |
| Check against a supplied baseline | module requirement | no |
| Check declared house conventions | module requirement | no |
| Validate drift reports | module requirement | no |
| Record judgements by node id | module requirement | no |
| `CheckCommand` | component | yes |
| `LinkChecker` | component | yes |
| `RenameChecker` | component | yes |
| `DissolvedModuleChecker` | component | yes |
| `RequirementLinkChecker` | component | yes |
| `LexiconChecker` | component | yes |
| `UsageStringChecker` | component | yes |
| `CountChecker` | component | yes |
| `TestShapeChecker` | component | yes |
| `RequirementPairChecker` | component | yes |
| `HeadingChecker` | component | yes |
| `LinkSpreadChecker` | component | yes |
| `DriftReportChecker` | component | yes |
| Check command tests — describes `CheckCommand` | test section | no (one component) |
| Graph check tests — describes `LinkChecker`, `RenameChecker`, `DissolvedModuleChecker`, `RequirementLinkChecker` | test section | yes |
| Baseline check tests — describes `HeadingChecker`, `LinkSpreadChecker` | test section | yes |
| Corpus check tests — describes `LexiconChecker`, `UsageStringChecker`, `CountChecker`, `TestShapeChecker`, `RequirementPairChecker` | test section | yes |
| Drift report check tests — describes `DriftReportChecker` | test section | no (one component) |

Thirteen new components and five new test sections. All six new module requirements derive from [[d5a8407d38e1|validate spec structure]]. Every new component implements one of them and is described by one of the five new test sections, so coverage holds in both directions without further nodes.

### New nodes — schema module

| Node | Type | Task |
|---|---|---|
| Define drift schema | module requirement | no |
| `DriftSchema` | component | yes |

`Define drift schema` derives from [[d5a8407d38e1|validate spec structure]], alongside the other schema definitions. `DriftSchema` is covered by joining `describes` on **Schema validation tests** (`96f944302b78`), which already names `ProjectSchema` and `ModuleSchema` and stays a task.

### Modified component leaves — one task each

| Module | Component | Hash | Why |
|---|---|---|---|
| validator | ErrorReporter | `0f98ca780873` | now composes the report for two commands, not one |
| validator | ValidateCommand | `59235a75aa44` | obliged by the change to `608f8ca2e1b0`, which it also implements |
| schema | SchemaLoader | `ee88263d6555` | obliged by the change to `b7c3bccd7c64`: the embedded set gains the drift schema |
| delivery | SpecGate | `4153dbd38133` | the gate runs `spex check` and asserts on its verdict; the two lens steps fold into it |
| delivery | CIPipeline | `1c2de3dbfe1c` | the workflow materialises a baseline, adds the step and drops the two lens steps |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| validator | Structured error output | `608f8ca2e1b0` | ErrorReporter, ValidateCommand |
| schema | Embed schemas in binary | `b7c3bccd7c64` | SchemaLoader |
| delivery | Spec gate on every PR | `07bbd73df14f` | SpecGate |
| delivery | Tiered CI by trigger | `68f38bb4cc74` | CIPipeline |

Every obligation is discharged by the component table above.

### Meta-leaf accounting

`validator`, `schema` and `delivery` all have `module.json` changes. Each also changes or adds a requirement in the same module, so the meta rule is suppressed in all three and no component beyond those listed owes a leaf.

### Modified test sections

| Module | Test section | Hash | Task |
|---|---|---|---|
| validator | Validation pipeline tests | `dad7c5e68169` | yes |
| schema | Schema validation tests | `96f944302b78` | yes |
| schema | Schema loading tests | `8719672c7580` | yes |
| delivery | CI and Spec Gate Tests | `191f87d4981b` | yes |

### The checks that can actually fail

Parity is the acceptance test, and it must be demonstrated rather than asserted: for each of the eleven rows above, the fixture the script currently rejects is rejected by the new component, and the fixture it currently accepts is accepted. That is only a real check where the current behaviour is pinned — and **ten of the eleven scripts have no dedicated `_test.sh`** (`lens-dissolved-modules` is the exception). Pinning them is owed before this lands and is deliberately not folded into this proposal: it has standalone value now, and doing it here would let the same session define both sides of the parity claim.

Beyond parity: a judgement test asserting a recorded exception still applies after its leaf is moved to a new path, which is precisely what the current format cannot do; a gate test asserting a report-only finding from `CountChecker` leaves the exit code at 0 while a `LexiconChecker` hit does not; and a drift test asserting that a report missing a required field fails `spex check` today, where nothing fails it.

### Scope

Twenty-six tasks under one epic: nineteen component leaves — fourteen new, five modified — and seven test sections, being the three new ones that describe two or more components plus the four modified ones. Nothing is renamed and no spec node is removed.

The retirement of the scripts themselves is implementation work rather than graph work: the eleven checks, `check-lib.sh` and the three `.allow` files are deleted once parity passes, the two workflow steps that ran lenses directly are removed, and the skills that name the scripts are rewritten to invoke `spex check`. No spec leaf currently names any of them, so the retired-vocabulary sweep below starts clean.

## Retired vocabulary

- `scripts/heading-check.sh`
- `scripts/link-check.sh`
- `scripts/link-spread.sh`
- `scripts/no-rename-check.sh`
- `scripts/lens-counts.sh`
- `scripts/lens-dissolved-modules.sh`
- `scripts/lens-lexicon.sh`
- `scripts/lens-requirement-links.sh`
- `scripts/lens-requirements.sh`
- `scripts/lens-test-shape.sh`
- `scripts/lens-usage-strings.sh`
- `scripts/check-lib.sh`
- `scripts/lens-dissolved-modules.allow`
- `scripts/lens-lexicon.allow`
- `scripts/lens-test-shape.allow`
