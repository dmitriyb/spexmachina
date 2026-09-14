# Change Proposal: Ingest adopt mode

## Context

This proposal was first written as a recorded design and held back until a real second user asked for it. Two have. `faber` carries a spex-shaped spec — one `project.json`, seven modules, seventy-nine content leaves — that fails `spex validate` with 188 errors, every one of them format age rather than authoring: requirements still carry `title` where the current format says `name`, and modules declare `impl_sections`, a leaf kind the current profile does not know. `portitor` carries hand-written seeds by its own account: five module files, no `project.json`, no identity hashes, and a README that says the seeds wait to be "run through the spex workflow". Both describe systems whose code is finished and in use. Neither has a `.spex/` directory.

The gap they meet is the one this proposal has always addressed. Someone who writes a spec describing work that is already finished has no path today that does not propose a task per node. `spex init` seeds the canonical empty tree deliberately — never a snapshot of the spec that already exists — so the first diff shows every node as added and `spex plan` mints a create for each. Correct for a project being born, wrong for a project being documented.

`2026-08-20-derivation-status` covers part of that problem. With a declared-but-underived state on project requirements, an adopter names the whole system shallowly, derives only the module they are actually working on, and lets the rest sit declared and honest. Tasks are created for the module in flight and nothing else. That serves an adopter who wants to start small. It does not serve one who arrives with a full spec of an existing system — components, flows, test sections, whole modules — and wants it baselined as documentation without generating hundreds of tasks for work that shipped long ago. Both adopters above are in that second position.

### Why refresh cannot serve it

`spex ingest --mode refresh` looks like the answer and is not, for two reasons, both deliberate.

It refuses a run while the journal is empty. [[e68653819f38|Refresh mode]] absorbs drift *between* cycles; an empty journal means no cycle has ever completed, and that is the bootstrap case, which belongs to the normal pipeline. Every adopter arrives with an empty journal.

And it refuses added or removed module and envelope nodes unconditionally, as the handler's own fixed rule rather than a profile declaration, because baselining one would hide from every downstream tool a change that `spex diff` or `spex validate` rejects. A retro-written spec is mostly new modules. It hits that wall on its first entry.

Everything else refresh once refused it now absorbs: under the default profile every declared node type is absorbable in both directions. The remaining refusals are not an allow-list that could be loosened. They are the two properties that make refresh safe for ordinary operation, and loosening either would make the bypass permanently available to everyone. Retro-adoption makes a different claim from drift: these nodes *would* owe work under the normal rules, and the adopter is asserting the work is already done in code, outside the tracker. Refresh has no way to express that assertion, so it refuses — correctly.

The assertion needs its own mode, its own gate, and its own record. `2026-08-20-project-lifecycle` left the hook for it in the refresh requirement itself: the journal-empty predicate "stays available as a gate, so a future adoption-style ingest mode and refresh can be made mutually exclusive by construction." This is that mode.

## Proposed change

### A third pathway behind the same surface

`--mode adopt`, behind the existing [[3589714e50f8|spex ingest]] api. Not a new command: [[20589ccf7072|no runtime subprocess]] states that `--mode refresh` "selects a pathway behind that single surface rather than standing up a second one, so both modes are bound by the guarantee above and neither can drift away from it in the graph". Adopt is bound the same way, and that requirement's wording changes from two modes to three.

A new component `AdoptHandler` owns the pathway, alongside `RefreshHandler` (`f9033352c13f`), both dispatched by `IngestCommand` (`db90eb607bcb`). Like `RefreshHandler` it writes through `JournalEncoder` (`6ce1df0a456b`) and takes a `uses` edge on it.

### Gated on an empty journal, which is the whole design

`AdoptHandler` refuses any run where the task journal is not empty.

That single predicate is what makes this safe, and it is structural rather than documentary. Refresh requires a **non-empty** journal, meaning a cycle has completed. Adopt requires an **empty** one. The two are therefore mutually exclusive by construction: once a single cycle has run, adopt is unavailable forever, and it cannot be reached for later as a way around the task lifecycle. Neither handler needs to trust the other, and no document has to ask anyone to be careful.

The predicate has a consequence worth stating, because it shapes how adoption is run. An empty journal holds no `registered` event either, and a `registered` event is what opens a proposal's lifecycle. Adoption therefore cannot be a spex proposal with an epic behind it: it is pre-history by construction. The spec an adopter baselines is authored without a proposal, and the adopt run leaves no epic behind — only the receipt described below. The first proposal a project registers comes after adoption, and describes the first change to what was adopted.

### What adopt absorbs, and why the profile does not decide

Adopt absorbs every node the current spec declares, of every type, in the added direction — including module and envelope nodes, which refresh refuses as its own fixed rule. The two decisions differ because the situations do. Refresh refuses a new module because a downstream tool that has already seen this project would be blind to a whole module appearing without any gate seeing it. At adoption there is no downstream yet: no snapshot but the empty tree, no journal, no task. The adopter's assertion is that the entire declared spec is already realised, and a module is the natural unit of that assertion, not an exception to it.

For the same reason adopt does not consult the profile's per-type, per-direction absorbable declarations that refresh reads. Those declarations answer "which drift may pass without task work between cycles" — a policy about the steady state. Adopt runs once, before the steady state exists, over an assertion that admits no per-type nuance: either the spec describes finished work or the adopter should be running the normal pipeline. A profile that could exempt a type from adoption would produce a project half-baselined and half-owed, which is the ambiguous state `2026-08-20-project-lifecycle` exists to make impossible.

### One event per absorbed node, so the biography survives

`RefreshHandler` absorbs by appending one change event per absorbed entry to the task journal, closed by one refresh receipt, and rewriting the snapshot — atomically, with no task lifecycle. `AdoptHandler` does the same, for the same reason: a node baselined with no journal event has no record it ever existed, so [[3c8a43221ed2|spex map context]] would answer nothing for it, permanently. The journal is where a node's biography lives, and adoption must not create nodes that were never born.

The adopter's assertion is itself part of that record. The run is closed by its own receipt — event kind `adopt`, carrying the same fields as a refresh receipt: `git_head` and the list of absorbed event ids — so "these nodes were adopted as pre-existing work at this git head" is a fact in the journal forever, distinguishable from a refresh rather than dressed as one. The journal-line schema enumerates receipt kinds, so the new kind is a schema change and is accounted for below.

### What it does not do

It does not create tasks, close tasks, or touch a tracker in any way. It does not accept a changeset or receipts carrying ops — the same refusal refresh makes, for the same reason. It does not migrate a spec, author one, or judge whether a leaf's claims are true of the code. It runs exactly once in a project's life, and the empty-journal gate is what enforces "once".

### The work around the mode

Adoption is two jobs, and the mode is only the second. The first is producing a spec that validates: for `faber`, a format migration; for `portitor`, authoring from seeds, with identity hashes and a `project.json`. Both are structural work that `2026-09-14-authoring-commands` delivers as commands — migration honouring the format-version contract, and node creation over the resolved profile — and this proposal depends on it for that reason. Nothing in the mode itself is deferred by the dependency; what is deferred is an adopter's ability to reach the mode without hand-editing a spec into shape.

Two pieces ride with the epic outside the graph. `/mint`, the one place the baseline moves, gains an adopt branch beside pipeline and refresh, run with a stated reason like a refresh is. And an adoption skill holds the judgement no component can hold: before the run, for each component, that its arch leaf names code that exists and its test leaf names tests that exist. That parity pass is the adopter's assertion made checkable before it is made permanent; the skill ends by handing to `/mint`, and baselines nothing itself.

## Impact expectation

### New nodes — ingest module

| Node | Type | Task |
|---|---|---|
| Adopt mode for existing code | module requirement | no |
| `AdoptHandler` | component | yes |
| Adopt mode tests — describes `AdoptHandler` | test section | no (one component) |

The new requirement derives from [[81f8102ae1b5|apply changes]], as the module's other pathway requirements do. `AdoptHandler` implements it, takes a `uses` edge on `JournalEncoder`, and is described by the new test section, so coverage holds without further nodes.

### Modified nodes

| Module | Node | Hash | Task | Why |
|---|---|---|---|---|
| ingest | `IngestCommand` | `db90eb607bcb` | yes | dispatches a third pathway; gains a `uses` edge on `AdoptHandler` |
| ingest | `RefreshHandler` | `f9033352c13f` | yes | its gate becomes one half of a mutual exclusion, and the leaf must say so from its side |
| ingest | Ingest flow | `6fd1f0cbb76c` | yes | the flow gains a third pathway |
| ingest | Refresh mode tests | `a483524c406c` | no | one component |
| ingest | Ingest command tests | `3e9de336f65f` | no | one component |
| schema | `JournalLineSchema` | `65013bcc73e6` | yes | the receipt kinds gain `adopt` |
| schema | Schema loading tests | `8719672c7580` | yes | describes `JournalLineSchema` among four components |
| map | `MappingStore` | `205e67ca4aad` | yes | its event table and fold name the new receipt kind |
| map | Map command tests | `571971ee44d6` | yes | describes `MappingStore` among two components |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| ingest | No runtime subprocess | `20589ccf7072` | IngestCommand |
| ingest | Refresh mode for impl_only drift | `e68653819f38` | RefreshHandler |
| schema | Define journal-line schema | `43e38a51a7e0` | JournalLineSchema |
| map | Store mapping records | `934d627f0e90` | MappingStore |

`20589ccf7072` is forced: it enumerates the modes bound by the guarantee, and there are now three. `e68653819f38` already speaks of the future mode in the conditional; it changes to the indicative, and the mutual exclusion is stated on both sides of a two-sided invariant. `43e38a51a7e0` and `934d627f0e90` each enumerate the receipt kinds the journal may hold, so a new kind changes both.

### Meta-leaf accounting

`ingest`, `schema` and `map` all have `module.json` changes. Each also changes or adds a requirement in the same module, so the meta rule is suppressed in all three and no component beyond those listed owes a leaf. The suppression matters most in `ingest`, which has seven components after `2026-08-20-reconciler-split`.

### The checks that can actually fail

The mutual exclusion is the property worth testing from both sides, because a one-sided test would pass against a broken implementation: adopt must refuse a project whose journal has any entry, and refresh must refuse a project whose journal has none. Run together they assert that no project state admits both.

Then: an adopt run over a spec full of added components and added modules must baseline every one of them and create no task; the journal after it must answer `spex map context` for a node the run absorbed, which is the biography property; a second adopt run against the same project must be refused, because the first one's receipt is now in the journal; and a changeset carrying ops must be refused, matching refresh's existing refusal rather than inventing a second convention for the same mistake.

### Scope

Seven tasks under one epic: five component leaves — one new, four modified — one data flow, and two test sections. No spec node is renamed or removed.

**Sequencing.** The predicate this gate keys on shipped with `2026-08-20-project-lifecycle`, and `2026-08-20-reconciler-split` restructured `ingest` before a new mode was added to it; both prerequisites are met. What remains is `2026-09-14-authoring-commands`, which gives an adopter the migration and authoring path to a spec this mode can baseline.
