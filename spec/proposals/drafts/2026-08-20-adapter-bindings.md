# Change Proposal: Adapter bindings

## Context

Decoupling spex from the tracker worked. `plan` emits a tool-agnostic changeset, an adapter outside the binary executes it, `ingest` folds the receipts, and [[58ea35f52b86|no runtime subprocesses]] holds throughout — spex never calls a tracker. Since `2026-08-31-task-lifecycle` the adapter has two halves, both outside the binary: an export half that derives the task-state artifact `spex plan --tasks` reads, and an apply half that consumes the changeset and writes receipts. The contract is right and it is done.

The entry point is wrong. The only working adapter is a pair of shell scripts targeting `br`, a tracker almost nobody else runs: `scripts/apply-br.sh` at 496 lines and `scripts/export-br.sh` at 66. `BrReferenceAdapter` (`7f2e76cecab3`) is honest about being a reference — [[7c2fea6b1963|reference implementation scope]] says users wanting production adapters should fork and adapt — but "fork and adapt" is over five hundred lines of shell before a new user sees a single task created. The coupling that remains is not to a tracker. It is to bash competence, and nothing in the spec names it.

The vocabulary makes this worth fixing, because it is tiny. The apply half executes exactly three ops: `create`, `close`, `retarget`. The top-level `absorbed` array is not the adapter's at all — [[4277dbd90063|reading the changeset]] says it is ignored entirely and consumed by ingest. The export half answers one query: which tasks are in flight, projected to a task id and a status drawn from `open` and `in_progress`. Three mutations and one query against a tracker is a table, not a program.

There is a second thing to fix while this module is open. Five of its requirement titles knowingly misdescribe the contract, and their own descriptions say so:

- **Read changeset v1** — "The title's version is identity-bearing legacy; four is the contract."
- **Resolve three ref shapes** — "The title's count is identity-bearing legacy; two shapes is the contract."
- **Idempotent create via label** — the probe is "an optional adapter capability … an adapter for a tracker without label support omits the probe entirely".
- **Idempotent close via label** — "Close idempotency keys on the tracker's own status, not on any label … (the title's via-label wording is identity-bearing legacy)."
- **Write receipts v1** — "the document carries version 2 … (the title's version is identity-bearing legacy; two is the contract)."

The reason they were left wrong is sound: a title is the identity, so correcting one is delete-plus-create. But the cost was overestimated in this particular module, and it is at its lowest exactly now, while the module is being opened anyway. Two of the five went stale a second time when the formats moved to v4 and v2, which is the argument against ever putting a version number in a title again. Leaving five titles that contradict their own descriptions is a poor thing to hand a new adapter author who is reading the contract for the first time.

This proposal is independent of the others remaining in the series. It touches no taxonomy, no project state, no check surface, and can land in any order; it is placed after `2026-08-20-ingest-adopt-mode` because adoption is the entry point and a tracker binding is what an adopter needs next.

## Proposed change

### A binding is a table, not a program

A binding is a declarative document describing how one tracker expresses the three ops and the one query. For each op it carries:

- **The mutation** — a command template or an HTTP request template, with the op's fields interpolated.
- **The extraction** — how to read the resulting task id back out. This is the half that decides whether the format stays declarative, because every tracker answers differently: a line on stdout, a field in a JSON body, a `Location` header. An extraction expression per op is not optional decoration; underspecify it and the binding becomes a program again.
- **The idempotency probe** — a *query* template alongside the mutation, answering "has this op already been applied?". Without it a half-applied run stops being resumable, and resumability is a property the journal design exists to protect. The probe is optional per op in exactly the way [[b8d894dff9b5|the create probe]] is already optional: a binding may declare it absent and accept that a crash between a mutation and its receipt can duplicate work on a blind re-run.
- **Unsupported ops declared explicitly.** A binding that cannot perform an op says so, and the runner refuses the changeset rather than skipping the op. A silently skipped op is work that is never tracked and never noticed — the worst failure this system can have, because it looks like success.

For the export half it carries **the listing query** — a command or request template that returns the tracker's tasks — and **the projection**: how to read each entry's id and status, and which tracker statuses map onto `open` and `in_progress`. Everything the projection does not name is excluded, which is what the task-state artifact means: in-flight tasks and nothing else. A binding without a listing query cannot serve `spex plan` at all, so the query is not optional.

Forward-reference substitution stays **out** of the binding. [[2f0a1f1152a0|the op ID substitution table]] behaves identically for every tracker, so it belongs to the runner.

A binding declares its own format version, as every authored format now must under the schema module's format-version contract (`b1baa51bd7a9`): the first shipped format is binding format version 1, and the validator refuses a version it does not know with one message naming the file, its version and the supported range.

### Two tiers, and the second one is not a fallback apology

`BindingRunner`, a new component, is the generic runner with two modes matching the adapter's two halves: it reads a binding and either derives the task-state artifact from the listing query, or reads a changeset, resolves refs, executes the table, and writes receipts conforming to [[3486b44f4f64|the receipts contract]]. Like every adapter it lives outside the binary.

`BrReferenceAdapter` is **not** deleted and **not** replaced. It is re-framed as the escape hatch and the worked example: what an adapter looks like when the table genuinely cannot express the tracker — auth refresh, pagination, rate limiting, an output format that resists extraction. [[7c2fea6b1963|reference implementation scope]] is amended to describe two tiers rather than one, because a declarative format that pretends to cover everything is a format that will be fought.

### spex validates the binding and executes nothing

A new api `spex adapter validate` checks a binding document against a schema and reports what it declares — including which ops it refuses. A new `BindingSchema` component in the `schema` module owns that schema, beside the project, module, journal-line and task-state schemas that already live there and the drift schema `2026-08-20-spex-check` adds.

This does not weaken [[58ea35f52b86|no runtime subprocesses]] and the point is worth stating plainly: spex reads a document and writes a verdict. The runner is a separate program a person invokes, exactly as the br scripts are today. The guarantee is untouched, not stretched.

### Where bindings live

`bindings/<tracker>.json` at the repository root. A binding is authored, not derived, so it does not belong in `.spex/`, the state directory `2026-08-20-project-lifecycle` reserves for what the tool writes. It is also not spec content — it describes a tracker, not the system being specified — so it does not belong under `spec/`. It is project configuration consumed by the runner, and it sits where a person expects project configuration to sit.

### Which bindings ship

The format, plus **br expressed as a table**, and nothing else. That is the module's own scope requirement talking rather than a preference: spex ships references and contracts, and users fork for production. Shipping GitHub Issues or Jira bindings would be surface with no user yet asking for it, maintained against APIs this project does not track.

A format with one binding is a format nobody has stress-tested, and that is a real objection. It is answered inside the test suite rather than by shipping more trackers: a second, **test-only** binding in testdata expressed as HTTP rather than CLI, exercising extraction from a JSON body and from a `Location` header, and a listing projection from a paginated-looking JSON array. It proves the format generalises without claiming support for a tracker nobody has asked about.

### The five titles

Renamed to say what their descriptions already say, and to carry no version number that could go stale again:

| Now | Becomes |
|---|---|
| Read changeset v1 (`4277dbd90063`) | Read the versioned changeset |
| Resolve three ref shapes (`a2645b77b8bc`) | Resolve two ref shapes |
| Idempotent create via label (`b8d894dff9b5`) | Optional create idempotency probe |
| Idempotent close via label (`7bad082a34b6`) | Idempotent close on status |
| Write receipts v1 (`3486b44f4f64`) | Write versioned receipts |

Each description loses the sentence apologising for its own title.

## Impact expectation

### New nodes

| Module | Node | Type | Task |
|---|---|---|---|
| adapters | Declare a tracker binding | module requirement | no |
| adapters | Declare the listing query | module requirement | no |
| adapters | Extract task ids from responses | module requirement | no |
| adapters | Probe idempotency per binding | module requirement | no |
| adapters | Refuse unsupported ops | module requirement | no |
| adapters | `BindingRunner` | component | yes |
| adapters | `BindingValidator` | component | yes |
| adapters | `spex adapter validate` | api | no |
| adapters | Binding runner tests — describes `BindingRunner` | test section | no |
| adapters | Binding validation tests — describes `BindingValidator` | test section | no |
| schema | Define binding schema | module requirement | no |
| schema | `BindingSchema` | component | yes |

The five new adapters requirements derive from [[81f8102ae1b5|apply changes]], the project requirement the module's existing contract requirements already derive from. `Define binding schema` derives from [[d5a8407d38e1|validate spec structure]], alongside the other schema definitions.

`BindingSchema` is covered by joining `describes` on **Schema validation tests** (`96f944302b78`), which already names `ProjectSchema` and `ModuleSchema` and stays a task.

### Renamed requirements — delete-plus-create

The five renames are cheaper here than the general rule suggests, and the arithmetic is worth showing rather than asserting:

- Requirements are **not** plan-relevant, so neither the removed nor the added node mints a task of its own.
- A removed node mints a cleanup task only when it had a **closed task**. These never had tasks, so no cleanup tasks.
- Every inbound link is in one leaf: `arch_br_reference_adapter.md` links all five, at five places. Those get rewritten with the new hashes.
- `BrReferenceAdapter`'s `implements` array is rewritten with the new ids.
- The old titles go to the retired-vocabulary sweep, which is what stops the old wording surviving somewhere nobody looked.

Once `2026-09-14-authoring-commands` has landed, a rename is one command that performs this whole list as a transaction; the arithmetic above is what that command does by hand today.

### Modified nodes

| Module | Node | Hash | Task | Why |
|---|---|---|---|---|
| adapters | `BrReferenceAdapter` | `7f2e76cecab3` | yes | re-framed as escape hatch; five links and the implements array rewritten |
| adapters | Adapter flow | `703128d3ebd6` | yes | the flow now has two paths, table-driven and hand-written, each with an export and an apply half |
| cli | `RootCommand` | `b6758cdfabc4` | yes | the bounded-surface enumeration gains a constructor |
| adapters | Idempotency tests | `99a021074f54` | no | the probe is now binding-declared |
| adapters | Substitution table tests | `6ba400aaeb0f` | no | substitution is the runner's, not the binding's |
| adapters | Br integration test | `e62fbe481413` | no | the sandbox run exercises both the br binding through `BindingRunner` and the scripts, and asserts they agree |
| cli | Root Command Tests | `476f594a2f5f` | no | one new constructor registered |
| schema | Schema validation tests | `96f944302b78` | yes | gains `BindingSchema` |

Three modified requirements oblige leaves already in that table. [[7c2fea6b1963|Reference implementation scope]] changes to describe two tiers, obliging `BrReferenceAdapter`. [[970260050e3e|Integration test against real br]] changes to name both paths, obliging the same leaf. [[293b27f73924|Bounded third-party CLI surface]] enumerates the subcommand constructors by count in its own description, so a new subcommand changes it and obliges `RootCommand` — a consequence of adding any api at all, and easy to miss until `spex diff` finds it. The adapters module description changes for the same reason as `7c2fea6b1963`.

### Meta-leaf accounting

`adapters`, `schema` and `cli` all have `module.json` changes. Each also changes or adds a requirement in the same module, so the meta rule is suppressed in all three. `adapters` has exactly one existing component, so even without suppression its exposure would have been one leaf.

### The checks that can actually fail

The HTTP test-only binding is the one that carries the format's generality claim: it asserts a task id extracted from a JSON body and from a `Location` header, and a task-state artifact projected from a JSON listing, against a binding that shares no syntax with the CLI-shaped br binding. If the format only fits shell commands, that test fails.

Beyond it: a binding declaring `retarget` unsupported must make the runner refuse a changeset containing one, with the op named — never skip it; a binding without a listing query must fail `spex adapter validate`; a resumption test asserting a changeset half-applied by the runner completes correctly on re-run through the idempotency probes; and the br integration test running both paths against one sandbox and asserting identical receipts and an identical task-state artifact, which is the parity check that the table expresses everything the scripts did.

### Scope

Seven tasks under one epic: five component leaves — three new, two modified — one data flow, and one test section. Five requirements are renamed, which shows in the diff as five removals and five additions carrying no tasks.

## Retired vocabulary

- `Read changeset v1`
- `Resolve three ref shapes`
- `Idempotent create via label`
- `Idempotent close via label`
- `Write receipts v1`
