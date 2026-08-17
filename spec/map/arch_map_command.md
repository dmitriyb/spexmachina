# MapCommand

CLI entry point for [[1f1b7fdacbf3|`spex map`]] and its subcommands. Those subcommands are the whole
of [[27c046dde129|querying mappings from the command line]]: three reads over the task journal.

## Responsibilities

- Parse CLI arguments and flags
- Wire [[205e67ca4aad|MappingStore]] and [[6b79188dff4c|ContextResolver]] — the store answers `get`
  and `list` on its own, the resolver answers `context`
- Output structured JSON to stdout
- Set exit codes: 0 for success, 1 for errors

## Subcommands

### spex map get \<key\>

[[38ddf587012f|`spex map get`]] returns one node's folded journal linkage as JSON, on one line. The
key is an identity hash or a task id, distinguished by shape — twelve lowercase hex characters is a
node, anything else is tried as a task id. There is no integer record id and no third key form.
One consequence of hash-wins dispatch: a proposal epic, whose fold key is a slug, is reachable
only through its task id — and a tracker id that happened to be twelve bare hex characters would
be unreachable, which is safe for `br` ids because they always carry a hyphen.

```
$ spex map get a1b2c3d4e5f6
{"node":"a1b2c3d4e5f6","task_id":"spexmachina-abc","name":"ActionClassifier","node_type":"component","module":"impact","git_head":"cafe1234","proposal":"2026-08-02-merge-impact-emit"}
```

Exit code 0 on success, 1 if neither the journal fold nor the spec knows the key.

### spex map list

[[394ec2c8d669|`spex map list`]] returns the whole fold as a JSON array, likewise on one line —
one entry per node with a task-bearing event, in journal order.

```
$ spex map list
[{"node":"a1b2c3d4e5f6","task_id":"spexmachina-abc",...},{"node":"0f1e2d3c4b5a","task_id":"spexmachina-def",...}]
```

Exit code 0. Empty array `[]` if the journal is absent or holds no task-bearing events.

### spex map context \<key\>

[[3c8a43221ed2|`spex map context`]] resolves the full spec context for a node, live or removed.
The same key shapes apply. Alone among the three, its payload is indented rather than printed on a
single line.

A live node's files resolve entirely from the spec: the resolver discovers the declaring module,
reads the component's own `content` for the arch file, and scans `describes`/`uses` arrays for the
test and flow paths. Every answer — live or removed — also carries the event bracket off the
node's latest task-bearing journal event: `eid`, `event`, `before_head`, `after_head`, so a
consumer sees the change (`git diff <before_head> <after_head> -- <leaves>`), not just the state.
A removed node resolves from the journal: name, node type, module, removing proposal, last task,
and the same bracket off its `removed` event — which is what keeps a cleanup task, whose journal
pairing references that removal event, a `git show` away from full context.

```
$ spex map context a1b2c3d4e5f6
{
  "arch_file": "spec/plan/arch_action_classifier.md",
  "test_files": ["spec/plan/test_classification.md"],
  "flow_files": ["spec/plan/flow_plan.md"],
  "module_file": "spec/plan/module.json",
  "eid": "cafe1234:op-7",
  "event": "modified",
  "before_head": "beef5678",
  "after_head": "cafe1234"
}
```

All from one key, all deterministic, nothing read from a cache; existing keys keep their meaning,
so the output is a superset of the pre-bracket shape. Exit code 0 on success, 1 if the
key is unknown to both spec and journal or a module.json is unreadable.

## Interface

`spex map` with no subcommand is a dispatcher and a help page. It opens no file, declares no flag of
its own and reads nothing, so it cannot fail for want of a journal; `get`, `list` and `context` hang
off it and do all the work.

The journal's location is a function of `--spec-dir` alone: each child reads
`<spec-dir>/.history.jsonl` when it runs — not when the command tree is assembled — so `spex --help`
touches no files and an unreadable journal is reported by the child that was asked to use it. The
retired `--map-file` flag went with the file it pointed at; there is nothing left to point elsewhere.

## Declared surfaces

Four api nodes cover this command tree: the parent `spex map` and each of its three children. Every
one names MapCommand in `provided_by`, paired with the worker that answers it — MappingStore for
`spex map get` and `spex map list`, ContextResolver for `spex map context`. The parent names
MapCommand alone, because on its own it dispatches and prints help; it opens nothing.

Declaring the children rather than only the parent is what makes the surface legible and what makes
the parent retirable.

- **Legible.** MapCommand's `uses` edges reach MappingStore and ContextResolver whichever child ran,
  so they cannot say which subcommand needs which. `provided_by` is the only place that pairing is
  recorded.
- **Retirable.** The removal-time name check searches the corpus for a removed api's name,
  longest-match-first, discarding hits that a longer *live* name already covers. `spex map` is a
  prefix of all three children, so most of its corpus occurrences belong to them; with the children
  declared, retiring the bare parent reports only the occurrences that are genuinely bare. Without
  them, every mention of every child would count against it.

A declared name is the string a caller types and nothing more. The api is `spex map get`, not
`spex map get <key>` — an argument placeholder does not survive the corpus tokenization the check
relies on, so a name carrying one could never be matched again.

## Design Rationale

### Read-only by construction

Every `spex map` subcommand is a read. None of them appends an event, and none of them contacts a
tracker.

This is not a scope decision that could be revisited subcommand by subcommand — it follows from
where journal events come from. A change event is born from a changeset op at baselining; a receipt
is born from the adapter's report of what the tracker did — and pairs the task whether or not that
adapter stamped any label on it; a `registered` event is born from a
registration. An event written by `spex map` would
have no op behind it, no receipt, and no lifecycle it opens — a forged line in a file whose
whole value is that every line traces to a pipeline run or a registration. So MapCommand is the
query face of a journal whose every append runs through MappingStore's writer-owner primitive on
behalf of `ingest` or the Registrar; skills read spec context through it, nothing writes
through it.

### A node is the only thing addressable

`get` and `context` address a node — by its identity hash directly, or through a task id that the
fold maps to one; `list` addresses none and returns the fold whole. Neither key is a path and
neither is a node kind, so no `spex map` invocation names a file, a section, or a type of spec
content — which is why the four declared api names are stable against changes in how the spec graph
is laid out. `context` is the one place the spec's own vocabulary shows through, and it shows
through in the payload's key set rather than in the surface a caller types.

That asymmetry bounds the blast radius of a spec-format change: retiring a kind of section changes
what `context` prints and changes nothing about `get`, `list`, the key shapes, or the names
themselves.

### JSON-only output

On a successful `get`, `list` or `context`, structured JSON for machine consumption is the whole of
stdout. Errors and diagnostics go to stderr as plain text, so a skill can parse stdout whole — no
filtering, no line-sniffing — and still see what went wrong when it went wrong. Human-readable
formatting is left to `jq` or similar tools.
