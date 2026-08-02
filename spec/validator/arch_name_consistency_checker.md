# NameConsistencyChecker

Validates that module names are consistent between project.json and module.json — the agreement [[7f53c3f63360|module name consistency]] requires, together with the lowercase convention that comes with it.

## Responsibilities

- Walk the modules in the order project.json lists them and read each one's `module.json`
- Compare project.json `modules[].name` with module.json `name` — must match exactly
- Report against the module's own `module.json`, carrying both spellings in the message
- Tell a difference of case apart from a difference of substance, and name the correction when case is all that differs

## Rules

- project.json module name must equal module.json name (exact string match)
- If the two names differ only by case, report a fixable mismatch and name the project.json spelling as the value module.json should carry
- If they differ by more than case, report a conflict carrying both spellings, and no correction with it
- Module names must be lowercase, matching the directory-name convention. This rule is reached on the names that already agree: when project.json and module.json disagree, the mismatch is the entry and the casing rule is not evaluated for that module
- A module contributes at most one entry per run. A spec that cannot be loaded is the exception to these rules rather than an instance of them: the load failures are returned under this checker's own name, located at `project.json` or at the `module.json` that failed rather than at any module name, and no name is compared
