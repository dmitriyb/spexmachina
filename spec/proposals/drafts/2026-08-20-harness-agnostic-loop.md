# Change Proposal: Harness-agnostic loop

## Context

The authoring loop is four skills — `/propose`, `/spec`, `/spec-review`, `/drift-fix` — and each exists as one `SKILL.md` under `skills/`, in Claude Code's skill format. `docs/skills.md` opens by saying so: "four Claude Code skills under `skills/` drive it."

That the skills are swappable is a genuine design position, and the right one. The schema is meant to be the interface: bring whatever authoring process you like, as long as what it produces is a valid spex project. But that only holds if the loop's contract is written somewhere a different process can read, and today it is not. It is distributed across four prose files addressed to one tool. Someone driving spex from another harness, or by hand, has to reverse-engineer it.

The gap is sharper than "prose is prose", because of how spex is distributed. A user who installs the released binary — the documented path, `curl`, verify, `bash install.sh` — gets `spex` and nothing else. `skills/` is a directory in this repository, not part of the artifact anyone downloads. So the loop is not merely Claude-Code-shaped; for every user who did not clone the repo, it is absent.

The parts of the loop that *are* enforced are enforced well, and they are the reason this is worth doing rather than documenting. Each phase produces an artifact, and for most of them something already refuses a bad one:

| Phase | Artifact | What enforces it |
|---|---|---|
| propose | a proposal in `spec/proposals/` | `Registrar` (`24180f55c0b4`) — [[2b62ad5e8ef2|register proposal]] detects the type from H2 headings and rejects a proposal missing a required section |
| spec | `project.json`, `module.json`, content leaves | `ValidateCommand` (`59235a75aa44`) and the checkers behind it, plus `DiffCommand`'s (`c8b958ec310d`) completeness pass |
| spec-review | a correction proposal, when there are findings | the same `Registrar`, on the proposal it emits — the audit itself produces nothing checkable |
| drift-fix | spec corrections plus a deliberate mint-or-refresh | `RefreshHandler` (`f9033352c13f`) gates the refresh; **nothing enforces the drift report's shape** |

That last cell is a real gap and worth stating rather than smoothing over. `schema/drift.schema.json` exists, and `CLAUDE.md` instructs implementers to file reports against it — but the file is not in the `//go:embed` list beside `project.schema.json`, `module.schema.json` and `bead-map.schema.json`, no Go code reads it, and the `schema` module declares neither a requirement nor a component for it. A schema nothing loads is a document that asks, not a gate that binds, which is precisely the failure this project names elsewhere.

This proposal is **independent of every other in the series** and can land in any order.

## Proposed change

### `spex guide <phase>`

A new api emitting, for one phase of the loop, two things:

**The instructions**, in prose — what this phase is for, what it must not do, the constraints that make its output survive the next phase.

**The contract**, machine-readable — the inputs the phase consumes, the artifact it owes, and what validates that artifact, named as a component rather than described. An agent that reads the contract can check its own output before handing back, instead of learning at review time that it produced something unregisterable.

The contract is the half that makes this more than documentation. For `propose` it says the artifact is a markdown proposal whose H2 headings must satisfy `Registrar`; for `spec` it says the artifact is a spec directory that `ValidateCommand` must pass and `DiffCommand`'s completeness pass must not reject. Those are not descriptions of enforcement — they name the enforcing component, so a reader can run it.

For `drift-fix` the contract states the honest position: the report's shape is `schema/drift.schema.json` and **no component enforces it**. Recording the gap as data rather than folklore is the useful outcome here; closing it is a separate change with its own argument.

### It generalises `spex template`, and reuses it

`TemplateProvider` (`cc8adc823719`) already emits an authoring artifact from the binary — [[e8c48d1b4cde|provide templates]] outputs a project or change proposal template to stdout. That is the propose phase's artifact shape, already owned by a component.

So `spex guide` is a generalisation of `spex template`, and it **reuses** rather than replaces it: `GuideProvider` takes a `uses` edge on `TemplateProvider` and gets the propose phase's artifact shape from the single place that already owns it. `spex template` stays exactly as it is — retiring an api to fold it into a larger one would be a rename with no benefit, and the two surfaces answer different questions: one is "give me the template", the other is "tell me everything about this phase".

### The prose lives in the binary

This is the decision worth arguing rather than assuming, and it goes against the cheaper option.

Embedding the phase prose means every prose change ships as a release. That is a real cost: a wording fix waits for a version. The alternative — emit only the contract and leave the instructions in `skills/` — keeps the prose editable in the repository and costs nothing to change.

Embedding wins anyway, for one reason that outweighs the rest: `skills/` is not part of what anyone installs. A user who verified and ran the installer has the binary and no repository. Leaving the instructions outside it means the portable half of the loop is the half that says what to check, and the half that says what to *do* is missing for exactly the users this change exists for. A contract with no instructions is not a loop anyone can run.

The consequence is worth naming: the loop's prose becomes versioned with the tool, and `spex version` then identifies which loop you are running. For a contract, that is a feature.

`skills/` is not deleted. The four `SKILL.md` files become **one rendering** of the loop — the Claude Code packaging of it — wrapping `spex guide` rather than restating it. That is the framing of skills as swappable, made true instead of aspirational.

### Which module owns it

The `proposal` module. Its description today is "Proposal lifecycle management: registration, structure validation, history tracking, templates", which sounds narrower than the loop — but its own [[1d33b4c6c36e|proposal lifecycle]] data flow already spans the whole span this covers: proposal file → register → spec change → plan → bead actions → history query. The module already claims the lifecycle end to end; the guide describes the phases of the same lifecycle. Its description broadens to say so.

A new module was considered and rejected: it would own one component and duplicate a span the proposal module already declares.

## Impact expectation

### New nodes — proposal module

| Node | Type | Bead |
|---|---|---|
| Emit phase instructions | module requirement | no |
| Emit phase contracts | module requirement | no |
| `GuideProvider` | component | yes |
| `spex guide` | api | no |
| Guide tests — describes `GuideProvider` | test section | no (one component) |

Both new requirements derive from [[b42bc8a9b82a|manage proposals]], the project requirement the module's existing lifecycle requirements already derive from. `GuideProvider` implements both and takes a `uses` edge on `TemplateProvider` (`cc8adc823719`), which stays acyclic.

### Modified nodes

| Module | Node | Hash | Bead | Why |
|---|---|---|---|---|
| proposal | `ProposalCommands` | `21bc124a46c0` | yes | wires a fourth subcommand; its description enumerates the three it wires |
| proposal | `HistoryViewer` | `97f73ced5a02` | yes | obliged by the change to `fd540b407fb4` below |
| cli | `RootCommand` | `b6758cdfabc4` | yes | the bounded-surface enumeration gains a constructor |
| proposal | History and template tests | `b313b3531f32` | yes | describes `HistoryViewer`, which changes |
| proposal | Proposal command tests | `a20cc3f7f6f1` | no | one component |
| cli | Root Command Tests | `476f594a2f5f` | no | one component |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| proposal | Bead data via stdin | `fd540b407fb4` | HistoryViewer, ProposalCommands |
| cli | Bounded third-party CLI surface | `293b27f73924` | RootCommand |

`fd540b407fb4` is forced rather than incidental: its description states that "this module exposes **three** surfaces through a single command component", and `spex guide` makes it four. `293b27f73924` is the same trap that has now caught three proposals in this series — it enumerates the subcommand constructors by count in its own description, so adding any api at all obliges it.

The proposal module description also changes, to widen from proposal lifecycle to the loop's phase contracts.

### Meta-leaf accounting

`proposal` and `cli` both have `module.json` changes. `proposal` changes `fd540b407fb4` and `cli` changes `293b27f73924`, so the meta rule is suppressed in both and no component beyond those listed owes a leaf.

### The checks that can actually fail

A contract test per phase, asserting that the artifact the contract *describes* is one the named component actually accepts — and, more usefully, that a deliberately malformed artifact is one it rejects. For `propose` that means a proposal missing a required H2 heading must fail `Registrar`; for `spec`, a spec directory with an uncovered component must fail `ValidateCommand`. A contract that names a validator which does not in fact refuse anything is worse than no contract.

Plus one test asserting `spex guide propose` and `spex template change` agree on the template — the single-source property that keeps the generalisation from becoming a copy.

### Scope

Five beads under one epic: four component leaves — one new, three modified — and one test section. Nothing is renamed and nothing is removed.

Work outside the graph riding with the epic: the four `SKILL.md` files are rewritten to wrap `spex guide` rather than restate it, and `docs/skills.md` stops describing the loop as four Claude Code skills and starts describing it as a loop with one Claude Code rendering.
