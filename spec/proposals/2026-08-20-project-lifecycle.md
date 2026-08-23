# Change Proposal: Project identity and lifecycle

## Context

A spex project has no moment of birth. There is no command that creates one, nothing that answers "is this set up correctly", and no state that a project is required to have. Absence of the baseline is not an error — it is a supported mode: `merkle.EmptyTree()` is documented as the baseline `SnapshotStore.Load` returns when the snapshot file is absent, `cmd/spex/diff.go` stats the path and falls back to it, and normal-mode ingest bootstraps from there. The first cycle works with nothing on disk, by design.

That design has three costs, and they compound as the tool leaves the repository that grew it.

**Absence means two different things.** "There is no snapshot" currently answers both *"your baseline is empty"* — asked by [[f223179a540a|diff against snapshot]] — and *"this project has never completed a cycle"* — asked by the refresh bootstrap guard in the ingest module. One fact, two questions, and nothing keeps them aligned.

**The fallback is ambiguous where it matters most.** `spex diff` on a project that was never initialised and on a properly initialised project whose first cycle has not yet run print the same output: everything added. Piping either into `spex plan` produces a full task set. In one case that is the intended bootstrap; in the other it is an accident, and the two are indistinguishable at the point of decision.

**Every reader re-implements the rule.** `DiffCommand` (`c8b958ec310d`) handles the missing file correctly today. Whether the next reader does is a matter of remembering, not of structure — and derived state that any consumer may misread is exactly the kind of invariant this project holds in a component rather than in a habit.

The trigger is adoption beyond this repository. This is the second of an agreed series of eight proposals; the first, `2026-08-20-derivation-status`, made a partially-decomposed spec expressible. This one gives a project an identity: a place its derived state lives, a command that creates it, a command that diagnoses it, and a single answer to what absence means. Nothing here refers forward to later proposals in the series.

This is a change of invariant, not a clarification of one. Today a project with no baseline is *valid*; after this it is *uninitialised or broken*. That is worth arguing on its merits rather than smuggling in as a tidy-up.

## Proposed change

### A new module: lifecycle

The commands and the resolver need an owner, and no existing module can take them without contradicting itself. The `cli` module's [[293b27f73924|bounded third-party CLI surface]] states that cobra reaches "cli, which holds the root command and nothing else"; `merkle` owns the baseline but not the journal, and `map` owns the journal but not the baseline. A resolver that must speak for both, and commands that create both, straddle that line.

So: a new module `lifecycle`, owning the on-disk contract of what a spex project *is*. It declares `requires_module` on `merkle` and `map` — both acyclic, since neither depends on lifecycle — and holds three components, five requirements, two apis and two test sections.

It derives from a new project requirement, **Project initialization and health** (`functional`, priority 1): a spex project has a declared state directory created by an explicit command, its absence is an error rather than a fallback, and its condition is diagnosable without repair.

### The `.spex/` directory

Derived state moves into `.spex/` at the project root: the snapshot and the task journal, and nothing else. Authored content — spec leaves, `project.json`, `module.json`, proposals — stays exactly where a human edits it. The split is the rule, not the convenience: `.spex/` is what the tool writes, `spec/` is what a person writes.

`.spex/` is committed, not ignored. The journal is the one artifact in the system that cannot be reconstructed from anything else, and a dotfile at the repository root is precisely the kind of thing a `.gitignore` habit swallows.

**`absorb.json` moves too, and this proposal answers that question rather than leaving it open.** It is a run input — a human writes the reasons, a single `spex plan` invocation consumes them — and it currently sits at the repository root beside `go.mod` and `README.md`, where it reads as a permanent artifact. It is neither authored spec nor derived state, so it belongs in neither `spec/` nor beside the snapshot. It moves to `.spex/runs/absorb.json`, a directory whose contents are run scratch: written for one cycle, consumed by it, and safe to remove afterwards. The recommendation over the alternatives — leaving it at the root, or filing it under `spec/` — is that both of those make a transient artifact look durable, and the journal already holds the permanent record of what an absorb decided.

### `spex init`

A new api `spex init`, provided by a new component `InitCommand`, creating `.spex/` with two files:

- **An empty snapshot** — the canonical empty tree, never a snapshot of the spec that already exists. Seeding the baseline from the current spec would make the first diff clean, and no work would ever be born from the initial spec.
- **An empty journal.** Deliberately no init event. The empty journal is what "no cycle has completed" means, and an event written at init would make that predicate permanently false — which is precisely the predicate the refresh guard below needs. Provenance recorded at birth is a real thing to want, but not at the cost of the fact the rest of the system reads.

`init` refuses a directory that already has `.spex/` rather than overwriting it. It is the one command that can destroy a journal.

### Absence becomes an error

[[e99a846810df|Store snapshots]] is amended: `SnapshotStore.Load` errors on an absent snapshot instead of returning a baseline. `merkle.EmptyTree()` is not deleted — it moves, from the forgiving default inside `Load` to the seed value `InitCommand` writes. That is the seam where the old behaviour would otherwise survive by accident, so the requirement must say where the empty tree is produced and where it no longer is.

No command implicitly initialises. A read command must not write, and the ambiguity described in Context is resolved by refusing rather than by guessing.

### One pre-flight, two absences, one exit code

A new component `ProjectResolver` is the single pre-flight every subcommand calls, returning either a project context or a typed error. Not fifteen per-command messages that drift apart as commands are added.

It distinguishes two absences, because conflating them is dangerous:

- **No `.spex/` at all** — the project was never initialised. The error names `spex init`.
- **`.spex/` present, snapshot or journal missing or unparseable** — the project is broken. The error names `spex doctor`.

If both said "run init", a user with a bad merge would re-initialise and destroy the journal. The failure mode is asymmetric, so the messages must be too.

"Not a spex project" gets its own stable, documented exit code, distinct from the existing input-error and invariant codes, so CI and scripts branch on the code rather than grepping the message.

### `spex doctor`

A new api `spex doctor`, provided by a new component `DoctorCommand`. It reports what is present, what is missing and what is unreadable, and it names the command that would fix each finding.

It never mints or moves a baseline — not by default, and not behind a `--fix` flag. The moment doctor can repair a snapshot, "the baseline moves only deliberately" acquires an automated exception, and that exception would be exercised in exactly the situation where nobody is thinking clearly. Doctor diagnoses; a human decides.

Doctor is also the answer to a question nothing currently answers: *is my project set up correctly?* For someone arriving at spex for the first time, that is the first thing they want to know.

### The refresh guard re-keys onto the journal

`ingest`'s [[e68653819f38|refresh mode for impl_only drift]] refuses a refresh run with no pre-existing snapshot, using "does a snapshot file exist?" as a proxy for "has a cycle ever completed?". Once `init` always writes a snapshot, that proxy is permanently true and the guard stops guarding. It re-keys onto the journal being empty, which is the fact it wanted. Ingest already works in terms of the journal fold, so this introduces no new dependency for it.

The stakes are modest and the proposal should say so rather than overstate the fix. Refresh only absorbs added `requirement` and `api` leaves; added `component`, `data_flow`, `test_section`, `module` and `meta` are refused in both directions. A real new project has modules and components, so the deeper gate refuses the run regardless. Losing the bootstrap guard would degrade one early, clear refusal into a late one listing every added entry — the narrow leak being a spec whose only additions are requirements and apis. The re-key is hygiene on a guard that should mean what it says, not the closing of a hole.

### Path resolution and a compatibility window

`spec/.snapshot.json` and `spec/.history.jsonl` are named in thirteen arch leaves across seven modules, in five module requirement descriptions, in the scripts and in the CI gate. A hard cutover breaks every one of them on the same commit, and the sibling tools that follow the same conventions with them.

`ProjectResolver` owns location resolution: it prefers `.spex/`, falls back to the legacy paths when `.spex/` is absent, and reports which it used. The window is explicit and finite — a project on the legacy layout keeps working and is told, once, how to migrate. The leaves stop naming literal paths and name the resolved locations instead, which is what makes the window possible without thirteen leaves lying.

The old paths are **not** declared retired vocabulary here, and that is deliberate: while the window honours them, a corpus sweep for those strings would fire on every legitimate compatibility mention. Their retirement is owed by the change that closes the window, and belongs in that change's proposal.

### The first run, documented rather than engineered around

This proposal changes what the first run does, so it also owns how the first run is explained.

**The first cycle is add-only, structurally.** Removals are detected as "present in baseline, absent in current", and an empty baseline has no keys for anything to be absent from. That is a property of the baseline being empty, not of a file being missing — materialising the snapshot does not change it — and on a project being born there is genuinely nothing to remove. It is disclosed, not fixed.

**The read-only path gets said out loud.** `spex validate`, `spex diff` and `spex render` answer real questions with no tracker, no adapter and no journal activity. The documentation currently opens on the full pipeline, so a reader concludes a tracker binding is the price of entry. After `init`, the shortest useful sequence is three commands and no integration.

### One constraint recorded, not designed

A sibling ingest mode for adopting a spec written over code that already exists is deliberately out of scope. It is recorded here only as a constraint: the journal-empty predicate introduced by the refresh re-key must remain available as a gate, so that a future mode of that kind and refresh can be made mutually exclusive by construction rather than by documentation.

## Impact expectation

This is the largest proposal in the series, and almost all of the size is the path move rather than the new behaviour: thirteen existing leaves name a location that stops being the only truth.

### New nodes — lifecycle module

| Node | Type | Bead |
|---|---|---|
| `lifecycle` | module | no — a `meta` leaf, which produces none |
| Project initialization and health | project requirement, priority 1 | no |
| Project state directory | module requirement | no |
| Initialize a project | module requirement | no |
| Diagnose project health | module requirement | no |
| Uninitialized project is an error | module requirement | no |
| Legacy state path compatibility | module requirement | no |
| `ProjectResolver` | component | yes |
| `InitCommand` | component | yes |
| `DoctorCommand` | component | yes |
| `spex init` | api | no |
| `spex doctor` | api | no |
| Resolver tests | test section — describes `ProjectResolver` | no (one component) |
| Lifecycle command tests | test section — describes `InitCommand`, `DoctorCommand` | yes |

Coverage obligations are satisfied inside the module: the new project requirement is derived by all five module requirements; `ProjectResolver` implements the state-directory, error and compatibility requirements; `InitCommand` and `DoctorCommand` implement theirs; and both test sections together describe all three components.

### Modified component leaves — one bead each

| Module | Component | Hash | Why |
|---|---|---|---|
| merkle | SnapshotStore | `b2fcd9457a28` | Load errors on absence; EmptyTree is no longer its default |
| merkle | TreeBuilder | `dfe1467b7a4b` | names the snapshot location |
| merkle | DiffCommand | `c8b958ec310d` | drops the stat-and-fall-back path; resolves instead |
| merkle | DiffEngine | `cb262b280963` | names the snapshot location |
| merkle | ImpactClassifier | `f1a672216ce9` | names the snapshot location |
| ingest | SnapshotSaver | `f85bd2f94aeb` | writes through the resolved location |
| ingest | RefreshHandler | `f9033352c13f` | bootstrap guard re-keys onto the journal |
| ingest | IngestCommand | `db90eb607bcb` | names both locations |
| map | MappingStore | `205e67ca4aad` | owns the journal; names its location |
| map | MapCommand | `08909d62930b` | names the journal location |
| plan | PlanCommand | `92ae9dab6d6d` | names the journal location and the absorb file |
| schema | BeadMapSchema | `d125b5e775b4` | the journal-line schema names the journal |
| cli | RootCommand | `b6758cdfabc4` | the bounded-surface enumeration gains two constructors |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| merkle | Store snapshots | `e99a846810df` | SnapshotStore |
| ingest | Snapshot save gated on complete status | `cf47671793fa` | SnapshotSaver |
| ingest | Atomic file replacement | `ffab5d1337ac` | SnapshotSaver |
| ingest | No runtime subprocess | `20589ccf7072` | IngestCommand |
| ingest | Refresh mode for impl_only drift | `e68653819f38` | RefreshHandler |
| map | Store mapping records | `934d627f0e90` | MappingStore |
| schema | Define bead-map schema | `f7ef8bef0ba1` | BeadMapSchema |
| cli | Bounded third-party CLI surface | `293b27f73924` | RootCommand |
| plan | Pure function over inputs | `cf4f1ab8264a` | PlanCommand |

Every obligation is discharged by the component table above.

### Meta-leaf accounting

`merkle`, `ingest`, `map`, `schema`, `cli` and `plan` all have `module.json` changes, each moving that module's `meta` leaf. In every one of them a requirement in the same module also changes, which suppresses the meta rule — so no module owes changed leaves from components not already listed.

The `plan` case is the one that needed deliberate handling. The journal location appears in `PlanCommand`'s *component description* inside `plan/module.json`, and a component-description edit alone moves the meta leaf with no requirement change to suppress it — which would oblige changed leaves from all eight `plan` components. Amending `Pure function over inputs` (`cf4f1ab8264a`), which is where plan's inputs are named and therefore where the journal location legitimately belongs, suppresses the rule and obliges only `PlanCommand`, whose leaf is changing anyway.

### Modified test sections

Beads only where `describes` names two or more components:

| Module | Test section | Hash | Bead |
|---|---|---|---|
| merkle | Hashing tests | `31b096a76fd4` | yes |
| merkle | Diff and classification tests | `95a279cbdcbc` | yes |
| merkle | Snapshot tests | `4ff940b743d7` | no |
| merkle | Merkle command tests | `49a61e0d5737` | no |
| ingest | Consistency invariants | `2cd902e10873` | yes |
| ingest | Partial run recovery tests | `2515c91337a5` | yes |
| ingest | Refresh mode tests | `a483524c406c` | no |
| ingest | Ingest command tests | `3e9de336f65f` | no |
| map | Mapping store tests | `21eae6071d42` | no |
| map | Map command tests | `571971ee44d6` | no |
| plan | Plan command tests | `7f529376c087` | no |
| schema | Bead map schema tests | `fe35207b5210` | no |
| cli | Root Command Tests | `476f594a2f5f` | no |

The checks that can actually fail: a resolver test asserting a legacy-layout project still resolves and reports which layout it used; an init test asserting the seeded snapshot is the empty tree and not the current spec; a refresh test asserting the bootstrap refusal fires on an empty journal with a snapshot present, which is the exact state the re-key exists for; and a diff test asserting an uninitialised directory errors with the not-a-project exit code instead of printing everything as added.

### Scope

Twenty-one beads under one epic: sixteen component leaves — thirteen modified, three new — and five test sections. No node is renamed, so nothing is delete-plus-create and no inbound link needs rewriting. Nothing is removed.

Work outside the spec graph that rides with the epic: the `.gitignore` must be checked so `.spex/` is committed; `scripts/` and the CI gate resolve locations rather than naming them; and the repository documentation gains the first-run and read-only-path story described above. None of those are spec nodes, and none of them produce beads of their own.

## Addendum (2026-08-22): the compatibility window is dropped

Decided in the authoring session that turned this proposal into spec, before any commit or mint. The window defended a class of consumer that does not exist: the legacy layout has exactly one project on it — this repository — no binary carrying the new behaviour has been released, and the one legacy project can migrate itself inside the epic. A window nobody stands in buys nothing and costs a fallback path, a fifth requirement, and a permanent "which layout?" ambiguity in every leaf that names a location.

What changes against the body above:

- **"Legacy state path compatibility" is not authored.** `ProjectResolver` resolves `.spex/` and nothing else, and reports no layout. The two-absence contract — never initialised names `spex init`, broken names `spex doctor` — is unchanged; it never depended on the window. Lifecycle carries four module requirements, not five.
- **The old paths become retired vocabulary in this change after all.** The body deferred their retirement to "the change that closes the window"; with no window, this is that change, and the sweep is owed here. That pulls into scope the leaves the body's tables deliberately excluded: `flow_hash_computation.md`'s bootstrap narrative changes for real — the Load-falls-back-to-empty-tree story it told is now false — which is one data_flow bead the body's count did not carry. The remaining mentions (three other flow leaves, a handful of test leaves, the proposal module's leaves) are path-spelling sweeps that owe no code and ride an absorb list, reviewed reason by reason in the PR.
- **The open init-on-legacy question dissolves.** With no honoured legacy layout there is no populated legacy journal for an empty `.spex/` to shadow; the TODO recorded for it is withdrawn.
- **Migration is a constraint on landing order, not a feature.** This repository's own state files (`spec/.snapshot.json`, `spec/.history.jsonl`, and the root `absorb.json` → `.spex/runs/`) move in the same PR that lands the no-fallback resolver: any main commit where the new binary exists but the files have not moved is self-broken. The mint for this epic still runs with the current binary against the current locations, which is fine — only the in-epic landing order matters.

- **The Composable requirement is corrected in the same pass.** The spec-review over this amendment surfaced a standing contradiction the new exit code widens: project requirement `91f6f338de19` claimed "exits 0/1" while `spex diff` and `spex plan` have long exited 2 and the resolver adds the not-a-spex-project code. Its description now reads "a small set of documented codes", which obliges the implementors of its two deriving module requirements — plan's `ChangesetBuilder` (already in scope) and render's `SpecReader` and `RenderCommand` (two beads this addendum adds). The render edits also state the read-only boundary the lifecycle makes load-bearing: render reads authored spec only, never project state, and runs before `spex init` ever has.

## Retired vocabulary

Retired by the addendum, not the body: while the window stood, these spellings were legitimate compatibility mentions; with the window gone they are dead locations, and the lexicon sweep enforces their absence from the corpus.

- `spec/.snapshot.json`
- `spec/.history.jsonl`
