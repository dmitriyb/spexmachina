# TreeBuilder

Builds the merkle tree from the parsed spec graph. [[3ada6b800cc5|Each node's key is the spec node's identity hash]] — the same 12-character hex string stored in its JSON `id` field, with no module or type prefix wrapped around it.

## Responsibilities

- Parse the spec graph from project.json and module.json files
- Create leaf nodes keyed by identity hash for each content file
- Create leaf nodes keyed by identity hash for each requirement (module-level and project-level)
- Create interior nodes keyed by the module's identity hash
- Compute hashes bottom-up

## Tree Structure

Nodes are keyed by identity hash, not by file path or by path-style integer composition. The key of a component, requirement, data_flow, test_section, or api is exactly the value of its `id` field. This makes the tree rename-stable for file moves and — because the identity hash is also the task journal's node key — eliminates any rekeying when impact analysis correlates merkle changes against existing beads.

Every entry in the tree is one of the following, and its key, level and hash source are fixed by which one it is:

| key | level | node type | hashed from |
|---|---|---|---|
| `project` | root | — | the sorted hashes of its children |
| `meta/project` | leaf | `meta` | the bytes of `project.json` |
| identity hash of a project requirement | leaf | `requirement` | that requirement's JSON fields |
| identity hash of a module | interior | — | the sorted hashes of that module's children |
| `meta/<module identity hash>` | leaf | `meta` | the bytes of that module's `module.json` |
| identity hash of a module requirement | leaf | `requirement` | that requirement's JSON fields |
| identity hash of an api | leaf | `api` | that api's JSON fields |
| identity hash of a component, data_flow or test_section | leaf | that node's type | the bytes of the file its `content` field names |

A module's children are its own envelope leaf plus the requirement, api, component, data_flow and test_section entries that `module.json` declares — with the one exception described under "Empty content is not a node" below. The root's children are the project envelope leaf, the project requirement leaves, and one interior node per module. The root and the module interiors carry no node type at all; every leaf carries one.

Which leaves exist is the resolved profile's declaration: TreeBuilder creates one leaf per node of each declared type — a profile-declared content-bearing type's nodes hash from their content files, a declared non-content type's from their JSON fields — while the tree shape itself (project root, module interiors, identity-hash keys, the synthetic meta leaves) is fixed and not declarable. Under the default profile the declared types are exactly the five in the table above, so no existing tree changes shape and no existing hash moves.

The only synthetic keys are for the JSON envelope leaves that have no `id` field of their own:

- `meta/project` — for `project.json` itself
- `meta/<module-identity-hash>` — for each `module.json`

These are unambiguous because real identity hashes are pure hex (no `/`).

## Interface

Two operations. The first takes a spec directory and returns the root of the finished tree, or fails with an error naming the file it could not read or parse. Nothing is written and nothing is cached between calls — the tree is rebuilt from the files on disk every time it is asked for.

The second takes a finished tree and returns a map from each module's identity hash to that module's name, read off the module interior nodes. That map is the second argument `spex diff` passes to ImpactClassifier — the thing that lets a report say `merkle` where the diff said a hash — and it is built here and nowhere else: the classifier has exactly one caller, and the map that caller hands it comes straight off the tree this component just returned.

Every digest in that tree, leaf and interior alike, is taken by [[325f48728e04|Hasher]]. TreeBuilder decides what gets hashed and how children are grouped; it never decides how a digest is computed.

Each node in the returned tree carries five things: its key, its content hash, its level (`leaf`, `module` or `project`), its node type where it has one, and the identity hash of the module it belongs to — empty for the root, for the project envelope leaf and for project-level requirement leaves. Interior nodes additionally carry their children. All of those the snapshot persists, so a tree loaded from the snapshot file and a tree just built can be compared field for field. A module interior node carries one more that the snapshot does not: the module's own name, which is what the second operation reads back out. That is why the name map is meaningful only on a freshly built tree — ask a snapshot-loaded tree for it and every name comes back empty.

Because the owning module is carried as an identity hash rather than an integer, any consumer that needs to know which module a leaf belongs to compares it against module-level identity hashes directly, with no lookup table in between.

## Algorithm

1. Read `project.json`. Hash it as the `meta/project` leaf. For each project-level requirement, create a leaf keyed by its identity hash (`id` field) with a deterministic JSON hash of its fields.
2. For each module entry, read its `module.json`. Hash it as the `meta/<module-identity-hash>` leaf.
3. For each module requirement, create a leaf keyed by its identity hash with a deterministic JSON hash of its fields.
4. For each api, create a leaf keyed by its identity hash with a deterministic JSON hash of its fields. An api has no content file — see "API Leaf Hashing" below.
5. For each component, data_flow, and test_section, hash the referenced content file and key the leaf by the node's identity hash. A node whose `content` field is empty is skipped — see "Empty content is not a node" below.
6. Compute the module interior hash from its sorted child hashes.
7. Compute the project root hash from sorted module hashes plus the `meta/project` leaf and the project requirement leaves.

[[97cc591528f9|Steps 6 and 7 compose an interior hash out of nothing but the hashes of the children beneath it]], which is why the walk finishes bottom-up: a module's hash is not knowable until every leaf under it has one.

Content files reach step 5 only by being named in a `content` field. No directory is ever listed, so a markdown file sitting in a module directory that no `module.json` entry references is not hashed, gets no leaf, and never appears in a diff. The opposite case is a hard stop rather than a skip: a `content` field naming a file that is not on disk aborts tree building with an error carrying that path, and `spex diff` exits 1 without printing a report. `spex validate` reports the same missing file as a validation error, which is where it is meant to surface.

## Empty content is not a node

For components, data_flows and test_sections, TreeBuilder skips
any entry whose `content` field is the empty string. The skip is silent: no leaf
is created, nothing is logged, and no error is returned.

The consequence is total, not partial. A skipped node has no leaf, so it has no
hash; with no hash it can never appear in a diff, so plan never sees it, never
emits an op for it, and it never acquires a bead. It is declared in
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

Earlier versions used path-style keys like `module/3/component/2` built from integer module and node IDs. Two problems followed: (1) integer collisions on parallel branches forced manual coordination, and (2) the mapping store used a different format (`<module-name>/<node_type>/<id>`), so the matching stage had to translate between the two via `buildMerkleIndex` and `deriveSpecNodeID`. Both translations are deleted now: the merkle tree and the task journal use the same identity hashes as keys, so plan looks up changed merkle nodes directly in the journal fold with no rewriting.

## Requirement Leaf Hashing

Requirements do not have content files. Their content hash is computed from a deterministic JSON serialization of the requirement's fields. Fields are sorted by key and zero-value/omitted fields are excluded (matching `omitempty` semantics). The per-type field allowlists below — the policy deciding what counts as a semantic change on a JSON-backed leaf — are read from the resolved profile, and the default profile declares exactly these lists, so they are stated here as the default declaration rather than as compiled-in constants.

For module-level requirements, the serialized fields are: `depends_on`, `description`, `id`, `preq_id`, `title`, `type`.

For project-level requirements, the serialized fields are: `depends_on`, `description`, `id`, `priority`, `title`, `type`. A project requirement's `derivation` field is deliberately not among them, and the serialization is an allowlist, so the exclusion asks for no branch: a field the enumeration does not name is simply never serialized. The rationale: graduating a requirement from pending to derived must move no requirement leaf, because the derivation itself is already visible as the added module requirements and components that carry the work. Hashing the field would mint a modification on the parent requirement every time a module is decomposed: noise on a node that did not change in substance.

The exclusion holds the requirement leaf still; it does not hold the whole tree still. The two rules meet on this edit and the difference has to be stated, because the graduation removes a field from `project.json`: the file's bytes change, so the `meta/project` envelope leaf — hashed from those bytes, per the Tree Structure table above — moves, and the root hash moves with it. That is the envelope guard doing its job rather than a leak in the exclusion. `meta/project` exists to notice any textual change to the project envelope, and a field deleted from it is one.

The two rules do not conflict, because the envelope's entry is inert. A `meta` leaf is [[425146f32e96|classified `structural`]], `spex plan` filters structural changes out before any journal lookup runs, and [[6f8284df92a2|change completeness]] collects only meta changes that name a module, which `meta/project` does not. So the graduation produces exactly one diff entry, on `meta/project`, and nothing behind it: the requirement leaf is unchanged, the components implementing that requirement are owed no leaf, no op is emitted and no bead moves. An absorb mark would add nothing here either: marking a node withholds its change from matching so that it yields no op, and a structural change is filtered out of matching already, so there is nothing left for a mark to withhold.

The `id` field is now a 12-character hex string rather than an integer, but it is still serialized as part of the leaf so that a node which is moved between modules (and therefore gets a new identity hash) is detected as a content change too — not just a key change.

This ensures that any change to a requirement's text, type, dependencies, or `preq_id` derivation edge produces a different content hash, making it visible as an individual change in the diff.

## API Leaf Hashing

APIs take the same path. An api has no content file, so its JSON fields are its content: the serialized fields are `description`, `group`, `id`, `name` and `provided_by`, sorted by key with zero-value fields excluded.

The consequence is that the diff sees exactly what the identity carries. Changing an api's `provided_by` set, its description or its group modifies the leaf; renaming the surface changes `name`, which changes the identity hash, which is a removal plus an addition rather than a modification. A change *behind* the name — a new flag, a dropped argument, a reshaped response — moves nothing here, because none of it is part of the declared surface string.

## Call sites

TreeBuilder is composed rather than invoked directly by users, and the callers
reach past the merkle pipeline:

- `spex diff` — builds the current tree once per invocation, hands it to
  DiffEngine for comparison against the snapshot loaded by SnapshotStore.
- `spex ingest` SnapshotSaver — builds the current tree to write out the
  fresh snapshot.
- `spex ingest --mode refresh` — RefreshHandler builds its own tree for the
  structural gate and for the content hashes it writes into the journal's
  change events.
- `spex validate` — the link check builds a tree to collect the leaf keys an
  inline spec link has to resolve against, which is why a module node is not a
  link target and a component with no content file is not one either.
- `spex plan` — the spec graph it loads carries a tree built the same way.

There is no standalone tree-building CLI. The first `spex diff` on a
project being born builds the current tree and compares it against the
empty-tree snapshot `spex init` seeded — everything reports as added, and
the pipeline bootstraps without a separate hash step. There is no
skip-the-load path any more: the snapshot location comes from the
lifecycle pre-flight, the load always happens, and an absent snapshot is
that pre-flight's error, not a fallback.
