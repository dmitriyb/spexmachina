# ProposalCommands

CLI entry points for proposal management: `spex register`, `spex log`, `spex template`.

## Responsibilities

- `spex register`: parse proposal path, wire Registrar to validate and register
- `spex log`: wire HistoryViewer to query and display proposal history
- `spex template`: wire TemplateProvider to output a project or change proposal template

## Interface

```
spex register <proposal-path>
spex log [--json]
spex template [project|change]
```
