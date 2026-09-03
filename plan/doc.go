// Package plan decides the next task-action changeset deterministically, in
// one pass from the merkle diff: reads the task-state artifact, matches
// changed spec nodes against the task journal, classifies each into
// create, close or retarget, and composes changeset.json v4 — an ordered,
// tool-agnostic operation list with forward-reference encoding — that an
// external adapter consumes. See spec/plan/flow_plan.md and
// spec/plan/module.json (data_flow node 878c885a2517).
//
// # Position in the pipeline
//
// plan is the third of five stages: spex validate gates, spex diff produces
// the classified changes, plan turns them into changeset.json, the adapter
// executes that and writes receipts.json, and spex ingest reconciles the
// pair. Every hand-off from diff onwards goes through a file, so each stage
// can be re-run from the artifact its predecessor wrote. A partial run —
// any op with status: error in the receipts — leaves the snapshot
// untouched, so the next diff recomputes from the same baseline and the
// next plan run sees only the ops that never landed.
//
// # Flow
//
// One invocation, `spex plan --proposal <ref> --git-head <sha> --tasks
// <file> [--diff <file>] [--absorb <file>]`, runs eight steps and writes
// one file:
//
//  1. PlanCommand loads the diff (refusing one carrying errors), folds the
//     task journal and resolves the run's registration, loads the spec
//     directory, parses the absorb list and withholds every validly marked
//     change from the stream, composing the absorbed entries off the
//     withheld diff entries.
//  2. TaskReader parses the required --tasks artifact — the version-1
//     task-state document listing only in-flight (open/in_progress) tasks;
//     each listed task's live status joins onto the fold's pairing whose
//     task id matches, in memory on the way past.
//  3. NodeMatcher joins the enriched pairings to the diff's changes on
//     identity hash, splitting the result into matched, unmatched and
//     orphaned.
//  4. ActionClassifier turns the three lists into create, close and
//     retarget actions, consulting the spec directory for what the diff
//     cannot tell it. An open task's node changing retargets it in place;
//     there is no close-and-recreate step.
//  5. TopologicalSorter partitions the create actions by tier and orders
//     each tier with Kahn's algorithm and a lex tiebreak.
//  6. IdempotencyLabeler answers with one spex:<eid> label per create
//     action, keyed to the journal event its task_created will reference.
//  7. Resolver writes each dep as ref:op or ref:task, points every
//     non-epic create's parent at the proposal epic, and walks
//     implements -> preq_id -> priority for each create's priority number.
//  8. ChangesetBuilder assembles the create ops, appends the retarget ops
//     and one close op per closed task, writes the absorbed array, and
//     answers with the finished changeset.json v4 in canonical field
//     order. PlanCommand writes that answer to stdout, or atomically to
//     --out — the builder composes the document, the command owns the
//     sink.
//
// This package's wire shape — ChangesetVersion, Ref/RefTask/Ref.TaskID,
// the create/close/retarget-only Op vocabulary — is v4 as of
// spexmachina-swvx.6, ahead of the component rewrites that consume it:
// today's ActionClassifier, Resolver, ChangesetBuilder and
// IdempotencyLabeler still implement the pre-task-lifecycle "obsolete,
// then recreate" shape the flow above describes as the target
// (ActionObsolete, Action.OldTaskID, the create's blocks-edge lineage dep
// on Ref.EdgeType) pending their own beads — spexmachina-swvx.16, .19, .20
// and .13 respectively — and the --beads/BeadReader task-state input this
// doc's step 2 describes as TaskReader/--tasks is still ReadBeads/ReadBeadsBytes
// behind --beads pending spexmachina-swvx.14 (TaskReader) and
// spexmachina-swvx.7 (BeadReader cleanup). See each named TODO(bead:…) at
// its call site.
//
// # Contract surface
//
// The wire-format Changeset / Op / Ref / Idem / AbsorbedEntry types — the
// JSON changeset.json v4 composes and the external adapter reads — the
// classifier -> builder-chain Action / OrderedOp shapes, plus the op-kind,
// action-type, spec_node_kind, ref-kind and label vocabularies, the tier
// and fallback-priority constants, and the schema version constant, are
// all public. Each step of the flow above ships as a component in this
// package: ReadBeads/ReadBeadsBytes (BeadReader, standing in for
// TaskReader until spexmachina-swvx.14), MatchNodes (NodeMatcher),
// ClassifyActions (ActionClassifier), Sort (TopologicalSorter), Labeler
// (IdempotencyLabeler),
// ResolveDeps/ResolveEpicAction/ResolveParent/ResolvePriority (Resolver),
// and Builder.Build (ChangesetBuilder). PlanCommand, the orchestrator that
// calls them in the order above, lives in cmd/spex/plan.go, outside this
// package.
package plan
