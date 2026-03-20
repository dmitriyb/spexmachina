# Apply command implementation

## Structure

`cmd/spex/apply.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, read impact report from stdin or file
2. For each obsolete action: `BeadCloser.Close(action)` with `spex:obsolete` + `commit:<HEAD>` labels
3. For each create action (in hierarchy order: epics → features → tasks): `BeadCreator.Create(action)` with type, parent, deps, priority
4. Call `ProposalTagger.Tag(allAffected, proposalRef)`
5. Call `SnapshotSaver.Save(currentTree)`
6. In dry-run mode, print actions without executing

## Creation Ordering

Creates are sorted by spec node type before execution:
1. Module beads (epics) — no parent dependency
2. Component beads (features) — parent is the module's epic (must exist)
3. Test_section beads (tasks) — parent is the component's feature (must exist)

This ensures `--parent` references are resolvable from the mapping file.
