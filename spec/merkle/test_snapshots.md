# Snapshot Tests

Integration and acceptance tests for the SnapshotStore (component 3). Validates that merkle trees can be serialized to JSON snapshot files, deserialized back, and that the round-trip preserves all tree information.

## Setup

Scenarios use `t.TempDir()` to create an isolated working directory and build their tree with `setupSpecDir(t)` followed by `BuildTree`, so the fixture is exactly the flat identity-hash shape TreeBuilder produces (a module interior node whose children are the `meta/` envelope leaf plus one leaf per spec node, each keyed by its 12-char hex identity `id`). Sketched, that shape is:


The snapshot file path is `<tmpdir>/.snapshot.json`; in production the path is not a constant — it is the location inside `.spex/` that the lifecycle pre-flight ([[a9aa93774cc2|ProjectResolver]]) answers. `Save` takes an explicit timestamp — `Save(tree *Node, path string, createdAt time.Time)` — and S5 passes a fixed `createdAt`.

## Scenarios

### S5: Load handles a snapshot produced from a real spec tree

**Given** a full spec directory fixture (project.json, module.json, content files)
**When** `BuildTree` is called to compute the current tree
**And** `Save` writes the snapshot with a fixed `createdAt`
**And** `Load` reads the snapshot back
**Then** the loaded tree is structurally and hash-identical to the computed tree

**Rationale**: End-to-end integration between TreeBuilder and SnapshotStore. This is the real-world usage path: `spex ingest` builds a tree and saves the snapshot on a complete run, then `spex diff` loads it later as the baseline. (The former standalone `spex hash` writer was removed by the pipeline-cleanup proposal.)

## Edge Cases

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
