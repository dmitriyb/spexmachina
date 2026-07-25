# Change Proposal: Declarative Spec Contracts and Typed Cross-Node Links

## Context

Three unrelated-looking defects found during a skill audit turn out to share one root cause.

**1. A deleted command still documented as current.** `spex apply` was removed in `a74dca3`
(2026-04-25, "Close spexmachina-0lk.3: remove ApplyCommand"). That commit touched nine files, all
code, zero spec. Ten references survive today: eight `spex apply` mentions across
`spec/map/flow_bead_mapping.md`, `spec/map/arch_mapping_store.md`,
`spec/schema/arch_bead_map_schema.md` and `spec/proposal/flow_proposal_lifecycle.md`, plus two
`ApplyCommand` mentions in `spec/impact/test_bead_matching.md` and
`spec/emit/arch_changeset_builder.md`. The `/spec` run for that proposal (`009233d`) edited
`spec/proposal/arch_history_viewer.md` and left its sibling `flow_proposal_lifecycle.md` stale in
the same directory in the same pass.

**2. A declared interface the code never had.** `spec/proposal/arch_registrar.md` has been edited
exactly once in repository history — `087a231`, the bootstrap. It declares
`func Register(ctx context.Context, proposalPath, specDir string) error`. The implementation
(`0efb835`, which touched zero spec files) is `func Register(proposalPath, specDir string) (string, error)`.

**3. Undocumented behaviour with authoring consequences.** `merkle/tree_builder.go:122` skips any
component, impl_section, data_flow or test_section whose `content` is empty — the node never enters
the tree, so it never produces a bead. `spec/merkle/arch_tree_builder.md` does not mention this.

### Why the pipeline could not see any of them

The merkle tree hashes the bytes of spec content leaves. It is complete with respect to spec-internal
structure and blind to two things.

*Prose is not an edge.* Declared references (`uses`, `describes`, `implements`, `preq_id`,
`requires_module`) are graph edges, and `validator/id_validator.go` already errors when one points at
a removed node — that check works. The ten leftovers survived precisely because they are untyped
prose inside markdown bodies, invisible to the graph by construction. Nothing changed their bytes, so
no hash moved, so no diff, so no bead.

*Code is not in the tree.* Nothing compares a declared signature to the symbol it names. `/spec-drift`
was the mechanism that once covered this; it was deleted in `2a13728` and
`docs/enforcement-migration.md` does not list it in the rule-disposition table. Portitor and faber
enforce process, not spec-code alignment.

### Why the arch layer is where this concentrates

Measured across the current spec: 27 of 52 arch leaves (51%) declare Go `func` signatures and 35
(67%) carry ```go fences, against 9 (16%) and 32 (57%) for impl leaves. Every one of the three
defects above lives in an arch or flow leaf; none is in an impl leaf. A Go signature written into a
spec leaf is a checkable duplicate of the code that nothing checks — full coupling cost, zero
correctness benefit.

The declarative parts of the spec have not had this problem. 31 leaves carry ```json data shapes, 18
carry ASCII structure diagrams, 4 already contain `digraph` blocks, and none of those forms appear in
the drift record.

### Why impl sections should not survive

Impl sections are non-bead-producing (`impact/action_classifier.go` cuts beads only for `module`,
`component`, `data_flow` and `test_section`), so they carry authoring cost with no work-item output.
They duplicate code, which is already the trackable ground truth for mechanism, creating a second
place to reconcile. And they bind the spec to a language: a stack change would require rewriting all
56 of them.

## Proposed change

Two segments, in order. Segment one changes the format and the tooling. Segment two migrates the
existing corpus to it. Segment two cannot begin until segment one has shipped, and the transition
between them is a deterministic gate rather than a review.

### Segment 1 — Structural

**1.1 Typed cross-node links.** A mention of another spec node inside a content leaf becomes a typed
link of the form `[[<module>/<NodeName>]]`, with the module segment optional for same-module
references. This mirrors the identity string format the graph already uses, so resolution is the
existing `schema.IdentityHash` path and a rename breaks the link exactly as it breaks the node —
which is correct, since rename is delete-plus-create. The validator resolves every link to a live
node and errors on any that does not resolve.

**1.2 Code-span enforcement.** The validator scans code spans and fences only — not running prose —
for tokens matching a known node name, and errors when one is not a typed link. Scoping to code
spans is what makes this precise rather than heuristic: `` `spex apply` `` is a mention,
"the defaults apply" is prose. Measured on the current corpus, matching the bare token `apply`
yields 64 hits of which 8 are real (12.5%); matching `ApplyCommand` yields 2 of 2 (100%).

The scan is a `filepath.WalkDir` plus `strings.Contains` over roughly 157 leaves under one megabyte,
entirely standard library. No subprocess, per project requirement `58ea35f52b86`
("No runtime subprocesses").

**1.3 Removal detection becomes free.** With 1.1 and 1.2 continuously enforced, deleting a node turns
every link to it into a broken reference, which `validator/id_validator.go` already reports as an
error today. No removal-time search is required. What remains uncatchable is a paraphrase that names
nothing ("the command that applies the changeset"); no mechanical design closes that, and the
proposal does not claim to.

**1.4 Node naming rule.** The validator rejects node names that collide with common English words.
This is what keeps 1.2 precise at authoring time rather than at scan time, and it is the reason
`ApplyCommand` is a tractable target while `apply` is not.

**1.5 Arch leaves declare contracts, not signatures.** Go signatures and ```go fences are removed
from arch leaves. A component's arch leaf states responsibilities, inputs and outputs, invariants,
and error semantics in stack-agnostic terms. Structure that has a better representation than prose
uses it: DOT for state machines and graphs (`render/dot.go` already makes DOT a first-class format
here), JSON for data shapes, tables for decision matrices. A fenced `digraph` block is
syntactically checkable, which prose is not.

**1.6 Pseudocode is permitted where the algorithm is the requirement.** The test is whether the
algorithm is a choice or a requirement. If it is a choice, describe the contract and let the code
decide. If the algorithm itself is the requirement — a performance bound, numerical stability, a
consistency guarantee — pseudocode belongs in the spec and its drift cost is accepted deliberately.
This is a small, named set of leaves, not a general licence.

**1.7 Impl sections are removed from the schema.** The `impl_sections` array leaves
`module.schema.json`, and the `impl_section` node type leaves the merkle tree builder, the impact
classifier and `mapping/context_resolver.go`. Content worth keeping moves into the owning
component's arch leaf as contract; the rest is deleted.

**1.8 Stack becomes an explicit non-functional project requirement.** The language, standard-library
policy and permitted third-party tooling are declared once as a project requirement rather than
implied by Go signatures scattered across half the arch layer. There is precedent in
`58ea35f52b86`.

**1.9 The authoring skills are rewritten to the new format.** `/spec` encodes the current format
directly: the `impl_section` node type in its schema reference and identity-string table, the
`impl_<snake>.md` filename convention, the content-path table, and the per-node prose-vs-JSON checks
that reference impl sections. `/spec-review` buckets findings by `impl_section` and `test_section`
shape in its step 5. Both are rewritten for typed links, contract-shaped arch leaves, the naming
rule, and the absence of impl sections.

This is a segment-1 deliverable and a hard prerequisite for segment 2, since the eleven per-module
migration sessions run `/spec`, and an un-migrated skill would re-introduce the format the migration
exists to remove. Because `/spec` is being rewritten anyway, the defects found in the skill audit are
corrected in the same pass rather than in a second edit: the undocumented mandatory `priority` field
on project requirements (`validator/id_validator.go:163`); the two undocumented
`requirement_coverage` error phases; the incorrect claim that `--module` is required for
`hash-id --type requirement`, which contradicts both the code and
`spec/cli/arch_hash_id_command.md:28`; the stale "warnings" terminology and missing exit-2 contract
for `spex diff`; and a warning that 15 project requirements, 6 milestones and 3 test scenarios carry
legacy index-derived identity hashes that `spex hash-id` does not reproduce and that must never be
recomputed.

### Segment 2 — Migration

Scope is the whole repository, not only the arch and impl layers, because six of the ten stale
`spex apply` references live in flow leaves that segment 1.5 does not otherwise touch.

- Rewrite 52 arch leaves: strip ```go fences and `func` signatures, restate as contracts, adopt DOT
  or JSON where the content is structural.
- Fold surviving impl content into the owning component's arch leaf; delete the 56 impl leaves and
  their `module.json` entries.
- Insert typed links for every cross-node mention.
- Sweep the accumulated debt in the same pass: the ten `spex apply` / `ApplyCommand` references; the
  wrong `Register` signature in `spec/proposal/arch_registrar.md`; the empty-`content` skip, which is
  a contract rather than an implementation detail and belongs in `spec/merkle/arch_tree_builder.md`;
  and `README.md:42`, which claims `/propose` and `/spec` call `spex` subcommands for registration.

### Gating

The migration cannot run through the normal pipeline, because it rewrites the leaves the pipeline
diffs. The gate is therefore mechanical rather than procedural.

**Two-phase rule flip.** The validator rules from 1.2 and 1.4 land in segment 1 as warnings, because
the corpus does not yet satisfy them. They flip to errors when segment 2 completes. That flip is the
migration's completion gate: it is deterministic, it cannot be satisfied by partial work, and it
requires no reviewer judgement.

**One fresh session per module.** Eleven modules, eleven sessions, each starting clean. Context
exhaustion within a single long session is what produced the half-swept result this proposal exists
to fix — `009233d` editing one leaf in `spec/proposal/` and missing its sibling.

**Per-session exit criterion.** `bin/spex validate` clean, and `bin/spex diff` showing content-only
change for that module. Any session whose diff shows unexpected graph-structure change has
overstepped and is rejected.

## Impact expectation

**Segment 1** is ordinary pipeline work and produces normal beads. It touches `schema` (link syntax,
removal of `impl_sections`), `validator` (link resolution, code-span scan, naming rule), `merkle`
(drop the `impl_section` node type from the tree builder), `impact` (drop it from the classifier) and
`map` (drop `impl_files` from the context resolver). `render` may need a link-aware markdown path.
Expect a proposal epic with feature beads per component and task beads for the cross-component test
sections.

**Segment 2** is content rewriting across all eleven modules. Removing impl sections cuts no beads
directly, since `impl_section` is not a bead-producing node type. Rewriting arch leaves changes
content hashes only.

**The skill rewrites in 1.9 produce no beads and no diff**, because `skills/` is not part of the spec
graph and never enters the merkle tree. They have to be tracked as ordinary work inside the segment-1
epic and gated by review, not expected to surface from the pipeline. Their completion is a
prerequisite for scheduling segment 2.

**Two quantities must be measured before segment 2 is scheduled, not assumed.** First, removing
`impl_sections` entries changes every `module.json`, which is a `meta` change classified as
`Structural`; `merkle/completeness_checker.go` will then demand a content edit for components in
those modules, and the volume of that demand is unknown. Second, it is unclear whether the arch
rewrite qualifies for `spex ingest --mode refresh` — it is content-only in shape, but the paired
module-meta change from 1.7 may disqualify it. If refresh does not apply, segment 2 produces a large
normal-mode changeset whose bead output needs review before it is applied.

The proposal deliberately leaves both open rather than guessing, because guessing wrong about
completeness-checker volume is what turns a migration into a stalled epic.
