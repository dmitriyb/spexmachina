# Change Proposal: Mapping Layer + CLI Module

## Context

Manual testing of `spex` revealed three architectural gaps that block self-hosting:

1. **Fragile traceability.** Spec metadata lives in bead labels (`spec_module:`, `spec_component:`), coupling spex to `br`'s label format. The `/implement` skill does deterministic preflight checks non-deterministically by parsing these labels. Any label format change in `br` breaks the pipeline.

2. **No rename detection.** Renaming a module directory (e.g. `Apply` → `apply`) produces 2 diff changes (1 remove + 1 add) instead of 1 update. Impact interprets these as close+create pairs when the real intent is a rename. This makes case renames — a routine refactor — destructive to the task graph.

3. **No spec↔bead bridge.** Spec and beads are independent worlds. There is no spex-owned data structure that records which spec node maps to which bead. Without this, rename detection, preflight checks, and mapping queries all require reverse-engineering bead metadata.

4. **No CLI infrastructure.** The CLI is a bare `switch` statement in `main.go` with no `--help` flags, no per-subcommand usage text, no `spex version`. Adding new subcommands (`spex map`, `spex check`) without CLI infrastructure makes the tool harder to use and extend.

## Proposed change

### New Module: Map

Owns the spec↔bead correlation layer. The mapping file is the single source of truth — spec stays pure, beads stay generic, spex owns the bridge.

**Components:**

| Component | Purpose |
|-----------|---------|
| MappingStore | Read/write `spec/.bead-map.json` — the spec↔bead mapping file |
| RenameDetector | Compare removes+adds against mapping to detect renames (consumed by Impact) |
| PreflightChecker | Deterministic preflight for `/implement`: trace bead→node, check deps, report readiness |
| MapCommand | CLI: `spex map` (show/query mapping), `spex check <bead-id>` (preflight) |

**Mapping file** (`spec/.bead-map.json`):
- Committed to git alongside `.snapshot.json`
- NOT hashed into merkle tree (it's metadata, not spec content)
- Maintained by `spex apply` (writes on create/update, removes on close)
- Read by `spex impact` (rename detection) and `spex check` (preflight)

**Changes to existing modules:**

- **Impact**: `node_matcher.go` reads mapping for rename detection; produces `update` instead of `close+create` when a remove+add pair matches a known mapping entry.
- **Apply**: Updates `.bead-map.json` after bead operations (writes entries on create, updates on review, removes on close).
- **`/implement` skill**: Calls `spex check <bead-id>` instead of label-parsing preflight logic.

**Module dependency**: Map depends on Schema (mapping file format). Impact and Apply consume Map.

### New Module: CLI

Cross-cutting CLI infrastructure that doesn't belong to any functional module.

**Components:**

| Component | Purpose |
|-----------|---------|
| HelpFormatter | Per-subcommand usage text, `--help` flag handling |
| VersionCommand | `spex version` — prints version and build info |
| ErrorFormatter | Consistent structured error output across all subcommands |

**Module dependency**: CLI has no functional dependencies. All command modules use it.

## Impact expectation

- **New beads**: Map module (4 components: MappingStore, RenameDetector, PreflightChecker, MapCommand) + CLI module (3 components: HelpFormatter, VersionCommand, ErrorFormatter)
- **Modified beads**: Impact/NodeMatcher (mapping integration for rename detection), Apply/ApplyCommand (mapping file maintenance)
- **Modified skill**: `/implement` (`skills/implement` — replace label-parsing preflight with `spex check`)
