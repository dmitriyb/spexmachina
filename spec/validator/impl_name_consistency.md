# Name consistency check implementation

## Structure

`validator/name_consistency.go` — a checker function called by ValidateCommand.

## Algorithm

1. Read project.json modules list (name + path pairs)
2. For each module, read `<path>/module.json` and extract the name field
3. Compare: `project_name == module_name`
4. If mismatch and `strings.EqualFold(project_name, module_name)`: report case mismatch with fix suggestion
5. If mismatch and not equal ignoring case: report name conflict
6. Return accumulated errors via the standard `[]ValidationError` pattern
