# Change Proposal: Ingest adopt mode

## Context

**This proposal is deferred. It is a recorded design, not scheduled work, and it should not be registered until a real second user asks for it.** Writing a proposal and leaving it unregistered mints nothing — no epic, no beads, no journal event — and that is the intended state. The reason is not doubt about the shape but about the requirements: this mode exists to serve someone adopting spex over a codebase that already exists, and nobody has yet done that. Its details should follow how an adopter actually behaves rather than a guess made from inside the repository that grew the tool.

The gap it addresses is real. Someone who writes a spec describing work that is already finished has no path today that does not propose a task per node. The normal pipeline sees every leaf as added and plans a create for each — correct for a project being born, wrong for a project being documented.

`2026-08-20-derivation-status` covers most of that problem and is the reason this ranks last. With a declared-but-underived state on project requirements, an adopter names the whole system shallowly, derives only the module they are actually working on, and lets the rest sit declared and honest. Tasks are created for the module in flight and nothing else. Most people arriving with a codebase will do exactly that and never need what follows.

What it does not cover is someone who wants to spec their entire existing system in full detail — components, flows, test sections — as documentation, without generating hundreds of tasks for work that shipped years ago. That single case is what this serves.

### Why refresh cannot serve it

`spex ingest --mode refresh` looks like the answer and is not, for three independent reasons, all of them deliberate.

It refuses a run with no pre-existing snapshot, on the grounds that without a baseline every leaf looks added and that is the bootstrap case, which belongs to the normal pipeline. It refuses a run whose changeset or receipts carry ops, because refresh has no per-op transitions.

And the decisive one: it absorbs only some node types, and only in some directions. Added `requirement` and `api` are absorbable, `component` is removal-only, and everything else — `data_flow`, `test_section`, `module`, `meta` — is refused in both directions. The reasoning recorded against the component case is the clearest statement of the invariant this mode must not break:

> An *added* component is a bead that was never created; baselining it into the snapshot would remove it from `spex diff` permanently, which is precisely the bead lifecycle refresh must not bypass.

A retro-written spec is a diff full of added components, data flows and test sections. It hits that wall on its first entry, and a real project also brings added `meta` leaves, which are refused for their own reason — absorbing one would baseline a whole module appearing without any gate seeing it.

The conclusion is not that the allow-list is too strict. [[e68653819f38|Refresh mode]] exists for drift that owes no bead work, and it is right to refuse anything that does. Retro-adoption makes the opposite claim: these nodes *would* owe work under the normal rules, and the adopter is asserting the work is already done in code, outside the tracker. Refresh has no way to express that assertion, so it refuses — correctly. Loosening the allow-list to let it through would make the bypass permanently available to ordinary operation, which is exactly what the comment above says must not happen.

The assertion needs its own mode, its own gate, and its own record.

## Proposed change

### A third pathway behind the same surface

`--mode adopt`, behind the existing [[3589714e50f8|spex ingest]] api. Not a new command: [[20589ccf7072|no runtime subprocess]] states that `--mode refresh` "selects a pathway behind that single surface rather than standing up a second one, so both modes are bound by the guarantee above and neither can drift away from it in the graph". Adopt is bound the same way, and that requirement's wording changes from two modes to three.

A new component `AdoptHandler` owns the pathway, alongside `RefreshHandler` (`f9033352c13f`), both dispatched by `IngestCommand` (`db90eb607bcb`).

### Gated on an empty journal, which is the whole design

`AdoptHandler` refuses any run where the task journal is not empty.

That single predicate is what makes this safe, and it is structural rather than documentary. `2026-08-20-project-lifecycle` re-keys refresh's bootstrap guard from "does a snapshot exist" onto "is the journal empty" — refresh requires a **non-empty** journal, meaning a cycle has completed. Adopt requires an **empty** one. The two are therefore mutually exclusive by construction: once a single cycle has run, adopt is unavailable forever, and it cannot be reached for later as a way around the bead lifecycle. Neither handler needs to trust the other, and no document has to ask anyone to be careful.

This is also why the ordering matters: without that predicate in place, adopt's gate would have nothing to key on.

### One event per absorbed node, so the biography survives

`RefreshHandler` absorbs "by appending one change event per absorbed drift entry to the task journal, closed by one refresh receipt, and rewriting the snapshot — atomically, with no bead lifecycle." `AdoptHandler` does the same, for the same reason: a node baselined with no journal event has no record it ever existed, so [[3c8a43221ed2|spex map context]] would answer nothing for it, permanently. The journal is where a node's biography lives, and adoption must not create nodes that were never born.

The adopter's assertion is itself part of that record. The run is closed by its own receipt, so "these nodes were adopted as pre-existing work on this date at this git head" is a fact in the journal forever, rather than something a later reader infers from a suspiciously empty tracker.

### What it does not do

It does not create tasks, close tasks, or touch a tracker in any way. It does not accept a changeset or receipts carrying ops — the same refusal refresh makes, for the same reason. It runs exactly once in a project's life, and the empty-journal gate is what enforces "once".

## Impact expectation

### New nodes — ingest module

| Node | Type | Bead |
|---|---|---|
| Adopt mode for existing code | module requirement | no |
| `AdoptHandler` | component | yes |
| Adopt mode tests — describes `AdoptHandler` | test section | no (one component) |

The new requirement derives from [[81f8102ae1b5|apply changes]], as the module's other pathway requirements do. `AdoptHandler` implements it and is described by the new test section, so coverage holds without further nodes.

### Modified nodes

| Node | Hash | Bead | Why |
|---|---|---|---|
| `IngestCommand` | `db90eb607bcb` | yes | dispatches a third pathway; gains a `uses` edge on `AdoptHandler` |
| `RefreshHandler` | `f9033352c13f` | yes | its gate becomes one half of a mutual exclusion, and the leaf must say so from its side |
| Ingest flow | `6fd1f0cbb76c` | yes | the flow gains a third pathway |
| Refresh mode tests | `a483524c406c` | no | one component |
| Ingest command tests | `3e9de336f65f` | no | one component |

### Modified requirements, and the leaves they oblige

| Requirement | Hash | Obliges |
|---|---|---|
| No runtime subprocess | `20589ccf7072` | IngestCommand |
| Refresh mode for impl_only drift | `e68653819f38` | RefreshHandler |

`20589ccf7072` is forced: it enumerates the modes bound by the guarantee, and there are now three. `e68653819f38` is forced too, though less obviously — the mutual exclusion is a property of both handlers, and a refresh requirement that does not mention it leaves the guarantee stated on only one side of a two-sided invariant.

### Meta-leaf accounting

`ingest/module.json` changes — one component, one requirement and one test section added, plus `uses` edges. That moves the module's `meta` leaf, and `ingest` has four existing components, so an unsuppressed meta rule would oblige leaves from `Reconciler` (`2b5158af774b`) and `SnapshotSaver` (`f85bd2f94aeb`) as well.

It is suppressed, because `20589ccf7072` and `e68653819f38` both change in the same module.

### The checks that can actually fail

The mutual exclusion is the property worth testing from both sides, because a one-sided test would pass against a broken implementation: adopt must refuse a project whose journal has any entry, and refresh must refuse a project whose journal has none. Run together they assert that no project state admits both.

Then: an adopt run over a spec full of added components must baseline every one of them and create no task; the journal after it must answer `spex map context` for a node the run absorbed, which is the biography property; and a changeset carrying ops must be refused, matching refresh's existing refusal rather than inventing a second convention for the same mistake.

### Scope

Four beads under one epic: three component leaves — one new, two modified — and one data flow. No test section crosses the two-component threshold, so none produces a bead of its own.

**Sequencing, if it is ever scheduled.** It requires the journal-empty predicate from `2026-08-20-project-lifecycle`; without it the gate has nothing to key on. It should follow `2026-08-20-reconciler-split`, because restructuring `ingest` underneath a new mode is harder than restructuring before one exists.

And to repeat the position at the top, since a proposal outlives the conversation that produced it: **this should stay unregistered until someone outside this repository actually needs it.** The design is recorded so the question does not have to be re-derived; the implementation should wait for a user whose adoption it can be checked against.
