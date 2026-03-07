# Apply command implementation

## Structure

`cmd/spex/apply.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, read impact report from stdin or file
2. For each create action: `BeadCreator.Create(action)`
3. For each close action: `BeadCloser.Close(action)`
4. For each review action: `BeadUpdater.Update(action)`
5. Call `ProposalTagger.Tag(allAffected, proposalRef)`
6. Call `SnapshotSaver.Save(currentTree)`
7. In dry-run mode, print actions without executing
