# Preflight Checker Tests

## Setup

- Create a temporary spec directory with project.json, two modules (A depends on B), and populated `.bead-map.json`
- Module B has all components implemented (beads closed)
- Module A has one component with an open bead

## Scenarios

### Preflight passes — all dependencies ready

- **Input**: bead_id for module A's component, where module B (dependency) is fully implemented
- **Expected**: Preflight returns status "ready" with the resolved spec node info (module, component, content path)

### Preflight fails — dependency module not implemented

- **Input**: bead_id for module A's component, where module B has open beads (not yet implemented)
- **Expected**: Preflight returns status "blocked" with a list of blocking beads/spec nodes in module B

### Preflight with component-level uses dependency

- **Input**: Component X uses component Y (within the same module). Y's bead is still open.
- **Expected**: Preflight returns status "blocked" listing component Y as the blocker

### Preflight for unknown bead

- **Input**: bead_id that has no mapping record
- **Expected**: Error — "no mapping record for bead <id>"

### Preflight for bead with stale spec hash

- **Input**: bead_id whose mapping record has a spec_hash that doesn't match the current spec content hash
- **Expected**: Preflight returns status "stale" indicating the spec has changed since the bead was created

### Determinism check

- **Input**: Run preflight twice with the same mapping state and spec state
- **Expected**: Both runs produce identical output (same status, same blockers, same order)

## Edge Cases

### Circular module dependencies

- **Input**: Module A requires module B, module B requires module A (invalid spec, but preflight should not hang)
- **Expected**: Error — reports the cycle rather than looping

### No dependencies

- **Input**: bead_id for a component in a module with no `requires_module` and no `uses`
- **Expected**: Preflight returns status "ready" — nothing to block on
