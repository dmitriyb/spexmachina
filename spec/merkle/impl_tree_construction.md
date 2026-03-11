# Tree Construction Implementation

## Algorithm

Bottom-up construction with ID-based keying:

1. **Read project.json** to get the module list with IDs and paths
2. **For each module**, read `module.json` and enumerate all nodes with their IDs (components, impl_sections, data_flows, test_sections)
3. **Hash each content file**, keying the leaf node by its spec ID:
   - Component 2 in module 3 → key `"module/3/component/2"`, hash the file at its `content` path
   - Impl_section 1 in module 3 → key `"module/3/impl_section/1"`
4. **Hash module.json** as a leaf with key `"module/<id>/meta"`
5. **Compute module hash** from sorted child hashes (all components, impl_sections, data_flows, test_sections, plus module.json)
6. **Hash project.json** as a leaf with key `"project/meta"`
7. **Compute root hash** from project.json hash + sorted module hashes

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
