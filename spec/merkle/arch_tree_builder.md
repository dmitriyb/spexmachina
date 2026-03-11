# TreeBuilder

Builds the merkle tree from the parsed spec graph. Nodes carry the spec ID (module ID, component ID, etc.) as their key, not the file path.

## Responsibilities

- Parse the spec graph from project.json and module.json files
- Create leaf nodes keyed by spec ID for each content file
- Create interior nodes keyed by module ID
- Compute hashes bottom-up

## Tree Structure

Nodes are keyed by spec ID, not file path. This makes the tree rename-stable — moving a file or renaming a directory does not change the tree structure as long as the spec IDs remain the same.

```
project (root, key: "project")
├── project.json (leaf, key: "project/meta")
├── module 1: schema (interior, key: "module/1")
│   ├── module.json (leaf, key: "module/1/meta")
│   ├── component 1 (leaf, key: "module/1/component/1")
│   ├── component 2 (leaf, key: "module/1/component/2")
│   ├── impl_section 1 (leaf, key: "module/1/impl_section/1")
│   └── impl_section 2 (leaf, key: "module/1/impl_section/2")
├── module 2: validator (interior, key: "module/2")
│   ├── module.json (leaf, key: "module/2/meta")
│   └── ...
└── ...
```

## Interface

```go
type Node struct {
    Key      string   // spec ID, e.g. "module/1/component/2"
    Hash     string
    Type     string   // "leaf", "module", "project"
    NodeType string   // "component", "impl_section", "data_flow", "test_section", "meta"
    Module   int      // module ID (0 for project-level nodes)
    Children []*Node
}

func BuildTree(specDir string) (*Node, error)
```

## Algorithm

1. Read `project.json` to discover module IDs and paths
2. For each module, read `module.json` to discover components, impl_sections, data_flows, test_sections
3. Hash each content file, keying the leaf node by its spec ID (e.g., `"module/3/component/2"`)
4. Compute module interior hash from sorted child hashes
5. Compute project root hash from sorted module hashes + project.json hash

## Key Format

The spec ID key follows the pattern `module/<module_id>/<node_type>/<node_id>`:
- `module/1/component/1` — component 1 in module 1
- `module/3/impl_section/2` — impl_section 2 in module 3
- `module/1/meta` — module.json for module 1
- `project/meta` — project.json
