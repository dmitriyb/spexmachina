# Tree Construction Implementation

## Algorithm

Bottom-up construction:

1. **Read project.json** to get the module list and paths
2. **For each module**, read `module.json` and identify content files
3. **Group content files** by type:
   - `arch_*.md` → arch group
   - `impl_*.md` → impl group
   - `flow_*.md` → flow group
4. **Hash leaf files** in each group
5. **Compute group interior hashes** from sorted leaf hashes
6. **Hash module.json** as a leaf
7. **Compute module hash** from module.json hash + group hashes
8. **Hash project.json** as a leaf
9. **Compute root hash** from project.json hash + sorted module hashes

## Content File Discovery

Content files are discovered from `module.json` content fields, not from directory listing. This ensures the tree only contains files referenced by the spec — extraneous files in the directory are ignored.

## Missing Content Files

If a content path in module.json points to a non-existent file, tree building fails with an error. Run `spex validate` first to catch these issues.
