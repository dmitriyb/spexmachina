# PreflightChecker

Deterministic preflight checking for `/implement` and `/review` skills.

## Responsibilities

- Resolve a bead ID to its mapping record and spec node
- Walk the spec dependency graph to check readiness
- Report status: ready, blocked, or stale
- Provide structured output for skill consumption

## Interface

```go
type PreflightResult struct {
    Status     string   `json:"status"`      // "ready", "blocked", "stale"
    Record     Record   `json:"record"`      // the resolved mapping record
    Blockers   []Blocker `json:"blockers,omitempty"`
    StaleHash  string   `json:"stale_hash,omitempty"` // current hash if stale
}

type Blocker struct {
    SpecNodeID string `json:"spec_node_id"`
    BeadID     string `json:"bead_id"`
    Reason     string `json:"reason"`
}

func Check(ctx context.Context, store Store, spec SpecGraph, beadID string) (PreflightResult, error)
```

## Readiness Algorithm

1. Look up the mapping record by bead ID
2. Check if the spec hash in the record matches the current content hash → if not, status = "stale"
3. Identify the module containing the spec node
4. Check `requires_module` dependencies — for each required module, verify all its components have closed beads (via mapping records)
5. Check component-level `uses` dependencies — for each used component, verify its bead is closed
6. If any dependency is unresolved or has an open bead → status = "blocked", list blockers
7. Otherwise → status = "ready"

## Dependency Resolution

Dependencies are checked transitively: if module A requires module B, and module B requires module C, then A is blocked if C has open beads. Cycle detection prevents infinite loops — if a cycle is found, return an error rather than hanging.

## Design Rationale

### Why deterministic?

Skills call `spex check` to decide whether to proceed with implementation. The same spec state and mapping state must always produce the same answer — otherwise skill behavior becomes unpredictable.

### Why not check bead status directly?

PreflightChecker reads bead status indirectly through the mapping file, not by calling `br list`. This keeps the check fast (file read vs subprocess) and deterministic (no external state). The mapping file is updated by `spex apply`, which is the only writer.
