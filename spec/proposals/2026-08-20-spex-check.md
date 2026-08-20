# Change Proposal: spex check

## Context

`scripts/` holds eleven non-test shell scripts. Three of them are not checks: `apply-br.sh` is the reference adapter, `check-lib.sh` is a sourced library, and `spec-gate.sh` is the CI seat that runs `spex validate` and `spex diff` and asserts on their JSON verdicts. The other **eight are mechanical spec checks**, and **not one of them runs in CI**. `.github/workflows/pr.yml` and `main.yml` invoke `spec-gate.sh` and nothing else.

They run because a skill's prose asks an agent to run them. That works in the repository that grew the tool, where one person absorbs the discipline personally. It does not survive contact with anyone else: a check that would have caught their mistake silently does not run, they ship the mistake, and the tool gets the blame for not catching it. This project's own position is that a gate binds and prose is advisory — [[07bbd73df14f|the spec gate on every PR]] exists precisely because a documented intention is not an enforced one. Eight checks living outside that gate is the doctrine's own violation, and it is the last one of any size.

The scripts are also written in bash, which has shaped how they are thought about. Being written in bash is not evidence that a check needs a shell: `grep`, `jq` and `awk` are how a shell script reads files and matches strings, and a binary does both natively. Every one of the eight is computable in Go over the same inputs.

Two of them compare a leaf against its previous version. That is the only place an outside tool appears, and the scripts already resolved it: `check-lib.sh`'s `base_resolve` accepts **a directory or a git ref**, and `heading-check.sh` states that "out-of-repo trees are first class … needs no git repository anywhere. Only the default base is a git ref and only that needs one." The baseline was always a directory; git is merely the convenient way to produce one. So nothing here reaches for git, no dependency is added to [[96c6c15ecc3e|the declared stack]], and [[58ea35f52b86|no runtime subprocesses]] is untouched — the baseline arrives as a caller-supplied path, exactly as `diff.json` and `--beads` already do.

This proposal has a **hard dependency on `2026-08-20-declarative-profile`**. Half these checks walk the graph by type and edge — which types own content leaves, which edges a node declares — and folding them into the binary against a hardcoded taxonomy would bake the ontology in one more place at exactly the moment it is being lifted out. They read the resolved profile or they do not move.

## Proposed change

### One command, eight checks, all in the binary

A new api `spex check`, provided by a new component `CheckCommand` in the `validator` module. It runs every mechanical check and emits one report.

`validator` owns it because these are spec-directory validation, which is what the module is for. Its description broadens from schema-and-graph conformance to include the corpus and baseline-relative checks, which is an honest widening rather than a stretch: `RequirementCoverageChecker` and `TestCoverageChecker` already live there and are graph-walking rather than schema-checking.

`spex check` stays separate from `spex validate` rather than absorbing it. They have different input contracts: validate needs only the spec directory and is the structural pass every other command depends on; check takes a diff, a journal and optionally a baseline, and is a review-time pass. Merging them would give `spex validate` inputs it does not need in order to answer a question nobody asked it.

### The parity matrix

Each script becomes one component, following the module's established one-checker-one-component pattern:

| Script | Catches | Becomes | Inputs beyond the spec |
|---|---|---|---|
| `link-check.sh` | a touched leaf that does not link its node's declared edges | `LinkChecker` | the diff |
| `no-rename-check.sh` | a surviving id whose `name` or `title` changed | `RenameChecker` | the journal |
| `lens-dissolved-modules.sh` | references to modules that no longer exist | `DissolvedModuleChecker` | the journal, judgement records |
| `lens-lexicon.sh` | retired vocabulary still live in the corpus | `LexiconChecker` | proposals |
| `lens-usage-strings.sh` | a `--flag` in a JSON description absent from the owning arch leaf | `UsageStringChecker` | — |
| `lens-counts.sh` | written-out counts beside the graph's actual numbers | `CountChecker` | — |
| `heading-check.sh` | a leaf's `##` heading list shrinking | `HeadingChecker` | a baseline directory |
| `link-spread.sh` | links appended to a leaf whose prose did not change | `LinkSpreadChecker` | a baseline directory |

`RenameChecker` needs no baseline at all: the journal records `name`, `node_type` and `module` on every node event, so "an id that survives keeps its name" is answerable from spex's own durable state. Only the last two rows depend on a supplied baseline.

`check-lib.sh` becomes nothing — it is a bash library whose helpers (the leaf scanner, the prose view, the fail-closed traps) exist to make shell scripts safe and have no counterpart in a binary.

### What varies per project is configuration, not who runs it

There is deliberately no plug-in lens mechanism in this proposal. An external check a project forgets to declare fails in exactly the way a script nobody runs fails, and re-introducing that failure mode would defeat the point.

What genuinely varies is parameters, and those belong in the profile as data: which checks are enabled, which are report-only, which heading declares retired vocabulary, and what flag syntax `UsageStringChecker` recognises. A project that needs a rule spex cannot compute is a real future case, but it is speculative today and adding an extension point for it now would cost the invariant this proposal exists to establish.

### Report-only findings reuse the notes channel

`spex check` emits the report shape [[608f8ca2e1b0|structured error output]] already defines — `valid`, `error_count`, `warning_count`, `errors` — through the same `ErrorReporter` (`0f98ca780873`), so a gate that already parses one verdict parses both.

`CountChecker` is report-only by design: its script always exits 0 and prints a worksheet for a reviewer to judge. It emits **notes**, not errors, using the disclosure channel `2026-08-20-derivation-status` adds to the validate report — findings that inform without gating. That is the same distinction `SpecGate` (`4153dbd38133`) already draws when it prints the completeness pass's notes without letting them decide the verdict.

### Judgement records key on node ids

`scripts/lens-dissolved-modules.allow` records hits a reviewer accepted as correct. Its own header says the format is `<path><TAB><substring>` and that "both must match, so a new stale reference in an already-listed file still fires" — a good property, built on a bad key. Half that key is a file path, so every recorded judgement silently expires the moment a leaf moves, and it expires by passing rather than by failing.

Identity hashes exist so a node survives a file move. Judgements move to a `spec/judgements.json` keyed on node id plus the matched term, carrying the reviewer's reason, sitting beside `project.json` because a person writes it. A judgement then survives a rename of the file and correctly dies when the node itself dies.

### Explicitly out of scope

No corpus index, no semantic analysis, nothing needing a model. The known limitation stays: `LexiconChecker` fires on a deliberate negative mention — "there is no `--map` flag" — and a reviewer judges it. No index resolves negation; only a model does, and spex is not one. The sweep's job is to refuse to let a stale term hide, not to decide what the sentence means.

## Impact expectation

### New nodes — validator module

| Node | Type | Bead |
|---|---|---|
| `spex check` | api | no |
| Run every mechanical check | module requirement | no |
| Check graph-derived corpus rules | module requirement | no |
| Check against a supplied baseline | module requirement | no |
| Check declared house conventions | module requirement | no |
| Record judgements by node id | module requirement | no |
| `CheckCommand` | component | yes |
| `LinkChecker` | component | yes |
| `RenameChecker` | component | yes |
| `DissolvedModuleChecker` | component | yes |
| `LexiconChecker` | component | yes |
| `UsageStringChecker` | component | yes |
| `CountChecker` | component | yes |
| `HeadingChecker` | component | yes |
| `LinkSpreadChecker` | component | yes |
| Check command tests — describes `CheckCommand` | test section | no (one component) |
| Graph check tests — describes `LinkChecker`, `RenameChecker`, `DissolvedModuleChecker` | test section | yes |
| Baseline check tests — describes `HeadingChecker`, `LinkSpreadChecker` | test section | yes |
| Corpus check tests — describes `LexiconChecker`, `UsageStringChecker`, `CountChecker` | test section | yes |

Nine new components and four new test sections. All five new module requirements derive from [[d5a8407d38e1|validate spec structure]]. Every new component implements one of them and is described by one of the four new test sections, so coverage holds in both directions without further nodes.

### Modified component leaves — one bead each

| Module | Component | Hash | Why |
|---|---|---|---|
| validator | ErrorReporter | `0f98ca780873` | now composes the report for two commands, not one |
| validator | ValidateCommand | `59235a75aa44` | obliged by the change to `608f8ca2e1b0`, which it also implements |
| delivery | SpecGate | `4153dbd38133` | the gate runs `spex check` and asserts on its verdict |
| delivery | CIPipeline | `1c2de3dbfe1c` | the workflow materialises a baseline and adds the step |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| validator | Structured error output | `608f8ca2e1b0` | ErrorReporter, ValidateCommand |
| delivery | Spec gate on every PR | `07bbd73df14f` | SpecGate |
| delivery | Tiered CI by trigger | `68f38bb4cc74` | CIPipeline |

Every obligation is discharged by the component table above.

### Meta-leaf accounting

`validator` and `delivery` both have `module.json` changes. Both also change a requirement in the same module, so the meta rule is suppressed in each and no component beyond those listed owes a leaf.

### Modified test sections

| Module | Test section | Hash | Bead |
|---|---|---|---|
| validator | Validation pipeline tests | `dad7c5e68169` | yes |
| delivery | CI and Spec Gate Tests | `191f87d4981b` | yes |

### The checks that can actually fail

Parity is the acceptance test, and it must be demonstrated rather than asserted: for each of the eight rows above, the fixture the script currently rejects is rejected by the new component, and the fixture it currently accepts is accepted. That is only a real check where the current behaviour is pinned — and **seven of the eight scripts have no dedicated `_test.sh`** (`lens-dissolved-modules` is the exception). Pinning them is owed before this lands and is deliberately not folded into this proposal: it has standalone value now, and doing it here would let the same session define both sides of the parity claim.

Beyond parity: a judgement test asserting a recorded exception still applies after its leaf is moved to a new path, which is precisely what the current format cannot do; and a gate test asserting a report-only finding from `CountChecker` leaves the exit code at 0 while a `LexiconChecker` hit does not.

### Scope

Eighteen beads under one epic: thirteen component leaves — nine new, four modified — and five test sections, being the three new ones that describe two or more components plus the two modified ones. Nothing is renamed and no spec node is removed.

The retirement of the scripts themselves is implementation work rather than graph work: the eight checks, `check-lib.sh` and `lens-dissolved-modules.allow` are deleted once parity passes, and the skills that name them are rewritten to invoke `spex check`. No spec leaf currently names any of them, so the retired-vocabulary sweep below starts clean.

## Retired vocabulary

- `scripts/heading-check.sh`
- `scripts/link-check.sh`
- `scripts/link-spread.sh`
- `scripts/no-rename-check.sh`
- `scripts/lens-counts.sh`
- `scripts/lens-dissolved-modules.sh`
- `scripts/lens-lexicon.sh`
- `scripts/lens-usage-strings.sh`
- `scripts/check-lib.sh`
- `scripts/lens-dissolved-modules.allow`
