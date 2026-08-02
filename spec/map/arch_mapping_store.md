# MappingStore

Owns the `.bead-map.json` file — the single source of truth for spec-to-bead correlation. Owning it
is the whole of [[934d627f0e90|storing mapping records]]: one record for each spec node that has
reached the tracker, pinning that node's identity hash to a bead id and to the metadata a later
reader needs to find the node again.

## Responsibilities

- CRUD operations on mapping records
- Ensure uniqueness — one bead to one record, and one spec node to one record. A create naming an
  already-mapped `spec_node_id` is refused; records whose `node_type` is `proposal` are the
  exception
- Provide lookup by record ID, bead ID, or spec node ID — by record id for a caller holding a
  `spex:<id>` label, by bead id for the modify-pair path recovering the record behind an obsoleted
  bead, and by spec node id for emit, resolving a dependency on a spec node to the open bead already
  tracking it
- [[4aee62bd3c15|Check the file against the bead-map schema]] on the way in, before any record is
  handed back or written
- Atomic file writes to prevent corruption

The schema check is neither advisory nor deferred: a `.bead-map.json` that fails it is refused with
an error naming the file, and whichever operation triggered the read never runs. The same check runs
against a whole-store replacement before it is written.

Every write goes to a temporary file in the same directory and is renamed into place, so a crash
part-way through leaves the previous file intact rather than a truncated one. Callers sharing one
opened store serialise their read-modify-write cycles against each other. The lock belongs to the
opened store, not to the file or to the process: a second store opened on the same path is on the
same footing as a second process, where the rename is the whole of the guarantee and the worst case
is one caller's write being lost rather than a corrupt file.

## Record Format

The file is one JSON object: a `next_id` counter and a `records` array, written sorted by record id.

Each mapping record contains:

```json
{
  "id": 42,
  "spec_node_id": "a1b2c3d4e5f6",
  "bead_id": "abc-123",
  "bead_type": "feature",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Auto-incrementing record ID, unique within the mapping file. Used as the bead label `spex:<id>`. Stays integer because it is internal to the bead-map and never referenced from the spec graph. |
| `spec_node_id` | string | Identity hash of the spec node — 12-char lowercase hex, identical to the merkle tree key for the same node. On a record whose `node_type` is `proposal` it holds the proposal reference instead; the bead-map schema requires only a non-empty string here. |
| `bead_id` | string | Bead ID from `br` or `bd` |
| `bead_type` | string | Bead issue type (`epic`, `feature`, or `task`) — determined by spec node type. Carried as a separate field because identity hashes do not embed type information. |
| `node_type` | string | The kind of spec node the record points at: `component`, `data_flow`, `test_section`, or `proposal`. Optional — it is present on records that need to distinguish a proposal epic from a spec-graph node, and the bead-map schema constrains it to that closed set. |
| `module` | string | Module name (human-readable, for context-resolver output and debug) |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Path to the spec content markdown file |
| `spec_hash` | string | Merkle content hash of the spec node at time of mapping |

`spec_node_id` is the bead-map's primary key into the spec graph. It is identical, byte for byte, to the merkle tree key for the same node, so the impact command can look up changed merkle nodes in the bead-map with no translation step.

A record points at a whole node, never at a part of one. The values `node_type` may take are exactly the kinds of node that can own a bead: a section of a component's contract is content belonging to that component's leaf and reaches the tracker inside the component's bead, so it never acquires a record, an identity in this file, or a `spex:<id>` label of its own. That is what makes the mapping store indifferent to how a component's contract is laid out across files — the store keys on the component, and a change to which files carry that contract is a content change to the component's leaf, not a change to the set of records.

An update in place touches `bead_id` and `spec_hash` and nothing else. That is the modify-pair
rebind: the record id survives and the bead behind it changes. Every remaining field is settled when
the record is created, and an update that would leave two records naming the same bead is refused,
exactly as a create naming an already-mapped bead is.

## Interface

Every request the store answers is a request about records — never about beads, and never about the
tracker, which it does not contact. A caller can

- add a record, and is told the id it was given;
- fetch the one record with a given record id, the one with a given bead id, or every record sharing
  a spec node id;
- fetch the proposal-epic record for a proposal reference — the highest-id one whose `bead_status`
  is not `closed` — which is how emit recognises a re-run whose epic bead already exists. That
  exclusion has nothing to act on in a file as things stand: no command writes `bead_status`, the
  committed `.bead-map.json` carries it on no record, and a re-run therefore finds the latest epic
  whatever the tracker says about it;
- update a record, delete a record, or ask for all of them in id order;
- read the record id the next addition would take, **without** spending it, which is how emit
  reserves `spex:<id>` labels while remaining a pure function of its inputs;
- replace the whole store — every record and the counter together — in one write, which is how
  ingest commits a reconciled working copy once its invariants hold.

Two of those reads are exposed on the command line unchanged: [[38ddf587012f|`spex map get`]] hands
back one record and [[394ec2c8d669|`spex map list`]] hands back all of them, each as JSON and each
verbatim. Nothing writes through that surface.

## File Location

The mapping file is `.bead-map.json` in the repository root, not inside the spec directory. Every command that touches it — `diff`, `impact`, `emit`, `ingest` via `--map`, and `map get/list/context` via `--map-file` — defaults to that bare relative path, and `scripts/apply-br.sh` reads the same default through `SPEX_MAPPING_FILE`. How a relative path resolves splits by command: `diff` and `map get/list/context` take it verbatim, against the working directory, while `impact`, `emit` and `ingest` resolve it against the spec directory's parent. The name is never derived from `--spec-dir`: no command appends `.bead-map.json` to the spec directory itself.

That separation is deliberate. `spec/.snapshot.json` belongs to the spec tree; `.bead-map.json` does not. The file is committed to git but is NOT hashed into the merkle tree — it is metadata about the relationship between spec and beads, not spec content, and keeping it outside `--spec-dir` means pointing spex at a different spec directory does not silently point it at a different mapping store.

## Design Rationale

### Why a separate file?

Embedding mapping data in module.json would make spec content depend on bead state, breaking the separation of concerns. The mapping file is maintained by `spex ingest`, which reconciles it from a changeset and the adapter's receipts — never by spec authoring, and never by the tracker directly. Emit only reads the store (record lookup and the `next_id` counter); ingest is the sole writer.

### Why not bead labels?

Bead labels are limited in capacity and format. Complex metadata in labels couples spex to the bead CLI's label format. The mapping file gives spex full control over the data structure.

### Why auto-incrementing record IDs?

The record `id` field is what gets stored in the bead label (`spex:42`). Integer record IDs are compact, predictable, and easy to type, and they are assigned by MappingStore so the caller never has to coordinate. This is distinct from the spec graph's identity hashes: the record `id` is internal to the bead-map and is not referenced from any spec node, so it does not need to be content-addressable. The `spec_node_id` field on the same record holds the identity hash, which is the actual link into the spec graph.

The counter is stored rather than recomputed from the highest id present. Deleting record 5 while the
counter reads 6 still hands out 6 next, so an id is never reused — which matters because ids leave
this file. A `spex:5` label survives on a closed bead, and reissuing 5 would silently point that
label at an unrelated spec node.

### Why a flat, sorted array?

Records sit in one flat array rather than nested per module. Flat means no field is privileged as a
lookup key: the same scan answers a query by record id, bead id or spec node id, and the file's
shape does not have to move when the spec graph's does. Sorting by record id means git sees what
actually happened — an append when a record is added, one changed entry when a record is rebound —
instead of a reshuffle on every write.
