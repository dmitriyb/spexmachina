// Package emit decides the next bead-action changeset deterministically.
//
// emit is a pure function over four inputs: an impact report, the task
// journal's fold, the spec graph, and a caller-supplied git HEAD SHA. It
// produces changeset.json — an ordered, tool-agnostic list of
// label/create/close/tag operations with forward-reference encoding — that
// an external adapter consumes.
//
// # Flow
//
// The pipeline within ChangesetBuilder.Build is:
//
//  1. Load inputs (impact report, journal fold, spec graph, git_head).
//  2. Partition create actions into type tiers
//     (proposal_epic / feature+data_flow / multi-component test).
//  3. TopologicalSorter orders ops within each tier (Kahn + lex tiebreak),
//     assigning op-NNNN ids. Returns batchMap: spec_node_id → op_id.
//  4. IdempotencyLabeler assigns each create op's label as a pure function
//     of the op itself — no cursor, no store read.
//  5. Resolver classifies each create's deps and parent into ref:op /
//     ref:bead and computes priority via the implements → preq_id →
//     project_requirement chain.
//  6. ChangesetBuilder composes the Op records, appends close ops, and
//     emits the v2 changeset with canonical field order.
//
// # Determinism
//
// Same (spec state, journal fold state, git_head, impact report) inputs
// always yield byte-identical changeset output: stable op ordering, fixed
// JSON field order, and labels read off each op rather than any external
// counter.
package emit
