# Version Injection

How version and build metadata are injected at build time.

## ldflags Pattern

Go's `-ldflags -X` linker flag sets string variables at build time without modifying source code. The version variables are declared in the `main` package (or the CLI package) with sensible defaults:

```go
var (
    version = "dev"
    commit  = "unknown"
    date    = "unknown"
)
```

The build command injects values:

```sh
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/spex ./cmd/spex/
```

## Makefile / Build Script Integration

The existing `go build -o bin/ ./cmd/spex/` command in CLAUDE.md should be updated to include ldflags. A `VERSION` file or git tags can be the source of truth for the version string. Example:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  ?= $(shell git rev-parse --short HEAD)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" -o bin/spex ./cmd/spex/
```

## Dev Builds

When built without ldflags (e.g., `go run ./cmd/spex/`), the defaults apply: version is `dev`, commit and date are `unknown`. This clearly distinguishes development builds from releases.

## Runtime Version

The Go version is not injected via ldflags — it is obtained at runtime via `runtime.Version()`, which returns the Go toolchain version used to compile the binary.
