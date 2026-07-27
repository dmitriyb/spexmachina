# Validate command implementation

## Structure

`cmd/spex/validate.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, resolve the spec directory to an absolute path
2. Run the eight checkers in a fixed order, appending each one's entries to a single slice: SchemaChecker, ContentResolver, IDValidator, DAGChecker, NameConsistencyChecker, TestCoverageChecker, RequirementCoverageChecker, CoupledSectionChecker. Each takes the spec directory alone and loads what it needs; none short-circuits on entries an earlier checker produced
3. Detect whether stdout is a terminal
4. Hand the aggregated slice, stdout and that terminal flag to ErrorReporter. It sorts by path, writes the JSON report, and returns the report it wrote
5. Derive the exit code from the returned report's `valid` field — never by re-inspecting the slice — so the process status can never disagree with the JSON just written
