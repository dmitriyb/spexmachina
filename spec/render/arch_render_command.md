# RenderCommand

CLI entry point for `spex render`. Generates human-readable or machine-readable output from the spec.

## Responsibilities

- Parse CLI flags: spec directory, output format (markdown/dot/json), and `--slim`
- Reject an unknown format, and reject `--slim` on any format but `json`, before a single file is read
- Wire [[7d1150c19724|SpecReader]] to parse the spec into an in-memory graph, once per invocation
- Hand that one graph to the renderer the format names — [[b4b4eba6b551|MarkdownRenderer]], [[45331bdc0bd0|DOTRenderer]] or [[172d16ca0eac|JSONRenderer]]
- Output to stdout

## Interface

```
spex render [dir] [--format markdown|dot|json] [--slim]
```

The spec directory is the positional argument when one is given and the root command's `--spec-dir` otherwise. The format defaults to `markdown`. A rejected flag combination and a spec directory that will not read are the same outcome to a caller: the message goes to stderr, stdout stays empty, and the command exits 1.

## Declared surface

The module declares one api node, [[2c08890a5e56|`spex render`]], with `provided_by` naming RenderCommand. The declared name is the invocation string alone.

The three formats are therefore one surface, not three. That is the substantive claim: [[8828685278e9|Render markdown]], [[a596d8caefb1|Render DOT]] and [[1078c088e0c6|Render JSON]] are three requirements met behind one name, so `--format dot` is not a separate entry point with its own identity, and changing what a format emits, adding a fourth, or adding `--slim` moves no name and produces no change the diff can attribute to the surface. What the diff does see is a rename of the subcommand — a new identity hash, read as a removal plus an addition, with the removal-time name check reporting every corpus mention still saying `spex render`.

The cost of that choice is the flip side: the flag surface documented above is checked by review, not by the tool. [[8d441659a190|Composable output]] is what makes it the right trade here, since a caller composing this command types `spex render` and selects a format; splitting the formats into three api nodes would assert an independence they do not have, as all three read the same graph from SpecReader and all three must carry every node type in it.
