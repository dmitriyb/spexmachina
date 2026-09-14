# Obligation tests

Acceptance scenarios for ObligationReporter as every writing command drives it: the refusal a command emits is the validator's error, and the obligations it prints are the completeness checker's entries.

## Setup

The fixture of the node editing tests — `tmp/spec/` with module `alpha`, requirement R1, components Comp1 and Comp2, test section T1 and api `demo run` — with a snapshot taken before each scenario so that `spex diff --spec-dir tmp/spec/ --json` afterwards reports the change the command made. The two oracles every scenario is read against:

- **Parity**: the same change applied by hand to a copy of the fixture, then `spex validate` over the copy. A command's refusal must equal the validator's error on the copy — same `check`, same message — and a command that writes must leave a tree the validator accepts to the same degree the hand copy is accepted.
- **Obligations**: `spex diff --json`'s `errors` array after the write must equal, entry for entry, what the command printed under `obligations` on stdout.

## Scenarios

### O1: An added requirement prints the implementation it now owes

**Given** the fixture, where Comp1 implements R1 and nothing implements anything else.
**When** `spex node add` is run with type `requirement`, module `alpha`, name `R2`, type field `functional` and preq_id P1.
**Then** exit 0, and stdout carries an `obligations` array with one entry, the `incomplete_change` for `requirement R2 (…) added but not implemented by any component`; `spex diff --json` afterwards carries the same entry and no other error — the module's `meta` leaf moved too, but a requirement in the module changed, so no whole-module sweep runs.

### O2: A module.json edit without a requirement change obliges every component

**Given** the fixture.
**When** `spex edge add` is run with source Comp2, field `implements`, target R1.
**Then** exit 0, and `obligations` names Comp1's and Comp2's leaves — every component in `alpha` — because the module's `meta` leaf moved and no requirement in the module changed. `spex diff --json` reports the same two entries.

### O3: A requirement change in the same module suppresses the meta obligation

**Given** the fixture after O1, with no snapshot taken between.
**When** `spex edge add` is run with source Comp2, field `implements`, target R2.
**Then** exit 0, and the command's `obligations` names Comp2's leaf only — `requirement R2 (…) added but component Comp2 content leaf unchanged` — and not Comp1's: R2's leaf is still an added requirement in this diff, so the whole-module sweep O2 saw is skipped and only the requirement rules apply. `spex diff --json` agrees entry for entry.

### O4: A refusal is the validator's error with a fix attached

**Given** the fixture.
**When** `spex node add` is run with type `component`, module `alpha`, name `Comp1` — a duplicate.
**Then** non-zero exit, no file changed, and the error document carries the validator's `id` check message for the duplicate and a `fix` naming the existing node's id. The parity copy, with the duplicate entry written by hand, fails `spex validate` with the same `check` and message.

### O5: A change the validator would accept is never refused

**Given** the fixture.
**When** `spex node add` is run with type `requirement`, no module, name `Unfulfilled`, type field `functional`, priority `1` — a project requirement nothing derives from.
**Then** exit 0 and the write lands: the validator's `requirement_coverage` finding on the hand copy is a validation error, not a refusal at write, and it appears under `obligations` as the finding the change left open. The commands add no rule the validator does not hold, and they hold back no write the validator would let stand.

## Edge cases

### O6: Nothing is written when any checker refuses

**Given** the fixture, and a run of `spex node add` that would both add a valid component and — through a second value on the same invocation — an entry the schema rejects.
**When** the command runs.
**Then** non-zero exit and the tree is byte-identical: a refusal on any part of a change leaves no part of it on disk.

### O7: A read command prints no obligations

**Given** the fixture.
**When** `spex profile show` is run.
**Then** stdout is the profile document alone — no `obligations` key — and no file changed.
