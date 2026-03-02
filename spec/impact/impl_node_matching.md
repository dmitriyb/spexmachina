# Node Matching Algorithm

## Approach

Build an index of beads by their spec coordinates, then look up each changed spec node.

## Algorithm

1. Index beads by `(module, component)` and `(module, impl_section)` pairs
2. For each changed spec node:
   - Determine the module from the change path
   - Determine the node type and name from the filename (e.g., `arch_hasher.md` → component "Hasher")
   - Look up matching beads in the index
3. Collect results into matched, unmatched, and orphaned lists

## Name Resolution

Content filenames map to node names via the naming convention:
- `arch_<snake_name>.md` → component with matching name
- `impl_<snake_name>.md` → impl_section with matching name
- `flow_<snake_name>.md` → data_flow with matching name

The exact name is looked up in `module.json` to handle any discrepancies between filename slugs and actual node names.

## Multiple Beads per Node

A single spec node may have multiple beads (e.g., an implementation bead and a review bead). All matching beads are returned.
