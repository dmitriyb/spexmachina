# Tree Construction Implementation

## Algorithm

Bottom-up construction with ID-based keying:

1. **Read project.json** to get the module list with IDs and paths, and project-level requirements
2. **For each project-level requirement**, create a leaf node:
   - Key: `"project/requirement/<req_id>"`
   - NodeType: `"requirement"`
   - Module: 0
   - Hash: SHA-256 of deterministic JSON serialization
3. **For each module**, read `module.json` and enumerate all nodes with their IDs (requirements, components, impl_sections, data_flows, test_sections)
4. **For each module-level requirement**, create a leaf node:
   - Key: `"module/<module_id>/requirement/<req_id>"`
   - NodeType: `"requirement"`
   - Module: the module ID
   - Hash: SHA-256 of deterministic JSON serialization
5. **Hash each content file**, keying the leaf node by its spec ID:
   - Component 2 in module 3 → key `"module/3/component/2"`, hash the file at its `content` path
   - Impl_section 1 in module 3 → key `"module/3/impl_section/1"`
6. **Hash module.json** as a leaf with key `"module/<id>/meta"`
7. **Compute module hash** from sorted child hashes (all requirements, components, impl_sections, data_flows, test_sections, plus module.json)
8. **Hash project.json** as a leaf with key `"project/meta"`
9. **Compute root hash** from project.json hash + project requirement hashes + sorted module hashes

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

The key is built from the spec graph, not from the filesystem:

```go
func nodeKey(moduleID int, nodeType string, nodeID int) string {
    return fmt.Sprintf("module/%d/%s/%d", moduleID, nodeType, nodeID)
}
```

This decouples the merkle tree from filesystem layout entirely.
