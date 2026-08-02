# ContextResolver

Given one key — a node's identity hash or a task id — resolves all spec files needed to implement
or review that node. That resolution is what it means to hold
[[40a3d3155131|the full spec context of a record]]: for a live node, the spec graph alone answers;
for a removed node, the [[205e67ca4aad|journal fold]] carries the biography that outlives the node.
No mapping record exists to be handed in — the key is the whole of the input.

## Responsibilities

- Resolve a task id to its node's identity hash through the journal fold; a 12-hex key skips this
  step and is the hash
- For a live node: locate the declaring module.json, read the component's declared `content` for
  the arch file, and discover test and flow paths by scanning declarations for the hash
- For a removed node: return the journal's biography — name, node type, module, removing proposal,
  last task, and the `git_head` refs bracketing its final change
- Return all of it as one structured result

## Interface

Resolution takes a spec directory and one key, and produces one result. For a live node the keys a
caller parses are `arch_file`, `test_files`, `flow_files`, `module_file`; for a removed node they
are `name`, `node_type`, `module`, `proposal`, `task_id`, `before_head`, `after_head`.
[[3c8a43221ed2|`spex map context`]] prints that result, so those keys are the result under another
name. The removed-node shape is what makes a `spex:cleanup-<hash>` label a working reference
instead of a dead end: the hash resolves to enough context to run `git show` on the retired leaf.

## Algorithm

1. If the key is not a 12-hex hash, fold the journal and map the task id to its node's hash; an
   unknown task id is a not-found error
2. Scan every `module.json` named by `project.json` for a component, data flow or test section
   declaring that hash — the module is *discovered*, never stored
3. If found live: the arch file is the component's declared `content` under its module directory;
   scan `test_sections` for `describes` containing the hash and `data_flows` for `uses` containing
   it, prepending `<spec dir>/<module>/` to each `content`; the module file is that module's
   `module.json`
4. If declared nowhere live: consult the journal for the hash's latest `removed` event and return
   the biography. The leaf path comes off the event's `path` field; `after_head` is the removing
   event's `git_head`, and `before_head` is the `git_head` of the node's latest prior change
   event, absent when the journal holds none (a backfilled node with no recorded prior change).
   A hash with no journal history either is a not-found error that names the key and says it is
   unknown to both spec and journal

## Error Handling

- A module.json that cannot be read or parsed is an error naming the path that was tried
- A live hash that no test_section describes and no data_flow uses is not an error — the lists come
  back empty, because a component may legitimately have no flows and no tests
- Not-found distinguishes its two cases: unknown everywhere, versus removed-and-remembered (which
  is a successful resolution, not an error)

## Design Notes

### Pure function

Resolution takes a spec directory (which contains the journal), reads files, and returns a result.
No side effects, no state, no tracker contact. This makes it testable and deterministic.

### Why a separate component?

Context resolution is reusable beyond the CLI — skills, review tooling and any future consumer need
the same "give me everything about this node" capability. Keeping it out of MapCommand makes it
callable as a library function.

### Everything is derived; nothing is cached

The retired record-based resolver read `module` and `content_file` off a stored record — and was
measurably wrong in five cases where the cache had gone stale while derivation stayed correct. This
resolver stores nothing: the module is found by scanning for the declaring `module.json`, the arch
path is the component's own `content` field, and the discovered lists are functions of the module's
declarations. The one thing derivation cannot recover — the identity of a node that no longer
exists — is exactly what the journal keeps, which is the division of labor: spec for the present,
journal for the past, nothing in between to drift.

### The result's shape follows the module, not any record

Every path in a live result is a function of which kinds of section `module.json` declares — one
key per kind that can name a component, plus the arch leaf and the module file. Retiring a kind of
section is therefore a change to this component and nothing downstream: one arm of the scan goes
away and one key leaves the result. No stored artifact needs migrating, because nothing stores
resolved paths.

### Resolution reads the spec graph and the journal, not the tracker

A bead can be closed, replaced by a modify-pair, or re-created under a new task id, and
`spex map context` returns the same files — the identity hash survives everything except a rename,
and a rename is a new node by definition. A resolver that consulted tracker state would return
different context depending on when it was asked, and the skills that consume it would lose the
determinism they rely on.
