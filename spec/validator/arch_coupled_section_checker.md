# CoupledSectionChecker

Validates the `sections` array in project.json generically. This checker is not specific to any particular section type (e.g., delivery) — it handles all section types based on the `type` field.

## Responsibilities

- Validate envelope fields on every section entry (id, name, type present and correctly typed)
- Check section ID uniqueness within the sections array
- Check section name uniqueness (no two sections can share a name)
- For sections with `type: "coupled"`:
  - Verify a module with matching `name` exists in the project's modules array
  - Verify `section.schema.json` exists at `spec/<module-path>/section.schema.json`
  - Load and compile the section schema
  - Strip envelope fields (id, name, type) from the section entry, then validate the remaining content against the module's section schema
- For sections with other types: validate envelope only (no module coupling enforced)

## Interface

```go
type CoupledSectionChecker struct {
    specDir string
}

func NewCoupledSectionChecker(specDir string) *CoupledSectionChecker

func (c *CoupledSectionChecker) Check(project *schema.Project, modules []schema.ModuleDecl) []ValidationError
```

## Integration with Validation Pipeline

CoupledSectionChecker plugs into the existing validation pipeline alongside SchemaChecker, DAGChecker, etc. ValidateCommand wires it in and its errors flow through ErrorReporter.

The checker depends on SchemaChecker (component 1) having run first — it assumes the project JSON is structurally valid per the project schema. However, it does its own content validation using the module-provided `section.schema.json`, which is a separate schema from the project schema.

## Content Extraction

To validate section content against the module's schema, the checker must separate envelope fields from freeform content. Approach:

1. Re-read the raw `project.json` as `map[string]interface{}` (or use `json.RawMessage` during initial parse)
2. For each section in the raw sections array, copy all fields except `id`, `name`, `type` into a content map
3. Validate the content map against the loaded `section.schema.json`

This avoids coupling the checker to any specific section content structure.

## Error Messages

All errors include:
- The section name and ID for context
- The specific check that failed (envelope, coupling, schema)
- Actionable guidance (e.g., "add a module with name 'delivery'" or "create spec/delivery/section.schema.json")
