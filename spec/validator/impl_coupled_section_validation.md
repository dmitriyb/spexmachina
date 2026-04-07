# Coupled Section Validation Implementation

## Algorithm

```
func Check(project, modules) -> []ValidationError:
    if project.Sections is nil or empty:
        return []  // nothing to validate

    errors = []
    seenIDs = {}
    seenNames = {}

    for i, section in project.RawSections:
        // 1. Envelope validation
        id, name, type = extractEnvelope(section)
        if id missing or name missing or type missing:
            errors.append(envelope error at index i)
            continue

        if id in seenIDs:
            errors.append(duplicate ID error)
        seenIDs[id] = true

        if name in seenNames:
            errors.append(duplicate name error)
        seenNames[name] = true

        // 2. Coupled enforcement (only for type == "coupled")
        if type != "coupled":
            continue

        module = findModuleByName(modules, name)
        if module is nil:
            errors.append(no matching module error)
            continue

        schemaPath = specDir + "/" + module.Path + "/section.schema.json"
        if not fileExists(schemaPath):
            errors.append(missing section.schema.json error)
            continue

        // 3. Schema delegation
        sectionSchema = loadAndCompile(schemaPath)
        if sectionSchema fails to compile:
            errors.append(schema compilation error)
            continue

        content = stripEnvelopeFields(section)  // remove id, name, type
        validationErrors = sectionSchema.validate(content)
        for each ve in validationErrors:
            errors.append(content validation error with path details)

    return errors
```

## Raw JSON Access

The standard `schema.Project` struct may not preserve the raw section content (since sections contain freeform fields). Two approaches:

1. **json.RawMessage field**: Add a `RawSections []json.RawMessage` field to the Project struct, populated during initial unmarshal. The typed `Sections` array holds the envelope, while `RawSections` preserves the full JSON for content validation.

2. **Separate parse pass**: Re-read `project.json` as `map[string]interface{}` specifically for section content validation. Slightly wasteful but keeps the main struct clean.

Approach 1 is preferred — it avoids double-parsing and keeps all data available from a single unmarshal.

## Schema Loading

Reuse the same JSON Schema validation infrastructure that SchemaChecker uses (the `santhosh-tekuri/jsonschema` library). The section schema is loaded from disk (not embedded) since it's authored by the module, not by spex core.

```go
compiler := jsonschema.NewCompiler()
sectionSchema, err := compiler.Compile(schemaPath)
if err != nil {
    return ValidationError{...}
}
```

## Envelope Stripping

To validate content separately from the envelope:

```go
func stripEnvelope(raw map[string]interface{}) map[string]interface{} {
    content := make(map[string]interface{})
    for k, v := range raw {
        if k != "id" && k != "name" && k != "type" {
            content[k] = v
        }
    }
    return content
}
```

The content map is what gets validated against the module's `section.schema.json`.

## Error Formatting

Errors follow the existing `ValidationError` structure used by other checkers. For schema delegation errors, the JSON Schema validation path is included (e.g., `sections[0].versioning.scheme: expected string, got number`).
