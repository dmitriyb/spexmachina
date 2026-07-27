# RenderCommand

CLI entry point for `spex render`. Generates human-readable or machine-readable output from the spec.

## Responsibilities

- Parse CLI flags: spec directory, output format (markdown/dot/json), and `--slim`
- Wire SpecReader to parse the spec into an in-memory graph
- Wire the selected renderer (MarkdownRenderer, DOTRenderer, or JSONRenderer)
- Reject `--slim` unless the format is json
- Output to stdout

## Interface

```
spex render [dir] [--format markdown|dot|json] [--slim]
```

## Declared surface

The module declares one api node, `spex render`, with `provided_by` naming RenderCommand. The declared name is the invocation string alone.

The three formats are therefore one surface, not three. That is the substantive claim: `--format dot` is not a separate entry point with its own identity, so changing what a format emits, adding a fourth, or adding `--slim` moves no name and produces no change the diff can attribute to the surface. What the diff does see is a rename of the subcommand — a new identity hash, read as a removal plus an addition, with the removal-time name check reporting every corpus mention still saying `spex render`.

The cost of that choice is the flip side: the flag surface documented above is checked by review, not by the tool. It is the right trade here because a caller composing this command types `spex render` and selects a format; splitting the formats into three api nodes would assert an independence they do not have, since all three read the same graph from SpecReader and all three must carry every node type in it.
