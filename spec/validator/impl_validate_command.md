# Validate command implementation

## Structure

`cmd/spex/validate.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, resolve spec directory to absolute path
2. Call `schema.LoadEmbedded()` to get project and module schemas
3. Run `SchemaChecker.Check(dir)` — collect errors
4. Run `ContentResolver.Resolve(dir)` — collect errors
5. Run `IDValidator.Validate(dir)` — collect errors
6. Run `DAGChecker.Check(dir)` — collect errors
7. Run `OrphanDetector.Detect(dir)` — collect errors
8. Pass all errors to `ErrorReporter.Format(errors, json)`
9. Write output to stdout, exit with appropriate code
