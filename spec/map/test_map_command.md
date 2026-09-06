# Map Command Tests

## Setup

- Create a temporary spec directory with project.json, two modules, and a populated
  journal at the fixture project's resolved location: change events for several live nodes with `task_created` receipts,
  one removed node with its full biography, one `registered` event with its epic `task_created`
  referencing it, and one legacy epic receipt keyed by proposal slug
- Build the `spex` binary with `go build`

## Scenarios

### spex map get — by identity hash

**Given** Setup's fixture spec and populated journal, and `spex map get <12-hex identity hash of a live node>`.

- **Expected**: JSON output carrying the node's folded linkage — identity hash, current task id, name, node_type, module, and the sourcing event's git_head and proposal. Exit code 0.

### spex map get — by task id

**Given** Setup's fixture spec and populated journal, and `spex map get <task id>` for the same node.

- **Expected**: identical output to the identity-hash form — the two key shapes are distinguished by pattern (12-hex vs anything else), never by a flag.

### spex map get — unknown key

**Given** Setup's fixture spec and populated journal, and `spex map get deadbeefdead` — no such node in journal or spec.

- **Expected**: error naming the key, exit code 1. The test asserts an error is returned, not its exact message.

### spex map list — folded linkage

**Given** Setup's fixture spec and populated journal, and `spex map list`.

- **Expected**: JSON array of the fold — one entry per node with a task-bearing event, in journal order. Exit code 0.

### spex map context — live node

**Given** Setup's fixture spec and populated journal, and `spex map context <identity hash of a live component>`.

- **Expected**: JSON output with arch_file, test_files, flow_files, module_file — every path derived from the spec, none from the journal — plus the event bracket eid, event, before_head, after_head off the node's latest task-bearing journal event. Existing keys keep their meaning: the output is a superset of the pre-bracket shape. Exit code 0.

### spex map context — live node with no task-bearing event

**Given** Setup's fixture spec and populated journal, and `spex map context <identity hash of a live component the journal has no task-bearing event for>`.

- **Expected**: the file set resolves normally; the bracket fields are null. Exit code 0.

### spex map context — removed node

**Given** Setup's fixture spec and populated journal, and `spex map context <identity hash of the removed node>`.

- **Expected**: JSON output with the node's name, node_type, module, the proposal that removed it, and the bracket off its removed event — eid, event, before_head, after_head — the material an agent needs to run `git diff` for the change and `git show` for the final leaf. Exit code 0.

### Output format consistency

**Given** Setup's fixture spec and populated journal, and `spex map get <hash>` piped through `jq .`.

- **Expected**: valid JSON conforming to the journal-line-derived output schema.

## Edge Cases

### No journal exists

**Given** a directory whose resolved journal file is absent, and `spex map list` run in it.

- **Expected**: the lifecycle pre-flight refuses before the store is consulted — a state directory missing its journal is a broken project and the error names `spex doctor`; a directory with no project state at all errors naming `spex init` with the not-a-spex-project exit code. In no case is any file created by a read-only command. The old empty-array answer survives only at the MappingStore library layer, which still folds an absent file to empty.

### Integer keys are gone

**Given** Setup's fixture spec and populated journal, and `spex map get 1`.

- **Expected**: treated as a task-id lookup and not found (unless a task literally named `1` exists) — there is no integer record id and no fallback parse. Exit code 1.

### Malformed journal line

**Given** Setup's fixture spec and a journal whose third line is invalid JSON, and `spex map list` over it.

- **Expected**: the lifecycle pre-flight refuses the broken project — stderr carries `map:` and names the offending line and `spex doctor`, with the not-a-spex-project exit code. The journal is schema-checked before the store is consulted, so no query surface ever folds over a malformed file.

### Concurrent CLI invocations

**Given** Setup's fixture spec and populated journal, and two parallel `spex map get` calls for different keys.

- **Expected**: both return correct results; reads take no lock — the journal is append-only and readers never write.
