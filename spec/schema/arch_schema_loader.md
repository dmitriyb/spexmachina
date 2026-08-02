# SchemaLoader

The `spex` binary's own copy of the three JSON Schema documents, and the one function that says what a spec node ID is. It opens no file at runtime: every answer it gives was compiled in.

## Responsibilities

- Carry [[79946d618829|ProjectSchema]], [[78883b84c32d|ModuleSchema]] and [[d125b5e775b4|BeadMapSchema]] inside the binary — [[b7c3bccd7c64|the schemas travel with the executable]], so validation needs no file beside it and no network.
- Hand each document back exactly as it was committed under `schema/`, unparsed. Compiling one is the caller's work — the validator for the project and module schemas, the mapping store for the journal-line schema — and leaving it there is what keeps this side of the boundary free of a JSON Schema library.
- Compute identity hashes for the callers that derive an ID rather than carry one.

## Interface

Three reads, one per embedded document. Each hands back that file's bytes as committed, and hands back the same bytes on every call, in every process and on every platform: the content sits in the binary's read-only data, so a read is a copy out of memory rather than a file open, there is no cache to warm, and no read leaves state behind for the next one. Concurrent reads need no coordination for the same reason.

The fourth entry point is behaviour rather than data. [[cdc9c58ba097|Identity hash algorithm]] is stated as an algorithm, so it is stated here as one:

```
identity_hash(parts):
    identity = join(parts, "/")
    digest   = SHA-256(identity)
    return lowercase_hex(first 6 bytes of digest)
```

Same parts in the same order, same 12-character lowercase hex string — on any platform, in any process. That is what makes the result safe to write into a JSON file and commit to git.

The parts are the node's position in the spec graph, taken from fields the node already carries, so anyone holding the spec source can recompute any ID by hand. `spex hash-id` does exactly that: one `--type`, one `--name`, a `--module` for everything but a project requirement or a module, and one hash on stdout.

| Node type | Parts, in order | Identity string |
|---|---|---|
| Project requirement | `project`, `requirement`, title | `project/requirement/<title>` |
| Module | `module`, name | `module/<name>` |
| Module requirement | module, `requirement`, title | `<module>/requirement/<title>` |
| Component | module, `component`, name | `<module>/component/<name>` |
| Data flow | module, `data_flow`, name | `<module>/data_flow/<name>` |
| Test section | module, `test_section`, name | `<module>/test_section/<name>` |
| API | module, `api`, name | `<module>/api/<name>` |

Nothing positional goes in: no array index, no sibling order, no body text. Reordering nodes within an array and editing a description therefore leave every ID exactly where it was, while renaming a node changes its ID — that last one is deliberate, and it is why a rename reaches `spex diff` as one node removed and another added rather than as an edit. `spex validate` re-derives the id of every module-scoped node and names any node whose declared id is not its own hash, together with the hash it should carry. Project-level requirement ids are exempt: some predate the convention, so they are carried as written and never recomputed. The module ids in `project.json` are left alone too, for a different reason — they do all derive today, but the check does not reach them, so a spec carrying hand-written module ids still passes.

## Design Rationale

Embedding schemas in the binary eliminates external file dependencies. The `spex` binary is self-contained — it carries the schema definitions it validates against. This supports the deterministic requirement: the same binary version always validates against the same schema.

`IdentityHash` lives in this package because the schema is what *defines* what an ID is — the hex pattern in the JSON Schema and the algorithm that produces strings matching that pattern are two halves of one contract. Co-locating them keeps that contract honest.

IDs are hashes rather than counters because a counter is order-dependent: two branches that each append a node both take the next integer, and the merge quietly points two different nodes at one key. Two branches that add different nodes write different identity strings and so get different hashes, and two branches that add the same node to the same module get the same hash — which is the right answer, since they are describing one thing. Nothing has to be coordinated between branches for that to hold.

Callers divide into two kinds, and the split is deliberate. The validator and the `hash-id` command **derive** IDs: they reconstruct an identity string from a node's position and check it against the stored `id`, so they call `IdentityHash` directly. Every other module **carries** IDs: it reads the 12-character hex string out of the JSON `id` field and treats it as an opaque key — a merkle tree key, a `node` on a journal event, a `spec_node_id` on a changeset op — without ever recomputing it.

That is what makes the tree rename-stable. A module that recomputed an ID from a name or a path would silently re-key a node the moment either changed; a module that carries the stored value cannot. Only one place in the codebase needs to change if the truncation length, hash function, or join separator ever changes, and nothing downstream can drift from it because nothing downstream computes it.

No schema versioning is needed initially. Schema changes are tracked via git commits on the schema files themselves. If multiple schema versions need coexistence in the future, a version parameter can be added.
