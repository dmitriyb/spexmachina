# Map Command Tests

## Setup

- Create a temporary spec directory with project.json, two modules, and a populated
  `spec/.history.jsonl`: change events for several live nodes with `task_created` receipts,
  one removed node with its full biography, one `registered` event with its epic `task_created`
  referencing it, and one legacy epic receipt keyed by proposal slug
- Build the `spex` binary with `go build`

## Scenarios

### spex map get — by identity hash

- **Input**: `spex map get <12-hex identity hash of a live node>`
- **Expected**: JSON output carrying the node's folded linkage — identity hash, current task id, name, node_type, module, and the sourcing event's git_head and proposal. Exit code 0.

### spex map get — by task id

- **Input**: `spex map get <task id>` for the same node
- **Expected**: identical output to the identity-hash form — the two key shapes are distinguished by pattern (12-hex vs anything else), never by a flag.

### spex map get — unknown key

- **Input**: `spex map get deadbeefdead` (no such node in journal or spec)
- **Expected**: error naming the key, exit code 1. The test asserts an error is returned, not its exact message.

### spex map list — folded linkage

- **Input**: `spex map list`
- **Expected**: JSON array of the fold — one entry per node with a task-bearing event, in journal order. Exit code 0.

### spex map context — live node

- **Input**: `spex map context <identity hash of a live component>`
- **Expected**: JSON output with arch_file, test_files, flow_files, module_file — every path derived from the spec, none from the journal — plus the event bracket eid, event, before_head, after_head off the node's latest task-bearing journal event. Existing keys keep their meaning: the output is a superset of the pre-bracket shape. Exit code 0.

### spex map context — live node with no task-bearing event

- **Input**: `spex map context <identity hash of a live component the journal has no task-bearing event for>`
- **Expected**: the file set resolves normally; the bracket fields are null. Exit code 0.

### spex map context — removed node

- **Input**: `spex map context <identity hash of the removed node>`
- **Expected**: JSON output with the node's name, node_type, module, the proposal that removed it, and the bracket off its removed event — eid, event, before_head, after_head — the material an agent needs to run `git diff` for the change and `git show` for the final leaf. Exit code 0.

### Output format consistency

- **Input**: run `spex map get <hash>` and pipe through `jq .`
- **Expected**: valid JSON conforming to the journal-line-derived output schema.

## Edge Cases

### No journal exists

- **Input**: `spex map list` in a spec directory with no `spec/.history.jsonl`
- **Expected**: empty JSON array `[]`, exit code 0. The file is NOT created by read-only commands.

### Integer keys are gone

- **Input**: `spex map get 1`
- **Expected**: treated as a task-id lookup and not found (unless a task literally named `1` exists) — there is no integer record id and no fallback parse. Exit code 1.

### Malformed journal line

- **Input**: `spex map list` over a journal whose third line is invalid JSON
- **Expected**: `map: journal line 3: …` on stderr, exit code 1 — the query surface fails loudly where gating commands would degrade to absent.

### Concurrent CLI invocations

- **Input**: two parallel `spex map get` calls for different keys
- **Expected**: both return correct results; reads take no lock — the journal is append-only and readers never write.
