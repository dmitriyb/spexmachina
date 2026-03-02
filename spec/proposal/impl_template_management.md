# Template Management Implementation

## Approach

Templates are embedded as string constants in the Go source. No external template files or template engines.

```go
const projectTemplate = `# Project Proposal: <Project Name>
...
`

const changeTemplate = `# Change Proposal: <Title>
...
`

func Template(templateType string, w io.Writer) error {
    switch templateType {
    case "project":
        _, err := io.WriteString(w, projectTemplate)
        return err
    case "change":
        _, err := io.WriteString(w, changeTemplate)
        return err
    default:
        return fmt.Errorf("proposal: unknown template type: %q", templateType)
    }
}
```

## Design Decision

No template variable substitution. Templates are static content with placeholder text. The user fills in the placeholders manually or with LLM assistance. This keeps the template system trivially simple and deterministic.
