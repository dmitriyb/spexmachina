# Impact command implementation

## Structure

`cmd/spex/impact.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, read diff from stdin or file
2. Call `BeadReader.ReadAll(beadCLI)` to get current bead state
3. Call `NodeMatcher.Match(diff, beads)` to correlate changes
4. Call `ActionClassifier.Classify(matches)` to determine actions
5. Call `ReportGenerator.Generate(actions)` to produce JSON report
6. Output report to stdout
