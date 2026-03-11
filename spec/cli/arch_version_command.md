# VersionCommand

The `spex version` subcommand. Prints version string and build metadata.

## Responsibilities

- Print the version string (e.g., `spex v0.1.0`).
- Print build metadata: commit hash, build date, Go version.
- Exit 0 after printing.

## Output Format

```
spex v0.1.0
commit: abc1234
built:  2026-03-10T12:00:00Z
go:     go1.22.1
```

The format is human-readable, one key-value pair per line. This is not machine-parseable output — if structured version info is needed in the future, a `--json` flag can be added.

## Version Variables

Three package-level variables are declared with default values and overridden at build time via `ldflags`:

| Variable | Default | ldflags key |
|----------|---------|-------------|
| `Version` | `dev` | `-X main.version=v0.1.0` |
| `Commit` | `unknown` | `-X main.commit=abc1234` |
| `Date` | `unknown` | `-X main.date=2026-03-10T12:00:00Z` |

The Go version is obtained at runtime via `runtime.Version()`.

## Registration

VersionCommand is a child of RootCommand. It is registered via `rootCmd.AddCommand(cli.NewVersionCmd())` in `main.go`.
