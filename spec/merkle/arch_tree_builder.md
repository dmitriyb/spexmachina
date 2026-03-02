# TreeBuilder

Builds the merkle tree by walking the spec directory structure.

## Responsibilities

- Walk the spec directory and map it to a tree structure
- Create leaf nodes for each file (markdown content, JSON)
- Create interior nodes mirroring the spec hierarchy
- Compute hashes bottom-up

## Tree Structure

```
project (root)
├── project.json (leaf)
├── module: schema
│   ├── module.json (leaf)
│   ├── arch (interior)
│   │   ├── arch_project_schema.md (leaf)
│   │   └── arch_module_schema.md (leaf)
│   └── impl (interior)
│       ├── impl_schema_definitions.md (leaf)
│       └── impl_go_embedding.md (leaf)
├── module: validator
│   ├── module.json (leaf)
│   ├── arch (interior)
│   │   └── ...
│   ├── impl (interior)
│   │   └── ...
│   └── flow (interior)
│       └── ...
└── ...
```

## Interface

```go
type Node struct {
    Name     string
    Hash     string
    Type     string   // "leaf", "arch", "impl", "flow", "module", "project"
    Children []*Node
}

func BuildTree(specDir string) (*Node, error)
```

## Algorithm

1. Read `project.json` to discover module paths
2. For each module, read `module.json` to discover content files
3. Hash each leaf file
4. Group leaves by type (arch, impl, flow) and compute interior hashes
5. Compute module hash from its children
6. Compute project root hash from all module hashes + project.json hash
