# MapCommand

CLI entry point for [[1f1b7fdacbf3|`spex map`]] and its subcommands. Those subcommands are the whole
of [[27c046dde129|querying mappings from the command line]]: three reads over the bead-map.

## Responsibilities

- Parse CLI arguments and flags
- Wire [[205e67ca4aad|MappingStore]] and [[6b79188dff4c|ContextResolver]] — the store answers `get`
  and `list` on its own, the resolver answers `context`
- Output structured JSON to stdout
- Set exit codes: 0 for success, 1 for errors

## Subcommands

### spex map get \<record-id\>

[[38ddf587012f|`spex map get`]] returns a single mapping record as JSON, on one line. The record-id argument is the integer record `id` (used in the bead label `spex:<id>`), not the identity hash; an argument that is not an integer is refused before the mapping file is opened, with the offending argument named in the error.

```
$ spex map get 42
{"id":42,"spec_node_id":"a1b2c3d4e5f6","bead_id":"abc-123","module":"impact","component":"ActionClassifier","content_file":"spec/impact/arch_action_classifier.md","spec_hash":"e3b0c44..."}
```

Exit code 0 on success, 1 if the record is not found.

### spex map list

[[394ec2c8d669|`spex map list`]] returns all mapping records as a JSON array, likewise on one line.

```
$ spex map list
[{"id":1,"spec_node_id":"a1b2c3d4e5f6",...},{"id":2,"spec_node_id":"0f1e2d3c4b5a",...}]
```

Exit code 0. Empty array `[]` if no mappings exist.

### spex map context \<record-id\>

[[3c8a43221ed2|`spex map context`]] resolves the full spec context for a component. It reads the mapping record, treats `spec_node_id` as the component's identity hash directly, reads `spec/<module>/module.json`, and returns all spec files relevant to that component. Alone among the three, its payload is indented rather than printed on a single line.

Algorithm:
1. `map get <id>` → record with `module`, `spec_node_id` (identity hash), `content_file`
2. Read `spec/<module>/module.json`
3. Find `test_sections` whose `describes` array contains the identity hash → their content paths
4. Find `data_flows` whose `uses` array contains the identity hash → their content paths
5. The arch file is already `content_file` on the record

There is no parse step — the identity hash flows from `spec_node_id` straight into the array containment checks.

```
$ spex map context 42
{
  "record": {"id": 42, "module": "impact", "component": "ActionClassifier", ...},
  "arch_file": "spec/impact/arch_action_classifier.md",
  "test_files": ["spec/impact/test_action_classifier.md"],
  "flow_files": ["spec/impact/flow_impact_pipeline.md"],
  "module_file": "spec/impact/module.json"
}
```

All from the record ID, all deterministic, no duplication. Exit code 0 on success, 1 if record not found or module.json unreadable.

## Interface

`spex map` with no subcommand is a dispatcher and a help page. It opens no file, declares no flag of its own and reads no record, so it cannot fail for want of a bead-map; `get`, `list` and `context` hang off it and do all the work.

Each of the three children declares its own `--map-file` flag, defaulting to `.bead-map.json` relative to the working directory. The store is opened when a child runs, from that child's flag — not when the command tree is assembled — so `spex --help` touches no files, works with no bead-map present, and a bad `--map-file` is reported by the child that was asked to use it rather than at startup. `--spec-dir` is not a store input: `get` and `list` never resolve it, and `context` resolves it only to hand a spec root to the resolver for the `module.json` read.

## Declared surfaces

Four api nodes cover this command tree: the parent `spex map` and each of its three children. Every one names MapCommand in `provided_by`, paired with the worker that answers it — MappingStore for `spex map get` and `spex map list`, ContextResolver for `spex map context`. The parent names MapCommand alone, because on its own it dispatches and prints help; it opens no store.

Declaring the children rather than only the parent is what makes the surface legible and what makes the parent retirable.

- **Legible.** MapCommand's `uses` edges reach MappingStore and ContextResolver whichever child ran, so they cannot say which subcommand needs which. `provided_by` is the only place that pairing is recorded.
- **Retirable.** The removal-time name check searches the corpus for a removed api's name, longest-match-first, discarding hits that a longer *live* name already covers. `spex map` is a prefix of all three children, so most of its corpus occurrences belong to them; with the children declared, retiring the bare parent reports only the occurrences that are genuinely bare. Without them, every mention of every child would count against it.

A declared name is the string a caller types and nothing more. The api is `spex map get`, not `spex map get <record-id>` — an argument placeholder does not survive the corpus tokenization the check relies on, so a name carrying one could never be matched again. The `--map-file` flag sits behind the names for the same reason.

## Design Rationale

### Read-only by construction

Every `spex map` subcommand is a read. None of them creates, updates or deletes a record, and none of them contacts a tracker.

This is not a scope decision that could be revisited subcommand by subcommand — it follows from where records come from. A mapping record's id is allocated by emit as an op's idempotency label, applied to a bead by the adapter, and materialised by ingest from the changeset/receipts pair. A record written by `spex map` would have no op behind it, no receipt, and no bead carrying its label, and `Reconciler.AssertInvariants` would reject the store on the next ingest — invariant 1 (every ok create has a record) and invariant 4 (no orphan records) both key off that correspondence.

So MapCommand is the query face of a store whose write path runs through the pipeline. Skills read spec context through it; nothing writes through it.

### A record is the only thing addressable

`get` and `context` address a record by its integer `id`; `list` addresses none and returns them all. A record in turn addresses a spec node by identity hash. Neither the record `id` nor the identity hash is a path and neither is a node kind, so no `spex map` invocation names a file, a section, or a type of spec content — which is why the four declared api names are stable against changes in how the spec graph is laid out. `get` and `list` hand records back verbatim; `context` is the one place the spec's own vocabulary shows through, and it shows through in the payload's key set rather than in the surface a caller types.

That asymmetry is worth stating because it bounds the blast radius of a spec-format change. Retiring a kind of section changes what `context` prints and changes nothing about `get`, `list`, the record-id argument, or the names themselves.

### JSON-only output

On a successful `get`, `list` or `context`, structured JSON for machine consumption is the whole of stdout. Errors and diagnostics go to stderr as plain text, so a skill can parse stdout whole — no filtering, no line-sniffing — and still see what went wrong when it went wrong. Skills parse this output to get spec context. Human-readable formatting is left to `jq` or similar tools.
