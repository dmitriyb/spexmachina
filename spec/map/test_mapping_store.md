# Mapping Store Tests

## Setup

- Create a temporary directory with a valid spec structure (project.json + one module)
- Write `spec/.history.jsonl` line by line as each scenario requires: change events
  (`added`/`removed`/`modified` with `eid`, `node`, `name`, `node_type`, `module`, `before`,
  `after`, `git_head`, `proposal`), `registered` events (`eid` of the form `<git_head>:<slug>`,
  `proposal`, `git_head`), and receipt events (`task_created`/`task_closed` with `for` and
  `task_id` — plus the legacy shape carrying `proposal` in place of `for`, for the
  legacy-branch scenarios)
- Construct a MappingStore instance pointing at the temp directory. Read scenarios go through
  its parse/fold surface; append scenarios go through its append primitive — the journal's one
  write path

## Scenarios

### Parse a well-formed journal

- **Input**: a journal with two change events and one `task_created` receipt referencing the first
- **Expected**: the store returns three parsed events in file order; every field round-trips

### Fold yields the latest task-bearing event per node

- **Input**: node X with `added` + `task_created` (task A), then `modified` + `task_created` (task B); node Y with `added` and no receipt
- **Expected**: the fold maps X → task B (not A — lineage, latest wins) and contains no entry for Y (no task-bearing event)

### Lookup by identity hash

- **Input**: the fold above, queried with X's identity hash
- **Expected**: returns X's linkage — current task id, the event that carried it, and the event's name/node_type/module

### Lookup by task id

- **Input**: the fold above, queried with task B's id
- **Expected**: returns the same linkage as the identity-hash lookup — the two keys are interchangeable ways to reach one node

### Removed node retains its biography

- **Input**: node Z with `added`, `task_created`, then `removed`
- **Expected**: the fold marks Z removed; a lookup still returns its name, node_type, module and the removing event's `proposal` and `git_head` — the journal is the only surviving name record for Z

### Registered event folds the epic

- **Input**: a `registered` event (`eid: "cafe1234:2026-08-11-event-keyed-linkage"`, proposal slug, git_head) followed by a `task_created` whose `for` references that eid
- **Expected**: the fold lists the epic task keyed by the slug the registered event carries, sourced from the registered event — the same referencing rule as every other receipt

### Legacy epic receipts fold without a change event

- **Input**: a `task_created` receipt carrying `proposal: "2026-04-18-decouple-spex-from-br"` and no `for`
- **Expected**: the fold's read-only legacy branch lists the epic task keyed by the proposal slug; no change event is required or invented, and nothing is migrated or rewritten

### Append validates and lands atomically

- **Input**: append a batch of two valid lines; then append a batch whose second line violates the journal-line schema
- **Expected**: the first batch lands whole — the file gains exactly the two lines, via write-and-rename; the second batch is refused naming the offending line, and the file is byte-identical to its pre-append state — a refused batch changes nothing

### Deterministic order

- **Input**: the same journal parsed twice
- **Expected**: identical fold output both times; list output ordered by file position — the journal's order is the order

## Edge Cases

### Missing journal file

- MappingStore is constructed where `spec/.history.jsonl` does not exist
- **Expected**: parse returns an empty event list, the fold is empty, and no error is raised — absence is a first-class state, not a failure

### Empty journal file

- A zero-byte `spec/.history.jsonl`
- **Expected**: same as missing — empty fold, no error

### Malformed line

- A journal whose third line is not valid JSON
- **Expected**: the map query surface reports `map: journal line 3: …` and exits 1. The store's parse API distinguishes this error so gating callers can degrade to absent instead of failing — the journal is never load-bearing for the pipeline

### Line that is valid JSON but violates the journal-line schema

- `{"event":"task_created"}` with neither `for` nor `proposal`, or a change event missing `node`
- **Expected**: same surfacing as the malformed line, naming the line number and the violated constraint

### Receipt referencing an unknown event id

- A `task_created` whose `for` names an `eid` no change event carries
- **Expected**: reported by the fold as a dangling receipt; the rest of the journal folds normally — one bad pairing does not poison the file
