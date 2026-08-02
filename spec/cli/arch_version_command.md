# VersionCommand

The subcommand behind [[283080bfd0cd|`spex version`]]. It prints the version string and the build metadata, and does nothing else.

## Responsibilities

VersionCommand is what satisfies [[e15f238e9534|Version command]]: it answers "which binary am I looking at?" without reading the spec directory, the network, or any file.

- Print the version string (e.g., `spex v0.1.0`).
- Print the build metadata: commit hash, build date, Go toolchain version.
- Accept no positional arguments — a stray word after `version` is a clean error, not silently ignored.
- On the success path, write nothing to stderr and exit 0.

## Output Format

```
spex v0.1.0
commit: abc1234
built:  2026-03-10T12:00:00Z
go:     go1.22.1
```

Four lines, one key-value pair per line, human-readable. This is deliberately not machine-parseable output — if structured version info is needed in the future, a `--json` flag can be added rather than the four lines being reshaped, because anything already reading them would break.

## Version Variables

Three of the four reported values are compiled in with a default and overridden at build time through the linker's `-X` flag. The fourth is read from the toolchain and cannot be overridden at all.

| Reported on line | Default when nothing is injected | Injected by |
|------------------|----------------------------------|-------------|
| `spex <version>` | `dev` | `-X main.version=v0.1.0` |
| `commit:` | `unknown` | `-X main.commit=abc1234` |
| `built:` | `unknown` | `-X main.date=2026-03-10T12:00:00Z` |
| `go:` | — | not injectable: the toolchain version that compiled the binary |

A build that injects none of them therefore reports `dev` with `unknown` twice, which is how a development build is told apart from a release without anyone having to be asked.

## Registration

VersionCommand is a child of [[b6758cdfabc4|RootCommand]] — one of the twelve constructors attached in the binary's single registration call — and it inherits the root's persistent flags while reading none of them, so `--spec-dir` changes nothing about its output.
