# ContextResolver

Given one key — a node's identity hash or a task id — resolves all spec files needed to implement
or review that node, plus the event bracket that locates its latest change in git. That resolution
is what it means to hold [[40a3d3155131|the full spec context of a record]]: for a live node, the
spec graph answers the files and the [[205e67ca4aad|journal fold]] answers the bracket; for a
removed node, the journal carries the biography that outlives the node. No mapping record exists
to be handed in — the key is the whole of the input. No tracker label is consulted or accepted as
a key either: identity hashes and task ids resolve entirely through the journal, so resolution
works unchanged against a tracker that carries no labels at all.

## Responsibilities

- Resolve a task id to its node's identity hash through the journal fold; a 12-hex key skips this
  step and is the hash
- For a live node: locate the declaring module.json, read the component's declared `content` for
  the arch file, and discover test and flow paths by scanning declarations for the hash
- For any node, live or removed: serve the event bracket — `eid`, `event`, `before_head`,
  `after_head` — off the node's latest task-bearing journal event, so a consumer can turn the
  answer into `git diff <before_head> <after_head> -- <leaves>` instead of reconstructing the
  delta by hand
- For a retargeted task, [[76fe608c3a40|widen that bracket to the whole accumulated change]]:
  `before_head` from the sourcing event of the task's original `task_created`, `after_head` from
  its latest `task_retargeted` event, so the implementer sees the full delta the task owes, not
  the last increment
- For a removed node: return the journal's biography — name, node type, module, removing proposal,
  last task — alongside that bracket
- Return all of it as one structured result

## Interface

Resolution takes a spec directory and one key, and produces one result. For a live node the keys a
caller parses are `arch_file`, `test_files`, `flow_files`, `module_file`, `eid`, `event`,
`before_head`, `after_head`; for a removed node they are `name`, `node_type`, `module`,
`proposal`, `task_id`, `eid`, `event`, `before_head`, `after_head`.
[[3c8a43221ed2|`spex map context`]] prints that result, so those keys are the result under another
name — a superset of the pre-bracket shape, with every existing key keeping its meaning. The
removed-node shape is what keeps a cleanup task born with working context instead of a dead end:
its journal pairing references the removal event, so the task id resolves to enough context to
run `git show` on the retired leaf.

## Algorithm

1. If the key is not a 12-hex hash, fold the journal and map the task id to its node's hash; an
   unknown task id is a not-found error
2. Scan every `module.json` named by `project.json` for a component, data flow or test section
   declaring that hash — the module is *discovered*, never stored
3. If found live: the arch file is the component's declared `content` under its module directory;
   scan `test_sections` for `describes` containing the hash and `data_flows` for `uses` containing
   it, prepending `<spec dir>/<module>/` to each `content`; the module file is that module's
   `module.json`. The bracket comes from the journal: `eid` and `event` are the node's latest
   task-bearing event, `after_head` that event's `git_head`, `before_head` the `git_head` of the
   node's preceding change event — null when the latest event is an `added` or the journal holds
   no prior. When that latest event is a `task_retargeted`, the bracket widens instead:
   `before_head` comes from the change event preceding the task's original `task_created` — the
   same value the task was born with — while `after_head` is the retargeted event's `git_head`,
   so consecutive retargets keep extending one bracket rather than each shrinking it to its own
   increment. A live node with no task-bearing event yet serves a null bracket; the file set does
   not depend on the journal
4. If declared nowhere live: consult the journal for the hash's latest `removed` event and return
   the biography with the same bracket shape. The leaf path comes off the event's `path` field;
   `eid` and `event` are the removal event's, `after_head` its `git_head`, and `before_head` the
   `git_head` of the node's latest prior change event, absent when the journal holds none (a
   backfilled node with no recorded prior change).
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
declarations. What derivation cannot recover — the identity of a node that no longer exists, and
the commit refs bracketing any node's latest change — is exactly what the journal keeps, which is
the division of labor: spec for the present state, journal for history, nothing in between to
drift.

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
