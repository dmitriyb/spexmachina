# Tree Construction Implementation

## Algorithm

Bottom-up construction with identity-hash keying. Every node's key is the value of its `id` field — the 12-character lowercase hex identity hash already stored in the JSON.

1. **Read project.json** to get the module list (with their identity hashes and paths) and project-level requirements
2. **For each project-level requirement**, create a leaf node:
   - Key: requirement's `id` (identity hash)
   - NodeType: `"requirement"`
   - Module: `""` (empty — project-level)
   - Hash: SHA-256 of deterministic JSON serialization
3. **For each module**, read `module.json` and enumerate all nodes with their identity hashes (requirements, components, impl_sections, data_flows, test_sections)
4. **For each module-level requirement**, create a leaf node:
   - Key: requirement's `id` (identity hash)
   - NodeType: `"requirement"`
   - Module: parent module's identity hash
   - Hash: SHA-256 of deterministic JSON serialization
5. **Hash each content file**, keying the leaf node by the node's identity hash:
   - Component with `id: "abc123def456"` → key `"abc123def456"`, hash the file at its `content` path
   - Impl_section with `id: "789abc012def"` → key `"789abc012def"`
6. **Hash module.json** as a leaf with key `"meta/<module-identity-hash>"`
7. **Compute module hash** from sorted child hashes (all requirements, components, impl_sections, data_flows, test_sections, plus the meta leaf)
8. **Hash project.json** as a leaf with key `"meta/project"`
9. **Compute root hash** from `meta/project` + project requirement hashes + sorted module hashes

The only synthetic keys are `meta/project` and `meta/<module-identity-hash>` for the JSON envelope leaves that have no `id` of their own. Real identity hashes are pure hex with no `/`, so the `meta/` prefix never collides with a real key.

## Requirement Leaf Hashing

Requirements have no content files. Their hash is derived from deterministic JSON serialization:

```go
func hashRequirement(req schema.Requirement) string {
    // Marshal with sorted keys, omitting zero-value fields
    // Fields: depends_on, description, id, preq_id, priority, title, type
    data, _ := json.Marshal(req)
    return HashBytes(data)
}
```

The standard `json.Marshal` with struct tags produces deterministic output because Go marshals struct fields in declaration order. The `omitempty` tags ensure zero-value fields are excluded consistently.

## Content File Discovery

Content files are discovered from `module.json` content fields, not from directory listing. Each content field is resolved relative to the module directory. This ensures the tree only contains files referenced by the spec — extraneous files in the directory are ignored.

## Missing Content Files

If a content path in module.json points to a non-existent file, tree building fails with an error. Run `spex validate` first to catch these issues.

## Key Construction

The key is the spec node's identity hash, read directly from its JSON `id` field:

```go
func nodeKey(node specNode) string {
    return node.ID // already the 12-char hex identity hash
}
```

For the two synthetic envelope leaves, the key is composed:

```go
func projectMetaKey() string                  { return "meta/project" }
func moduleMetaKey(moduleHash string) string  { return "meta/" + moduleHash }
```

There is no path-style composition from module ID and node type. The merkle tree key matches the mapping store `spec_node_id` byte for byte, which is what removes the rekeying layer that previously translated between the two formats. This decouples the tree from filesystem layout *and* from the bead-map's internal representation — both ends of the pipeline already speak the same identity-hash language.
