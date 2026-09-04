# JournalEncoder

Turns a journal event or receipt into its serialized journal line and validates that line against
the journal-line schema before it is written. This is invariant 5 of [[ee28b5d190ae|the mapping
consistency invariants]] — every appended line validates before the write — expressed as a
component rather than as helpers at the bottom of a larger file.

## Contract

- Input: one constructed journal event or task receipt from the batch.
- Output: the line as it will appear in the journal file — one JSON object per line, field order
  the encoder's own business (tests parse, never byte-compare).
- A line that fails schema validation refuses the batch **before** the write path is reached.
  The error names the violated constraint; the on-disk journal is untouched. There is no partial
  append — the validation gate sits between construction and the atomic commit, so a refused
  line refuses the whole run.
- A change event whose `node_type` names a kind the resolved profile does not declare is refused
  the same way, with the error naming the kind. The schema fixes the field's shape and enumerates
  nothing; the encoder holds the membership check against the profile's declared types, so a
  profile-declared kind's event lands and a `meta` leaf, a retired kind or a misspelling never
  does. The check runs on the write path only: a line already in the journal, written under a
  profile that has since dropped a type, still reads back, because the journal is permanent and
  the profile is not.

Validation is against the journal-line schema — the document the schema module's
JournalLineSchema ships as `schema/journal-line.schema.json`, read through the schema package —
the same contract every journal reader parses by. The gate lives with the encoder, not with each
caller, so any pathway that appends — normal-mode reconciliation and refresh-mode absorption
alike — inherits it rather than re-implementing it.

The schema is per-line and knows no lifecycle: it admits a `task_created` whether or not an
earlier pairing for the same node was ever closed, so the encoder passes a completed task's
successor exactly as it passes a first task. What a sequence of lines may mean is the
InvariantChecker's question; the encoder answers only whether each line is well-formed.

## A Boundary Made Visible

Ingest encodes journal lines while the map module owns the journal file and its write primitive:
the encoder produces lines, and the append commits through MappingStore's writer-owner primitive.
Whether line encoding should eventually move to the journal's owning module is a real question
this component makes askable — it is deliberately the whole serialization surface in one place,
so moving it would be moving one component, not hunting helpers.
