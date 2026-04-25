# Proposal commands implementation

## Structure

`cmd/spex/register.go`, `cmd/spex/log.go`, `cmd/spex/template.go` — each registered as subcommands.

## Flow

### register
1. Parse proposal path, read file
2. Call `Registrar.Register(path)` — validates structure, copies to `spec/proposals/`

### log
1. Read bead tracker JSON from stdin. If stdin is empty, exit non-zero with `"spex log: no bead data on stdin; pipe 'br list --json' or equivalent"`.
2. Parse the input as `[]BeadRecord`, accepting either a bare JSON array or a `{"issues": [...]}` envelope (the same shape impact accepts via `--beads`).
3. If `--proposal <ref>` is set, filter to beads carrying the matching `spec_proposal:<stem>` label (trailing `.md` tolerated).
4. Call `HistoryViewer.ShowHistory(beads)` — groups beads by `spec_proposal:` label and renders.
5. Output history (JSON via `--json`, otherwise human-readable). No subprocess invocation.

### template
1. Parse type argument (project or change)
2. Call `TemplateProvider.Generate(type)` — outputs template to stdout
