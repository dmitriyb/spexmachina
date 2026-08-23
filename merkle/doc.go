// Package merkle computes a SHA-256 merkle tree over a spec directory,
// persists it as a JSON snapshot, and diffs the current tree against the
// stored snapshot to surface what changed.
//
// # Hash computation flow
//
// Three components compose to turn a spec directory into a tree and back
// onto disk:
//
//   - Hasher       — SHA-256 of file bytes (leaf hashes) and SHA-256 of
//                    sorted child hashes (interior hashes).
//   - TreeBuilder  — walks project.json + module.json, calls Hasher per
//                    leaf, and assembles interior nodes bottom-up.
//   - SnapshotStore — reads spec/.snapshot.json for diff input; a
//                    missing file is an error (ErrSnapshotAbsent), never
//                    a fallback tree. Writes are exclusively performed
//                    by ingest's SnapshotSaver.
//
// The composition has no standalone CLI surface: a separate "compute and
// persist" step would either write a snapshot matching the current spec
// (next spex diff sees zero changes, pipeline stalls) or desync the
// snapshot from the task journal (breaking the snapshot+journal atomicity
// invariant). Both fail modes are avoided by running the composition
// inside spex diff (read path) and inside spex ingest's SnapshotSaver
// (write path).
//
// # Bootstrap (seeded snapshot)
//
// spec/.snapshot.json exists before the first diff: spex init seeds it
// with EmptyTree, the one place that baseline is produced. SnapshotStore
// loads that seed like any other snapshot; DiffEngine reports every
// current leaf as "added"; plan → adapter → ingest then create the first
// beads, and SnapshotSaver writes the first real snapshot once the
// journal's events are committed. A Load that meets an absent file
// instead of the seed reports ErrSnapshotAbsent — that is the lifecycle
// pre-flight's finding (uninitialised or broken project), not a
// bootstrap signal. See spec/merkle/flow_hash_computation.md.
//
// # Steady state
//
// Each subsequent change cycle uses the same composition: spec edit →
// spex diff (TreeBuilder rebuilds; SnapshotStore loads the stored
// snapshot; DiffEngine compares) → spex plan → adapter → spex ingest
// (SnapshotSaver overwrites the snapshot iff receipts are "complete").
//
// # Cross-component contract
//
// Wire shapes flowing between the three participating components are
// declared in spec/merkle/flow_hash_computation.md ("Data Shapes"). A
// change to any field there is a contract change — Hasher, TreeBuilder,
// and SnapshotStore must move in lockstep.
package merkle
