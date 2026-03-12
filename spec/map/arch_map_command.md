# MapCommand

CLI entry point for `spex map` and `spex check` subcommands.

## Responsibilities

- Parse CLI arguments and flags
- Wire MappingStore and PreflightChecker
- Output structured JSON to stdout
- Set exit codes: 0 for success/ready, 1 for errors/blocked

## Subcommands

### spex map get \<record-id\>

Returns a single mapping record as JSON.

```
$ spex map get 42
{
  "id": 42,
  "spec_node_id": "impact/component/3",
  "bead_id": "abc-123",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

Exit code 0 on success, 1 if the record is not found.

### spex map list

Returns all mapping records as a JSON array.

```
$ spex map list
[
  {"id": 1, "spec_node_id": "schema/component/1", ...},
  {"id": 2, "spec_node_id": "schema/component/2", ...}
]
```

Exit code 0. Empty array `[]` if no mappings exist.

### spex map context \<record-id\>

Resolves the full spec context for a component. Reads the mapping record, parses the component ID from `spec_node_id`, reads `spec/<module>/module.json`, and returns all spec files relevant to that component.

Algorithm:
1. `map get <id>` → record with `module`, `spec_node_id`, `content_file`
2. Parse component ID from `spec_node_id` (e.g. `schema/component/1` → component ID 1)
3. Read `spec/<module>/module.json`
4. Find `impl_sections` where `describes` contains component ID → their content paths
5. Find `test_sections` where `describes` contains component ID → their content paths
6. Find `data_flows` where `uses` contains component ID → their content paths
7. The arch file is already `content_file` on the record

```
$ spex map context 42
{
  "record": {"id": 42, "module": "impact", "component": "ActionClassifier", ...},
  "arch_file": "spec/impact/arch_action_classifier.md",
  "impl_files": ["spec/impact/impl_action_classification.md"],
  "test_files": ["spec/impact/test_action_classifier.md"],
  "flow_files": ["spec/impact/flow_impact_pipeline.md"],
  "module_file": "spec/impact/module.json"
}
```

All from the record ID, all deterministic, no duplication. Exit code 0 on success, 1 if record not found or module.json unreadable.

### spex check \<bead-id\>

Runs preflight checking for a bead.

```
$ spex check abc-123
{
  "status": "ready",
  "record": {"id": 42, ...}
}
```

Exit code 0 if ready, 1 if blocked or stale.

## Interface

```go
func NewMapCmd(store Store) *cobra.Command
func NewCheckCmd(store Store, spec SpecGraph) *cobra.Command
```

Both commands are registered on the root `spex` command via the CLI module's subcommand registration framework.

## Design Rationale

### Two top-level commands

`spex map` groups mapping CRUD operations. `spex check` is a separate top-level command because it's the most common entry point for skills — keeping it at the top level makes it easier to call.

### JSON-only output

All output is structured JSON for machine consumption. Skills parse this output to get spec context. Human-readable formatting is left to `jq` or similar tools.
