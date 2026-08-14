# Resolver

Resolves each create or retarget action's `DepSpecNodeIDs` — and each create's parent — into refs the adapter
can apply blind, and computes each create's priority via the project-requirement chain.

## The Two Ref Shapes

A dep an action names is written as one of the shapes below, or dropped —
[[f68bda93df12|the encoding that lets a changeset name work that does not exist yet]]. For each
dep spec_node_id, first match wins:

1. **`ref:op`** — another create op in the same batch targets this spec_node_id.
2. **`ref:bead`** — the task journal's fold already pairs this spec_node_id with a task that is
   not closed. The fold's latest-wins rule means the newest re-implementation supersedes earlier
   ones by construction.

That ordering is a precedence, not a search order: a dep that is both in the batch and open in
the fold is written `ref:op`, because the in-batch op is the authoritative latest work and the
fold can be stale before the batch lands.

If the fold's pairing for the spec_node_id is closed, the dep is **dropped** — the work is
satisfied, no edge needed.

A dep that is neither in-batch nor in the fold is a **plan error** naming the spec_node_id.
Changeset v2 removed the `ref:spec_node` adapter-time fallback and v3 keeps that: the adapter no
longer reads any spex-owned file, so nothing downstream could resolve what the builder cannot.
Failing at build time puts the error where the operator can still fix the input.

## Why These Shapes

`ref:op` is the structural fix for the broken-dep-graph bug (commit `21defea`). Pre-decouple,
deps were resolved against stored state at classification time. When a dep was itself being
obsoleted+recreated in the same batch, the resolver picked up the OLD (soon-closed) task ID, passed it
to `br create --deps blocks:<old>`, and the close phase then killed the referenced task — leaving
the new task pointing at a dead predecessor. `ref:op` sidesteps the problem by deferring
resolution to adapter-exec time: the adapter builds A, knows A's fresh task id, then builds B
with `--deps blocked-by:<A-new>`. Structural guarantee, no post-hoc patching.

Resolver is where the project requirement's "parent hierarchy, lineage tracking, and priority
propagation" is actually decided, and it decides all three **without knowing what tracker will
execute the result**. A ref names an op or a task — never a `br` flag. The translation to
`--parent`, `--deps <edge>:<id>` and `--priority` happens in the adapter, which is the only
component permitted to know a tracker's command surface. That separation is what lets a second
adapter target a different tracker against an unchanged changeset: an adapter needs create, close,
update, a label and an identifier, and never a view into spex's own files.

## Retarget deps

A retarget op's recomputed `DepSpecNodeIDs` go through [[7d45c20bd0f7|exactly the same
classification]]: in-batch op → `ref:op`, open fold pairing → `ref:bead`, closed pairing dropped,
unresolvable is a plan error. Nothing about the shape distinguishes a retarget's dep from a
create's — what differs is downstream application, where the adapter adds missing deps to the
existing task and removes none. Dep *removal* is deliberately absent from the vocabulary: a dep
the task carries and no longer needs is closed by its own lifecycle, and expressing removals here
would make the op non-idempotent. Retargets take no parent and no priority — the task already
sits under its epic at its priority, and only its target state and deps move.

## Parent Resolution

The proposal epic is the parent of every non-epic create in the run, and [[13296c25e250|the
journal fold is consulted first for which epic that is]]. The epic's identity is the proposal's
`registered` event, and two distinct journal reads answer for it together: the run's
**registration** — the `registered` event the journal holds for the proposal ref, if it holds one
— and the **fold**, which answers only whether an epic task is already paired to that event. The
fold cannot answer the first question on its own: it lists task-bearing pairings, so a proposal
registered but not yet epic'd and a proposal never registered miss it identically. Registration
is therefore read from the journal's events directly. The two answers combine in one order: the
fold is asked first, and the registration decides only what its silence means.

- If the fold **already pairs an epic task** with the proposal (re-run of a partial run, or
  idempotent re-emit — including legacy epics whose pairing reaches the fold through its
  read-only legacy branch), the epic op is skipped; each create's `parent` is
  `{ref:bead,bead_id:"<existing epic task>"}`. An epic that already exists wins over an in-batch
  one, so a re-run that misread the epic as new still parents its creates under the task that is
  already there. This branch is checked before the registration is consulted at all, which is
  what keeps a legacy epic — paired to a proposal whose lifecycle predates the `registered` event
  entirely — resolvable rather than an error: a live epic task is proof enough that the lifecycle
  is open.
- If the fold pairs no epic task and the run **has a registration**, the epic is a new create op
  (the first op), labeled with the registered event's eid. Each subsequent create's `parent` is
  `{ref:op,op_id:"<epic op>"}`.
- If the fold pairs no epic task and the journal holds **no registered event** for the proposal,
  that is a plan error naming the slug: registration opens the lifecycle, so the fix is
  `spex register`, not a synthesized epic. The fold's silence alone never decides this — it says
  only that no epic task exists yet, and it is the registration read that separates "not epic'd
  yet" from "never registered".

The epic's own `spec_node_id` is synthetic: no node in the spec tree corresponds to it, so the
value carried in `changeset.json` is the proposal ref itself rather than a 12-hex identity hash,
and a reader can tell the epic apart from every other create by that shape alone — as can the
fold, which lists epics keyed by the slug the registered event carries.

## Priority

Per create action, [[ab7176690bfb|priority is inherited from the project requirements the
component ultimately implements]]. Walk the chain:

1. Component → `implements: [req_id, …]`.
2. For each `req_id`, module requirement → `preq_id`.
3. Project requirement → `priority` field.

Take the **minimum** priority across all reachable project requirements. Lowest number wins.

If no priority chain is reachable (missing `preq_id` or missing project req), apply a
deterministic fallback: priority `3` (mid-range). The fallback is silent — the op carries `3` and
nothing in the changeset or on stderr records that the chain was unreachable — and it is not an
error, because the validator's `requirement_coverage_checker` is the authoritative gate for
upstream chain completeness.

## Interface

Resolver is set up with four things — the spec graph, the journal fold, the run's registration,
and the batch map of spec_node_id to op_id — and answers three questions per create action (what
refs its deps become, what ref its parent becomes, and what priority number it carries) and one
per retarget action (what refs its deps become).

Its read surface on the spec graph is deliberately narrow. It reads the implements → preq_id →
priority chain and nothing else, so nothing about a component's name, description or `uses` edges
can reach a ref or a priority. The command adapts the parsed spec directory onto that surface;
tests substitute a stand-in — and the fold is an equally narrow surface: node key in, latest
task-bearing pairing out. The registration is narrower still: one fact for the run, the eid of
the proposal's `registered` event or nothing at all, resolved by the command from the journal's
parsed events before the builder is assembled. It is a separate input precisely because the fold
does not carry it: a registration with no epic task yet is not a pairing and has no place in a
list of pairings.

The batch map must be complete before any dep is resolved. TopologicalSorter runs first for
exactly that reason: it fixes the order the op_ids are handed out in, and ChangesetBuilder hands
Resolver the finished map before the first dep is classified.

## Every ref names a node that can own a bead

`DepSpecNodeIDs` arrives already filtered: the classifier produces actions only for the node kinds
that can own a bead, and the sorter refuses to tier a create whose kind it does not recognise. The
two ref shapes plus the drop and the error are therefore exhaustive over what actually reaches
Resolver. A spec node that has no task and can never acquire one has no place in a dep list; that
malformation surfaces at build time as the unresolvable-dep error instead of travelling to
the adapter as a hash nothing can satisfy.

The priority walk enters the same set from the other end. It starts at a component's `implements`
array, so a project requirement's priority reaches a bead through the component that implements
the requirement and through no other kind of node.

## Determinism

- Iteration order over `DepSpecNodeIDs` is preserved as the classifier emitted it (a
  deterministic order).
- Priority computation uses `min` on a finite set; result is independent of enumeration order.
- Parent resolution has one deterministic output per `(proposal, journal state)` pair.

## Test surface

Resolver has no public API surface independent of `ChangesetBuilder` — nothing else in the
module's `uses` graph consumes it. Cross-component integration coverage (Resolver paired with
Sorter, Labeler, and Builder) lives in `test_changeset_builder`'s `describes` array, exercised
through `Builder.Build()`'s public API; the retarget-path pairing with ActionClassifier lives in
`test_classification`. Per-method unit tests for the individual classification,
priority, and parent-resolution paths live in `plan/resolver_test.go` and ship with this
component's implementation bead.
