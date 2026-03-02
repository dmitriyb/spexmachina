# ContentResolver

Validates that all `content` paths in module.json files resolve to existing markdown files.

## Responsibilities

- Walk all `content` fields in components, impl_sections, and data_flows
- Resolve each path relative to its module directory
- Report missing files as validation errors

## Interface

```go
func CheckContentPaths(specDir string, project *schema.Project) []ValidationError
```

## Behavior

1. For each module in `project.json`, read its `module.json`
2. For each component, impl_section, and data_flow with a `content` field:
   - Construct the full path: `<specDir>/<module.path>/<content>`
   - Check if the file exists
3. Report each missing file with the module name and node that references it

## Edge Cases

- Empty `content` field is valid (content is optional per schema)
- Content path should not contain `..` or absolute paths — flag these as errors
