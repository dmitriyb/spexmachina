# NameConsistencyChecker

Validates that module names are consistent between project.json and module.json.

## Responsibilities

- For each module in project.json, read the corresponding module.json
- Compare project.json `modules[].name` with module.json `name` — must match exactly
- Report mismatches with both values and the file paths involved
- Use case-insensitive comparison to detect likely matches and suggest fixes

## Rules

- Module names must be lowercase (matching directory name convention)
- project.json module name must equal module.json name (exact string match)
- If names differ only by case, report as a fixable mismatch with suggested correction
- If names differ entirely, report as an error with both values
