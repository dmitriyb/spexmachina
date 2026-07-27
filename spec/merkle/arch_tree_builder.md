# TreeBuilder

Builds the merkle tree from the parsed spec graph. Each node's key is the spec node's identity hash — the same 12-character hex string stored in its JSON `id` field.

## Responsibilities

- Parse the spec graph from project.json and module.json files
- Create leaf nodes keyed by identity hash for each content file
- Create leaf nodes keyed by identity hash for each requirement (module-level and project-level)
- Create interior nodes keyed by the module's identity hash
- Compute hashes bottom-up

## Tree Structure

Nodes are keyed by identity hash, not by file path or by path-style integer composition. The key of a component, requirement, impl_section, data_flow, or test_section is exactly the value of its `id` field. This makes the tree rename-stable for file moves and — because the identity hash is also what the mapping store uses as `spec_node_id` — eliminates any rekeying when impact analysis correlates merkle changes against existing beads.

```
project (root, key: "project")
├── project.json           (leaf, key: "meta/project")
├── project req            (leaf, key: <identity hash of project req>)
├── ...
├── schema module          (interior, key: <identity hash of module "schema">)
│   ├── module.json        (leaf, key: "meta/<identity hash of module>")
│   ├── requirement        (leaf, key: <identity hash of req>)
│   ├── component          (leaf, key: <identity hash of component>)
│   ├── impl_section       (leaf, key: <identity hash of impl_section>)
│   └── ...
├── validator module       (interior, key: <identity hash of module "validator">)
│   └── ...
└── ...
```

The only synthetic keys are for the JSON envelope leaves that have no `id` field of their own:

- `meta/project` — for `project.json` itself
- `meta/<module-identity-hash>` — for each `module.json`

These are unambiguous because real identity hashes are pure hex (no `/`).

## Interface

```go
type Node struct {
    Key      string   // identity hash, or "meta/..." for envelope leaves
    Hash     string   // content hash
    Type     string   // "leaf", "module", "project"
    NodeType string   // "component", "impl_section", "data_flow", "test_section", "meta", "requirement", "module"
    Module   string   // identity hash of the parent module ("" for project-level nodes)
    Children []*Node
}

func BuildTree(specDir string) (*Node, error)
```

`Module` is now a string identity hash rather than an integer module ID, so any consumer that needs to know which module a leaf belongs to can compare against module-level identity hashes directly.

## Algorithm

1. Read `project.json`. Hash it as the `meta/project` leaf. For each project-level requirement, create a leaf keyed by its identity hash (`id` field) with a deterministic JSON hash of its fields.
2. For each module entry, read its `module.json`. Hash it as the `meta/<module-identity-hash>` leaf.
3. For each module requirement, create a leaf keyed by its identity hash with a deterministic JSON hash of its fields.
4. For each component, impl_section, data_flow, and test_section, hash the referenced content file and key the leaf by the node's identity hash. A node whose `content` field is empty is skipped — see "Empty content is not a node" below.
5. Compute the module interior hash from its sorted child hashes.
6. Compute the project root hash from sorted module hashes plus the `meta/project` leaf and the project requirement leaves.

## Empty content is not a node

For components, impl_sections, data_flows and test_sections, TreeBuilder skips
any entry whose `content` field is the empty string. The skip is silent: no leaf
is created, nothing is logged, and no error is returned.

The consequence is total, not partial. A skipped node has no leaf, so it has no
hash; with no hash it can never appear in a diff, so impact never sees it, emit
never emits an op for it, and it never acquires a bead. It is declared in
`module.json` and invisible to every stage of the pipeline — and because its
absence is also stable across runs, no diff ever reports it as missing. The
failure mode is a spec node that exists to a reader and does not exist to the
tool.

This is why `content` is **required** with `minLength: 1` on all four node types
in `module.schema.json`. The constraint is what makes the skip unreachable: a
schema-valid `module.json` cannot contain an entry the tree builder would drop.
The branch remains in TreeBuilder as a defensive guard against a malformed file
reaching it ahead of validation — it must not be read as an opt-out. A node that
genuinely has no prose still needs a content file; the way to declare a node
without a content leaf is to use an `api`, which hashes from its JSON fields and
has no `content` field at all.

Requirements and apis are unaffected: neither has a `content` field, and both
hash from a deterministic serialization of their JSON fields.

## Why this keying scheme

Earlier versions used path-style keys like `module/3/component/2` built from integer module and node IDs. Two problems followed: (1) integer collisions on parallel branches forced manual coordination, and (2) the mapping store used a different format (`<module-name>/<node_type>/<id>`), so the impact command had to translate between the two via `buildMerkleIndex` and `deriveSpecNodeID`. Both translations are deleted now: the merkle tree and the bead-map use the same identity hashes as keys, so impact looks up changed merkle nodes directly in the mapping store with no rewriting.

## Requirement Leaf Hashing

Requirements do not have content files. Their content hash is computed from a deterministic JSON serialization of the requirement's fields. Fields are sorted by key and zero-value/omitted fields are excluded (matching `omitempty` semantics).

For module-level requirements, the serialized fields are: `depends_on`, `description`, `id`, `preq_id`, `title`, `type`.

For project-level requirements, the serialized fields are: `depends_on`, `description`, `id`, `priority`, `title`, `type`.

The `id` field is now a 12-character hex string rather than an integer, but it is still serialized as part of the leaf so that a node which is moved between modules (and therefore gets a new identity hash) is detected as a content change too — not just a key change.

This ensures that any change to a requirement's text, type, dependencies, or derivation produces a different content hash, making it visible as an individual change in the diff.

## Call sites

TreeBuilder is composed (not invoked directly by users) inside the two
merkle call paths:

- `spex diff` — builds the current tree once per invocation, hands it to
  DiffEngine for comparison against the snapshot loaded by SnapshotStore.
- `spex ingest` SnapshotSaver — builds the current tree to write out the
  fresh snapshot.

There is no standalone tree-building CLI. The first `spex diff` on a fresh
project builds the current tree and compares against the empty-tree
baseline returned by `SnapshotStore.Load` when `spec/.snapshot.json` is
absent — this is what bootstraps the pipeline without a separate hash
step. See `flow_hash_computation.md` for the full bootstrap flow.
