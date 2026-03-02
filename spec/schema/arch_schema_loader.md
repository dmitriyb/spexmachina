# SchemaLoader

Go package that embeds JSON Schema files into the `spex` binary and exposes them for programmatic use.

## Responsibilities

- Embed `project.schema.json` and `module.schema.json` using `go:embed`
- Expose schemas as byte slices or readers for the validator to consume
- Provide a function to retrieve a schema by type (project vs module)

## Interface

```go
// ProjectSchema returns the embedded project.schema.json content.
func ProjectSchema() []byte

// ModuleSchema returns the embedded module.schema.json content.
func ModuleSchema() []byte
```

## Design Rationale

Embedding schemas in the binary eliminates external file dependencies. The `spex` binary is self-contained — it carries the schema definitions it validates against. This supports the deterministic requirement: the same binary version always validates against the same schema.

No schema versioning is needed initially. Schema changes are tracked via git commits on the schema files themselves. If multiple schema versions need coexistence in the future, a version parameter can be added.
