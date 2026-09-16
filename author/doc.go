// Package author is the write path over the spec: the eight `spex node`,
// `spex edge`, `spex leaf`, `spex profile` and `spex migrate` surfaces that
// change spec/ and refuse with the validator's own predicate rather than a
// rule of their own. See spec/author/flow_authoring.md and
// spec/author/module.json (data_flow node f72baab0289d).
//
// # Position in the authoring loop
//
// An authoring skill reads a proposal, runs `spex profile show` to learn
// the project's declared types without ever naming one in the skill
// itself, then drives the tree towards the proposal's impact table one
// command at a time: `spex node add`/`remove`/`rename` to declare or
// retire nodes, `spex edge add`/`remove` to wire reference fields,
// `spex leaf scaffold` to lay down a content leaf's skeleton, and
// `spex migrate` once, first, for a spec born under an earlier format.
// Every write among those five surfaces passes through the same stage
// before it reaches disk: apply the change to an in-memory copy, run the
// validator's checkers over it, and either refuse — the validator's own
// entry, each one carrying the fix that resolves it — or accept and print
// what the change now obliges. `spex profile show` is the one surface with
// no write and no obligations; it prints the resolved profile and nothing
// else.
//
// # Flow
//
//  1. AuthorCommands reads flags into the input shapes this package
//     declares (NodeAddInput, NodeSetInput, NodeRemoveInput, RenameInput,
//     EdgeInput, ScaffoldInput) and hands them to the matching worker:
//     NodeEditor, NodeRenamer, EdgeEditor or LeafScaffolder. Migrator and
//     ProfileInspector take no input beyond the spec directory itself.
//  2. The worker decides everything structural the caller did not name —
//     array, id, content path — from the resolved profile and the identity
//     contract, and applies the change to an in-memory copy of the spec.
//  3. ObligationReporter runs the validator's checkers over that copy: a
//     finding present in the after-state and absent from the before-state
//     is a refusal, reported as a RefusalEntry with the fix that resolves
//     it; a finding the before-state already carried travels instead as an
//     obligation, using the completeness checker's own entry type
//     (merkle.DiffError), never a re-implementation of either rule.
//  4. On refusal, nothing is written and AuthorCommands exits 2 with the
//     refusal document on stdout. On acceptance, the worker writes its
//     after-state to disk and AuthorCommands prints a WriteReport — what
//     was written, the obligations, for a rename the retired name, and
//     for an edge add that displaced a cardinality-one target the
//     replaced target — and exits 0.
//
// # What is deferred
//
// This bead (spexmachina-yih0.1) scaffolds the cross-component wire
// surface only — the shapes AuthorCommands, NodeEditor, NodeRenamer,
// EdgeEditor, LeafScaffolder and ObligationReporter all build against, so
// that whichever of their beads lands first does not invent it. Component
// work is deferred to the beads that list this one as a blocker:
// ProfileLoader/SchemaLoader (spexmachina-yih0.2/.3), ProfileInspector
// (.4), RootCommand (.5), ObligationReporter (.6), NodeRenamer (.7),
// LeafScaffolder (.8), Migrator (.9), NodeEditor (.10), EdgeEditor (.11)
// and AuthorCommands (.12).
//
// # Cardinality-one retarget correction (spexmachina-yih0.16)
//
// A drift triage against .10/.11/.6/.15's implementation corrected the
// spec (baseline bfd0982): a cardinality-one reference field (preq_id
// under the default profile) is retargeted by one `spex edge add` — the
// add replacing the held target rather than refusing — and a required
// cardinality-one field can no longer be cleared by `spex edge remove` at
// all, since a required field has no valid state to pass through
// (spec/author/arch_edge_editor.md, "Idempotence";
// spec/author/test_node_editing.md's N15). This bead widens the one
// shared shape the correction touches — WriteReport gained
// ReplacedTarget, the `replaced_target` key flow_authoring.md's "On
// stdout" now names beside `retired_name` — so EdgeEditor has somewhere
// to report the displaced target once it replaces instead of refusing.
// The behavior itself — AddEdge replacing instead of refusing, RemoveEdge
// refusing a required field instead of clearing it — is EdgeEditor's own
// (spexmachina-yih0.17): setEdgeField's cardinality-one branch now
// replaces and reports the displaced target instead of refusing;
// RemoveEdge needed no new check of its own for the required-field
// refusal — clearing a required preq_id already surfaces, through the
// normal Report pass, as the validator's own newly-introduced "schema"
// (missing required property) and "id" ("missing preq_id") findings,
// which is exactly the "same two entries a hand edit ... earns from spex
// validate" the corrected spec calls for. Asserted by N15
// (edge_editor_test.go); the CLI-level N15 scenario in
// cmd/spex/node_editing_test.go is spexmachina-yih0.19's deliverable.
// AuthorCommands (.18) needs no change of its own, since
// finishAuthorResult already prints whatever WriteReport carries.
//
// ObligationReporter (Report, in obligation_reporter.go) realises the
// before/after pair as a pair of io/fs.FS values rather than a bespoke
// in-memory struct: before is typically os.DirFS(specDir), the real spec
// directory; after is typically a validator.MemFS, the worker's own
// in-memory copy with the change already applied and never written to
// disk. Every validator checker Report drives, and merkle.BuildTree and
// merkle.CheckCompleteness beneath it, gained an *FS entry point
// (schema.ResolveProfileFS, validator.CheckSchemaFS/CheckIDsFS/
// CheckIDDerivationFS/CheckDAGFS/CheckLinksFS and the five non-refusal
// checkers, merkle.BuildTreeFS, merkle.CheckCompletenessFS) that reads
// fsys instead of a directory path; the existing directory-path functions
// became thin os.DirFS wrappers around them, so every caller outside this
// package keeps working unchanged. This is what lets a refusal cost one
// validation pass over each state and no write, rather than a full on-disk
// copy of spec/ plus a spec load per checker per state
// (spec/author/arch_obligation_reporter.md, "Refusal is the validator's
// predicate").
//
// AuthorCommands (cmd/spex's node.go, edge.go, leaf.go, migrate.go,
// author_output.go) made the refusal document's top-level JSON envelope
// call the flow and test leaves left open: a refusal prints []RefusalEntry
// itself, with no wrapper object — the same bare-array shape a write
// report's own `obligations` key already holds.
//
// # spex node set (spexmachina-444f.1)
//
// Widens the shared wire surface for NodeEditor's third surface,
// `spex node set`: NodeSetInput (types.go), carrying an existing node's
// id, declared field values keyed by name, declared field names to
// unset, or both — the shape flow_authoring.md's "Into a worker" names
// for a set ("an id with field values keyed by declared field name,
// field names to unset, or both") and arch_node_editor.md's "Setting a
// field" details. No existing shape changed: WriteReport, RefusalEntry
// and every other worker's input are untouched, since a set's report
// carries nothing beyond what a write already reports and its refusals
// are the same RefusalEntry every other surface returns.
//
// Component work is deferred to the beads that list this one as a
// blocker: ObligationReporter (spexmachina-444f.2) gains whatever
// computeFix rows a set's own schema violations need (a kind or
// enumeration mismatch reaching the checkers rather than being rejected
// as an input error, per arch_node_editor.md's "each with the
// validator's own schema entry and the declared kind or enumeration as
// the fix"); NodeEditor (spexmachina-444f.3) adds the Set function
// itself — finding the entry at either scope, converting values by kind,
// and refusing name, id, content, a reference field and a module id with
// the surface that owns each, per the table in arch_node_editor.md's
// "Setting a field"; AuthorCommands (spexmachina-444f.4) wires
// `spex node set` in cmd/spex/node.go, `--field`/`--unset` flags
// mirroring `spex node add`'s own; the CLI-level scenarios are
// spexmachina-444f.5's (test_node_editing.md).
package author
