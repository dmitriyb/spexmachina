# MapCommand

CLI entry point for `spex map` subcommands.

## Responsibilities

- Parse CLI arguments and flags
- Wire MappingStore and ContextResolver
- Output structured JSON to stdout
- Set exit codes: 0 for success, 1 for errors

## Subcommands

### spex map get \<record-id\>

Returns a single mapping record as JSON. The record-id argument is the integer record `id` (used in the bead label `spex:<id>`), not the identity hash.

```
$ spex map get 42
{
  "id": 42,
  "spec_node_id": "a1b2c3d4e5f6",
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
  {"id": 1, "spec_node_id": "a1b2c3d4e5f6", ...},
  {"id": 2, "spec_node_id": "0f1e2d3c4b5a", ...}
]
```

Exit code 0. Empty array `[]` if no mappings exist.

### spex map context \<record-id\>

Resolves the full spec context for a component. Reads the mapping record, treats `spec_node_id` as the component's identity hash directly, reads `spec/<module>/module.json`, and returns all spec files relevant to that component.

Algorithm:
1. `map get <id>` → record with `module`, `spec_node_id` (identity hash), `content_file`
2. Read `spec/<module>/module.json`
3. Find `impl_sections` whose `describes` array contains the identity hash → their content paths
4. Find `test_sections` whose `describes` array contains the identity hash → their content paths
5. Find `data_flows` whose `uses` array contains the identity hash → their content paths
6. The arch file is already `content_file` on the record

There is no parse step — the identity hash flows from `spec_node_id` straight into the array containment checks.

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

## Interface

```go
func newMapCmd() *cobra.Command
```

`newMapCmd` builds the `map` parent command and attaches `get`, `list` and `context` as its children. It is unexported and lives in `cmd/spex/map.go` alongside every other subcommand constructor; the root `spex` command picks it up through the single `rootCmd.AddCommand(...)` call in `cmd/spex/main.go`.

Each of the three children declares its own `--map-file` flag, defaulting to `.bead-map.json` relative to the working directory. The store is opened inside `RunE` from that flag, not injected at construction, so building the command tree touches no files and `spex --help` works with no bead-map present. `--spec-dir` is not a store input: `get` and `list` never resolve it, and `context` resolves it only to pass a spec root to `ResolveContext` for the `module.json` read.

## Design Rationale

### Read-only by construction

Every `spex map` subcommand is a read. None of them creates, updates or deletes a record, and none of them contacts a tracker.

This is not a scope decision that could be revisited subcommand by subcommand — it follows from where records come from. A mapping record's id is allocated by emit as an op's idempotency label, applied to a bead by the adapter, and materialised by ingest from the changeset/receipts pair. A record written by `spex map` would have no op behind it, no receipt, and no bead carrying its label, and `Reconciler.AssertInvariants` would reject the store on the next ingest — invariant 1 (every ok create has a record) and invariant 4 (no orphan records) both key off that correspondence.

So MapCommand is the query face of a store whose write path runs through the pipeline. Skills read spec context through it; nothing writes through it.

### JSON-only output

All output is structured JSON for machine consumption. Skills parse this output to get spec context. Human-readable formatting is left to `jq` or similar tools.
