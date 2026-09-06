# Classification Tests

Integration and acceptance tests for ActionClassifier and Resolver. These tests verify that matched, unmatched, and orphaned node results are correctly classified into create, close and retarget actions from one bounded input — the task-state artifact — and that the Resolver turns each action's spec-graph references into refs and priorities consistently with what the classifier decided. The retarget path in particular is a two-component contract: the classifier decides that a task moves, the Resolver recomputes what it now depends on.

## Setup

Each scenario states its own action batch in its Given, and the task-state artifact where the scenario reads one; placeholder names (X, Y, Z, T) stand for components and the tasks the artifact lists. The scenarios share only the resolved default profile and the plan run's modified-event label, which every retarget op carries.

## Scenarios

### S6: Resolver recomputes a retarget's deps add-only

**Given** a retarget action for component X whose `uses` now names Y (in-batch create) and Z (a task the artifact lists as open):

- The retarget op's `deps` carries `{"ref":"op","op_id":"<Y op>"}` and `{"ref":"task","task_id":"<Z's task>"}` — the same two shapes create ops use, classified by the same precedence (in-batch wins over fold).
- A dep whose fold pairing names a task absent from the artifact is dropped, exactly as on a create — the work is satisfied.
- A dep that is neither in-batch nor in the fold is a plan error naming the spec_node_id — the retarget path gets no laxer resolution than the create path.
- Nothing in the op expresses dep removal: deps the task already carries in the tracker and no longer needs are untouched (add-only is the adapter's application rule, and the op simply lists the current set).

### S7: Retarget carries the run's modified-event label

**Given** a batch carrying two retarget actions for the same run's `git_head`.

The retarget op's `labels` array holds exactly one entry: `spex:<eid>` of this run's `modified` event for the node, derived from `(git_head, op_id)` exactly as a node-bearing create's `idempotency.label` is. Assert the eid embeds this op's own op_id — two retargets in one batch carry distinct labels.

### S8: Priority and parent do not apply to retargets

**Given** a batch carrying one retarget action alongside create ops whose implements → preq_id → priority chain resolves.

A retarget op carries no `parent` and no `priority` — the task already sits under its epic with its priority; only its target state and deps move. Assert both fields are absent from the op, and that create ops in the same batch still resolve parent and priority exactly as before (minimum across the implements → preq_id → priority chain, fallback 3).

### S10: Resolver precedence over a test task's `describes` deps

**Given** test_section T describing components X and Y, with Y's fate varied one fixture arm each — an in-batch create, a task the artifact lists as open, a pairing whose task the artifact does not list, and no pairing at all.

The four fates of a described component's hash, one fixture arm each — the same precedence create deps already take, with no Resolver change:

- **In-batch create**: test_section T describes components X and Y, all three fresh creates in one batch. T's create op carries `{"ref":"op"}` deps on X's and Y's ops, and the sorter emits both component ops before T's — the test task is not actionable until its components exist.
- **Live fold pairing**: Y instead has a task the artifact lists as open → T's dep on Y resolves to `ref:task`; the test task waits for the in-flight component work.
- **Pairing absent from the artifact**: Y's task is not listed → the dep is dropped, no error, and T's op carries no ref for it — the steady state, where a test against existing code stays immediately actionable.
- **No pairing at all**: Y has never been tracked by any journal event → the existing plan error naming the spec_node_id, exactly as a component `uses` dep resolves. Assert the error, not a silent drop.

A retargeted test section takes the same path: its recomputed `DepSpecNodeIDs` include `describes`, so a section retargeted in a batch that re-mints a described component gains a `ref:op` dep on the successor, applied add-only per S6.

## Dependency Collection Scenarios

For each create or retarget action, ActionClassifier walks the spec graph and records identity hashes of spec nodes the task will depend on in `DepSpecNodeIDs`. No journal lookup and no status filtering happens here — the Resolver classifies each identity hash into a `ref:op` or `ref:task` at changeset-build time, with full knowledge of the batch's op ids.

## Edge Cases

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
