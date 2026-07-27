# SchemaLoader

Go package that embeds JSON Schema files into the `spex` binary and exposes them for programmatic use. Also exposes the `IdentityHash` utility function that defines what spec node IDs look like.

## Responsibilities

- Embed `project.schema.json`, `module.schema.json`, and `bead-map.schema.json` using `go:embed`
- Expose schemas as byte slices or readers for the validator and other modules to consume
- Provide `IdentityHash(parts ...string) string` — the canonical node-ID computation used by every module that needs to read or write spec node IDs

## Interface

```go
// ProjectSchema returns the embedded project.schema.json content.
func ProjectSchema() []byte

// ModuleSchema returns the embedded module.schema.json content.
func ModuleSchema() []byte

// BeadMapSchema returns the embedded bead-map.schema.json content.
func BeadMapSchema() []byte

// IdentityHash joins parts with "/" and returns the first 6 bytes of
// SHA256 as a 12-character lowercase hex string. The result is the
// canonical spec node ID for the given identity string.
func IdentityHash(parts ...string) string
```

## Design Rationale

Embedding schemas in the binary eliminates external file dependencies. The `spex` binary is self-contained — it carries the schema definitions it validates against. This supports the deterministic requirement: the same binary version always validates against the same schema.

`IdentityHash` lives in this package because the schema is what *defines* what an ID is — the hex pattern in the JSON Schema and the algorithm that produces strings matching that pattern are two halves of one contract. Co-locating them keeps that contract honest.

Callers divide into two kinds, and the split is deliberate. The validator and the `hash-id` command **derive** IDs: they reconstruct an identity string from a node's position and check it against the stored `id`, so they call `IdentityHash` directly. Every other module **carries** IDs: it reads the 12-character hex string out of the JSON `id` field and treats it as an opaque key — a merkle tree key, a `spec_node_id` in the bead-map, a `spec_node_id` on a changeset op — without ever recomputing it.

That is what makes the tree rename-stable. A module that recomputed an ID from a name or a path would silently re-key a node the moment either changed; a module that carries the stored value cannot. Only one place in the codebase needs to change if the truncation length, hash function, or join separator ever changes, and nothing downstream can drift from it because nothing downstream computes it.

No schema versioning is needed initially. Schema changes are tracked via git commits on the schema files themselves. If multiple schema versions need coexistence in the future, a version parameter can be added.
