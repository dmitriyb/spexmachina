# Refresh handler implementation

Implementation notes for `RefreshHandler` (component id `f9033352c13f`). The
component lives in `ingest/refresh.go`. CLI flag wiring lives in
`cmd/spex/ingest.go` alongside the existing normal-mode flag handling.

## Top-level entry point

```go
// RefreshHandler.Apply runs the refresh-mode pathway end-to-end. Returns
// a summary on success or a structured error on refusal/IO failure.
func (h *RefreshHandler) Apply(specDir string) (RefreshSummary, error)
```

`Apply` is the only public method. It composes the steps below in order.
The IngestCommand calls `Apply` after parsing flags and confirming
`--mode refresh` was passed with empty changeset+receipts files.

## Step 1: pre-flight

- Confirm `spec/.snapshot.json` exists. Missing → return
  `ErrRefreshRequiresSnapshot`.
- Confirm the changeset and receipts files exist and parse, and that
  both are empty (no ops, complete status). Non-empty → return
  `ErrRefreshNonEmptyArtifacts`.
- Load the current spec directory (project.json + module.json files) via
  the existing schema parser. Validation errors → return as-is (refresh is
  not a place to debug a malformed spec).
- Load the current `.bead-map.json`.

## Step 2: compute the diff

Reuse the merkle module's existing path:

1. Build the current tree via `merkle.BuildTree` (which calls the hash
   primitives for every leaf).
2. Load the pre-refresh snapshot via `merkle.Load`. Note `merkle.Load`
   returns the EmptyTree baseline (not an error) when the file is
   missing, so the missing-snapshot pre-flight in Step 1 must stat the
   path explicitly.
3. Run `merkle.Diff(current, snapshot)`, which returns a flat
   `[]merkle.Change`; each entry's `Type` is `Added`, `Removed`, or
   `Modified`.

These are the same primitives `spex diff` uses — RefreshHandler does
not duplicate this code, it imports it.

## Step 3: gate on diff shape

```go
var added, removed []string
for _, c := range merkle.Diff(current, snapshot) {
    switch c.Type {
    case merkle.Added:
        added = append(added, c.Key)
    case merkle.Removed:
        removed = append(removed, c.Key)
    }
}
if len(added) > 0 {
    return summary, &RefreshRefusal{
        Kind:    "added_entries",
        Entries: added,
        Hint:    "structural change; use the normal pipeline",
    }
}
if len(removed) > 0 {
    return summary, &RefreshRefusal{
        Kind:    "removed_entries",
        Entries: removed,
        Hint:    "structural change; use the normal pipeline",
    }
}
```

`RefreshRefusal` is a typed error so the IngestCommand can map it to a
non-zero exit code and a structured stderr message.

## Step 4: gate on orphan records

```go
specNodeIDs := indexSpecGraph(currentSpec) // map[string]bool
for _, record := range beadMap.Records {
    if !specNodeIDs[record.SpecNodeID] {
        return summary, &RefreshRefusal{
            Kind:    "orphan_record",
            Entries: []string{record.SpecNodeID},
            Hint:    "orphan mapping record; resolve via the normal pipeline",
        }
    }
}
```

Index the spec graph once before the loop. The lookup is O(1) per
record, so the whole orphan check is O(records).

## Step 5: update spec_hash for stale records

For each bead-map record:

1. Look up the spec node by `record.SpecNodeID`.
2. Resolve the content leaf path for that node (component → arch_*.md;
   data_flow → flow_*.md; multi-component test_section → test_*.md).
3. Compute the content hash via the same primitive merkle uses for leaves
   (call it `merkle.HashFile` — extract from `Hasher` if not already
   public).
4. If `record.SpecHash != currentHash`, update `record.SpecHash =
   currentHash` in the in-memory copy. Otherwise no change.

Track which records were updated so the summary can report counts. Do not
mutate `record.BeadID`, `record.Status`, `record.OpID`, or any other field.

Use the tree built in Step 2 as the lookup table: flatten its leaves
into a `map[key]hash` once and read every record's current hash from
that map — it returns the right hash regardless of node kind.
Proposal-epic records (`node_type == "proposal"`) reference the proposal
ref, not a spec node, and are skipped — the same exemption the
Reconciler's orphan invariant applies.

## Step 6: atomic commit

```go
// Both writes must move together.
if err := atomicWriteJSON(beadMapPath, beadMap); err != nil {
    return summary, fmt.Errorf("write bead-map: %w", err)
}
if err := atomicWriteSnapshot(snapshotPath, currentTree); err != nil {
    // Roll back the bead-map write. atomicWriteJSON keeps a backup
    // alongside the temp file for exactly this case.
    rollbackBeadMap(beadMapPath)
    return summary, fmt.Errorf("write snapshot: %w", err)
}
```

Use the same atomic-write helper `SnapshotSaver` and `Reconciler` use. The
rollback path is the only place this differs from those: refresh writes
both files in one logical commit and must roll one back on failure of the
other.

The implementer should consider whether the rollback approach above is
sufficient or whether a two-phase commit / journal helper is warranted.
The decision is mode-local: `Reconciler` doesn't need it (snapshot is
written after the bead-map and is regenerable), but refresh's snapshot is
the diff baseline for the next run, so a half-committed refresh is the
exact failure we are protecting against.

## Step 7: build the summary

```go
type RefreshSummary struct {
    RecordsUpdated   int      `json:"records_updated"`
    RecordsUnchanged int      `json:"records_unchanged"`
    SnapshotSaved    bool     `json:"snapshot_saved"`
    Status           string   `json:"status"` // "complete"
}
```

Print as JSON to stdout. The Status field is always `"complete"` on
success — refresh has no `partial` state. Unsuccessful runs return an
error and never print a summary.

## Implementation choices left to the bead

- Whether to extract a `RefreshableTreeBuilder` interface or call merkle
  directly. The component's tests should not need to mock merkle —
  fixtures with real spec trees are clearer.
- Where the atomic-write helper lives. If `internal/atomic.go` doesn't
  exist yet, create it; if `Reconciler` already has one inline, lift it.
- Whether to reuse `IngestSummary` or define `RefreshSummary` separately.
  Separate types keep the refresh path's stdout shape explicit and make
  consumers (skills) easier to migrate.
