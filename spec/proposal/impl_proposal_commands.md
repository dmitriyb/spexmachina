# Proposal commands implementation

## Structure

`cmd/spex/register.go`, `cmd/spex/log.go`, `cmd/spex/template.go` — each registered as subcommands.

## Flow

### register
1. Parse proposal path, read file
2. Call `Registrar.Register(path)` — validates structure, copies to `spec/proposals/`

### log
1. Call `HistoryViewer.History()` — queries proposals and linked bead actions
2. Output history (JSON or human-readable)

### template
1. Parse type argument (project or change)
2. Call `TemplateProvider.Generate(type)` — outputs template to stdout
