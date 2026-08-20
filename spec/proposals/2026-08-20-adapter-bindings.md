# Change Proposal: Adapter bindings

## Context

Decoupling spex from the bead tracker worked. `plan` emits a tool-agnostic changeset, an adapter outside the binary executes it, `ingest` folds the receipts, and [[58ea35f52b86|no runtime subprocesses]] holds throughout — spex never calls a tracker. The contract is right and it is done.

The entry point is wrong. The only working adapter is `scripts/apply-br.sh`: 558 lines of bash targeting `br`, a tracker almost nobody else runs. `BrReferenceAdapter` (`7f2e76cecab3`) is honest about being a reference — [[7c2fea6b1963|reference implementation scope]] says users wanting production adapters should fork and adapt — but "fork and adapt" is 558 lines of shell before a new user sees a single task created. The coupling that remains is not to a tracker. It is to bash competence, and nothing in the spec names it.

The op vocabulary makes this worth fixing, because it is tiny. `plan` emits exactly three: `create`, `close`, `retarget`. The top-level `absorbed` array is not the adapter's at all — [[4277dbd90063|reading the changeset]] says it is ignored entirely and consumed by ingest. Three operations against a tracker is a table, not a program.

There is a second thing to fix while this module is open. Four of its requirement titles knowingly misdescribe the contract, and their own descriptions say so:

- **Read changeset v1** — "The title's version is identity-bearing legacy; three is the contract."
- **Resolve three ref shapes** — "The title's count is identity-bearing legacy; two shapes is the contract."
- **Idempotent create via label** — the probe is "an optional adapter capability … an adapter for a tracker without label support omits the probe entirely".
- **Idempotent close via label** — "Close idempotency keys on the tracker's own status, not on any label … (the title's via-label wording is identity-bearing legacy)."

The reason they were left wrong is sound: a title is the identity, so correcting one is delete-plus-create. But the cost was overestimated in this particular module, and it is at its lowest exactly now, while the module is being opened anyway. Leaving four titles that contradict their own descriptions is a poor thing to hand a new adapter author who is reading the contract for the first time.

This proposal is **independent of every other in the series**. It touches no taxonomy, no project state, no check surface, and can land in any order.

## Proposed change

### A binding is a table, not a program

A binding is a declarative document describing how one tracker expresses the three ops. For each op it carries:

- **The mutation** — a command template or an HTTP request template, with the op's fields interpolated.
- **The extraction** — how to read the resulting task id back out. This is the half that decides whether the format stays declarative, because every tracker answers differently: a line on stdout, a field in a JSON body, a `Location` header. An extraction expression per op is not optional decoration; underspecify it and the binding becomes a program again.
- **The idempotency probe** — a *query* template alongside the mutation, answering "has this op already been applied?". Without it a half-applied run stops being resumable, and resumability is a property the journal design exists to protect. The probe is optional per op in exactly the way [[b8d894dff9b5|the create probe]] is already optional: a binding may declare it absent and accept that a crash between a mutation and its receipt can duplicate work on a blind re-run.
- **Unsupported ops declared explicitly.** A binding that cannot perform an op says so, and the runner refuses the changeset rather than skipping the op. A silently skipped op is work that is never tracked and never noticed — the worst failure this system can have, because it looks like success.

Forward-reference substitution stays **out** of the binding. [[2f0a1f1152a0|the op ID substitution table]] behaves identically for every tracker, so it belongs to the runner.

### Two tiers, and the second one is not a fallback apology

`BindingRunner`, a new component, is the generic runner: it reads a changeset and a binding, resolves refs, executes the table, and writes receipts conforming to [[3486b44f4f64|the receipts contract]]. Like every adapter it lives outside the binary.

`BrReferenceAdapter` is **not** deleted and **not** replaced. It is re-framed as the escape hatch and the worked example: what an adapter looks like when the table genuinely cannot express the tracker — auth refresh, pagination, rate limiting, an output format that resists extraction. [[7c2fea6b1963|reference implementation scope]] is amended to describe two tiers rather than one, because a declarative format that pretends to cover everything is a format that will be fought.

### spex validates the binding and executes nothing

A new api `spex adapter validate` checks a binding document against a schema and reports what it declares — including which ops it refuses. A new `BindingSchema` component in the `schema` module owns that schema, beside the project, module, bead-map and drift schemas that already live there.

This does not weaken [[58ea35f52b86|no runtime subprocesses]] and the point is worth stating plainly: spex reads a document and writes a verdict. The runner is a separate program a person invokes, exactly as `apply-br.sh` is today. The guarantee is untouched, not stretched.

### Where bindings live

`bindings/<tracker>.json` at the repository root. A binding is authored, not derived, so it does not belong in the state directory `2026-08-20-project-lifecycle` reserves for what the tool writes. It is also not spec content — it describes a tracker, not the system being specified — so it does not belong under `spec/`. It is project configuration consumed by the runner, and it sits where a person expects project configuration to sit.

### Which bindings ship

The format, plus **br expressed as a table**, and nothing else. That is the module's own scope requirement talking rather than a preference: spex ships references and contracts, and users fork for production. Shipping GitHub Issues or Jira bindings would be surface with no user yet asking for it, maintained against APIs this project does not track.

A format with one binding is a format nobody has stress-tested, and that is a real objection. It is answered inside the test suite rather than by shipping more trackers: a second, **test-only** binding in testdata expressed as HTTP rather than CLI, exercising extraction from a JSON body and from a `Location` header. It proves the format generalises without claiming support for a tracker nobody has asked about.

### The four titles

Renamed to say what their descriptions already say:

| Now | Becomes |
|---|---|
| Read changeset v1 (`4277dbd90063`) | Read changeset v3 |
| Resolve three ref shapes (`a2645b77b8bc`) | Resolve two ref shapes |
| Idempotent create via label (`b8d894dff9b5`) | Optional create idempotency probe |
| Idempotent close via label (`7bad082a34b6`) | Idempotent close on status |

Each description loses the sentence apologising for its own title.

## Impact expectation

### New nodes

| Module | Node | Type | Bead |
|---|---|---|---|
| adapters | Declare a tracker binding | module requirement | no |
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

The four new adapters requirements derive from [[81f8102ae1b5|apply changes]], the project requirement the module's existing contract requirements already derive from. `Define binding schema` derives from [[d5a8407d38e1|validate spec structure]], alongside the other schema definitions.

`BindingSchema` is covered by joining `describes` on **Schema validation tests** (`96f944302b78`), which already names `ProjectSchema` and `ModuleSchema` and stays a bead.

### Renamed requirements — delete-plus-create

The four renames are cheaper here than the general rule suggests, and the arithmetic is worth showing rather than asserting:

- Requirements are **not** plan-relevant, so neither the removed nor the added node mints a bead of its own.
- A removed node mints a cleanup bead only when it had a **closed task**. These never had tasks, so no cleanup beads.
- Every inbound link is in one leaf: `arch_br_reference_adapter.md` links all four, at four places. Those get rewritten with the new hashes.
- `BrReferenceAdapter`'s `implements` array is rewritten with the new ids.
- The old titles go to the retired-vocabulary sweep, which is what stops the old wording surviving somewhere nobody looked.

### Modified nodes

| Module | Node | Hash | Bead | Why |
|---|---|---|---|---|
| adapters | `BrReferenceAdapter` | `7f2e76cecab3` | yes | re-framed as escape hatch; four links and the implements array rewritten |
| adapters | Adapter flow | `703128d3ebd6` | yes | the flow now has two paths, table-driven and hand-written |
| cli | `RootCommand` | `b6758cdfabc4` | yes | the bounded-surface enumeration gains a constructor |
| adapters | Idempotency tests | `99a021074f54` | no | the probe is now binding-declared |
| adapters | Substitution table tests | `6ba400aaeb0f` | no | substitution is the runner's, not the binding's |
| adapters | Br integration test | `e62fbe481413` | no | the br binding runs through `BindingRunner` |
| cli | Root Command Tests | `476f594a2f5f` | no | one new constructor registered |
| schema | Schema validation tests | `96f944302b78` | yes | gains `BindingSchema` |

Two modified requirements oblige leaves already in that table. [[7c2fea6b1963|Reference implementation scope]] changes to describe two tiers, obliging `BrReferenceAdapter`. [[293b27f73924|Bounded third-party CLI surface]] enumerates the subcommand constructors by count in its own description, so a new subcommand changes it and obliges `RootCommand` — a consequence of adding any api at all, and easy to miss until `spex diff` finds it. The adapters module description changes for the same reason as `7c2fea6b1963`.

### Meta-leaf accounting

`adapters`, `schema` and `cli` all have `module.json` changes. Each also changes or adds a requirement in the same module, so the meta rule is suppressed in all three. `adapters` has exactly one existing component, so even without suppression its exposure would have been one leaf.

### The checks that can actually fail

The HTTP test-only binding is the one that carries the format's generality claim: it asserts a task id extracted from a JSON body and from a `Location` header, against a binding that shares no syntax with the CLI-shaped br binding. If the format only fits shell commands, that test fails.

Beyond it: a binding declaring `retarget` unsupported must make the runner refuse a changeset containing one, with the op named — never skip it; a resumption test asserting a changeset half-applied by the runner completes correctly on re-run through the idempotency probes; and the existing br integration test re-pointed at the binding, which is the parity check that the table expresses everything the 558-line script did.

### Scope

Seven beads under one epic: five component leaves — three new, two modified — one data flow, and one test section. Four requirements are renamed, which shows in the diff as four removals and four additions carrying no beads.

## Retired vocabulary

- `Read changeset v1`
- `Resolve three ref shapes`
- `Idempotent create via label`
- `Idempotent close via label`
