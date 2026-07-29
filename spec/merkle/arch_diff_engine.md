# DiffEngine

[[f223179a540a|Compares the current hash tree against a stored snapshot and reports what moved]]. Same identity hash + different content hash = modified. Identity hash in current but not in snapshot = added. Identity hash in snapshot but not in current = removed.

## Responsibilities

- Compare current ID-keyed tree against a stored snapshot
- Identify added nodes (ID in current but not in snapshot)
- Identify removed nodes (ID in snapshot but not in current)
- Identify modified nodes (same identity hash, different content hash)
- Report changes with identity hashes and node metadata

## Interface

One comparison, taking the current tree and the snapshot tree and returning the list of changed leaves. It has no failure mode: two trees always compare, and where there is no stored snapshot at all every current leaf comes back as `added`.

Each entry in that list carries the key of the leaf that moved — an identity hash such as `a1b2c3d4e5f6`, or `meta/project` / `meta/<module-hash>` for the envelope leaves — the kind of movement (`added`, `removed` or `modified`), the node type, the identity hash of the owning module (empty for project-level nodes), and the two content hashes. The old hash is empty on an addition and the new hash is empty on a removal; a modification carries both, which is what lets a reader confirm from the report alone that the hash really changed.

The node type is carried as a separate field because identity hashes do not embed it — there is no way to look at `a1b2c3d4e5f6` and tell whether it points at a component or a test_section. The diff copies the node type off the merkle leaf, where the tree builder had already set it.

## Algorithm

1. Flatten both trees into identity-hash → content-hash maps
2. For each identity hash in current but not in snapshot: added
3. For each identity hash in snapshot but not in current: removed
4. For each identity hash in both with different content hashes: modified
5. Sort changes by key for deterministic output

Only leaves reach those maps. The module interiors and the root are walked through and never keyed, so no interior node is ever reported as a change. That loses nothing: an interior hash is composed from the hashes beneath it, so a moved leaf has already moved its module's hash, and reporting both would report one change twice. What the change is *about* comes from the node type on the leaf, not from the level it sat at.

Step 5 is what makes the result reproducible. Changes come out in ascending lexicographic key order, with no timestamp and no map iteration order surviving into the list, so the same pair of trees always yields the same entries in the same order. Identity hashes sort consistently because they are pure lowercase hex.

## Rename Stability

A module-scoped node's identity hash derives from its module, its node type and its own `name` — never from a filesystem path. So renaming a module directory or moving a content file produces no remove + add: the keys are untouched and a content edit alongside the move is still reported as a single modification.

Renaming the *node* is the opposite case. Changing a `name` changes the identity string it derives from, so the node's `id` changes with it, and the diff sees the old key vanish and a new key appear: one `removed` and one `added`, with nothing tying them together. This is the "rename is destructive" behaviour the rest of the pipeline is built on — in the bead layer a rename is a delete plus a create, never an edit.

## Bootstrap behavior

`DiffEngine` is invoked by `spex diff` against two trees: the one
[[dfe1467b7a4b|TreeBuilder]] just built from the current spec, and the one
[[b2fcd9457a28|SnapshotStore]] read back from the stored snapshot. On a fresh
project where `spec/.snapshot.json` does not exist, `spex diff` loads no
snapshot at all. DiffEngine is handed the populated current tree and nothing
against it, and reports every leaf as `added`. That is the
diff input the rest of the pipeline (impact → emit → adapter → ingest)
consumes to produce the first bead-map and the first snapshot. There is no
"prime the snapshot" step beforehand — bootstrap and steady-state share
the same DiffEngine call.
