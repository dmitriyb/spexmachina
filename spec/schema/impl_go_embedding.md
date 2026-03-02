# Go Embedding

## Implementation Approach

Use Go's `embed` package to embed JSON Schema files directly into the binary at compile time.

```go
package schema

import "embed"

//go:embed project.schema.json
var projectSchema []byte

//go:embed module.schema.json
var moduleSchema []byte

func ProjectSchema() []byte { return projectSchema }
func ModuleSchema() []byte { return moduleSchema }
```

## Key Decisions

### Byte slices, not parsed objects

Schemas are exposed as raw `[]byte`. Parsing into a schema object is the validator's responsibility. This keeps the schema package free of JSON Schema library dependencies.

### Same package as schema files

The Go code lives in the `schema/` package alongside the `.schema.json` files. This is required by `go:embed` — embedded files must be in the same directory or a subdirectory of the embedding package.

### No caching needed

`go:embed` variables are initialized at program start and are read-only. There is no repeated file I/O to cache.
