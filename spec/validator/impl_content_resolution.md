# Content Resolution Implementation

## Approach

Walk all nodes with `content` fields in each module.json and check file existence using `os.Stat`.

## Algorithm

1. For each module, parse `module.json` into typed structs
2. Collect all `content` values from components, impl_sections, and data_flows
3. For each content path:
   - Validate it does not contain `..` or start with `/`
   - Construct absolute path: `filepath.Join(specDir, module.Path, content)`
   - Call `os.Stat` on the path
   - If `os.IsNotExist`, record a validation error
4. Return all missing content errors

## Performance

File existence checks via `os.Stat` are fast — no file content is read. Even 1000 checks complete in milliseconds.
