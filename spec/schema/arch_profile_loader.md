# ProfileLoader

ProfileLoader owns the resolution of the project's spec profile: the declarative document that turns the node-type vocabulary from compiled-in policy into data. It reads `spec/profile.json` when the file exists, falls back to the built-in default profile otherwise, validates the document, and exposes the resolved profile to every consumer — the schema composition, the validator's checkers, the merkle tree builder, the plan classifier, refresh, the hash-id command and the renderers all read their type-level policy from this one resolution.

## Responsibilities

- **Resolve** the profile per run: `spec/profile.json` beside `project.json` when present — the file sits in `spec/` because it is authored by a person, not written by the tool — or the built-in default otherwise. Absence of the file is the supported default, never an error, which is what [[5392cca550c4|Declare the node taxonomy]] requires: an existing project adopts the mechanism by doing nothing.
- **Validate** the document before anything consumes it. A malformed profile — unparseable JSON, a node type without a plural array key, an edge naming an undeclared type — is one early, distinct failure naming the file and the defect. Resolution happens once per run before any check, so a broken profile never surfaces downstream as a cascade of schema-conformance errors, which is the early-failure half of [[ef55248cb3ca|Generate schemas from the profile]]. Every spex command resolves the profile before any other work, so a malformed `spec/profile.json` produces the same single early error on stderr and exit 1 — from validate, diff, plan, render and hash-id alike — and no other operation runs.
- **Expose** the resolved profile: the node types (name, plural array key, project- or module-scoped, content-bearing or not, and the two per-type role flags — completeness trigger, name-declarable), the legal edges with their optional `cyclic` exemption, and the graph rules of [[91270a8a2b57|Declare graph rules]] — coverage chains, the plan-relevant set, the per-type impact-level mapping, per-type hashed field allowlists, and refresh's per-type, per-direction absorbable set.

## The default profile is the golden policy record

The built-in default declares today's ontology exactly: the five node types with today's role flags, today's edge set with the `cyclic` flag omitted on every kind, the three coverage links, today's plan-relevant set, impact levels, allowlists and absorbable directions. It is the single document recording the policy previously spread across seven modules — a reviewer reads it instead of grepping the code — and it is the fixture the golden tests compare against. Each golden comparison uses the equality its artifact warrants: hashes and reports match byte-identically, while the composed schemas match the shipped documents as JSON values, since the shipped copies are hand-formatted and nothing consumes them as bytes. Under the default profile, every identity hash in this repository's own spec is unchanged, `spex validate` emits a byte-identical report, and `spex diff` against the current snapshot reports no changes.

## Fixed points

The profile describes a vocabulary; it carries no behaviour and cannot reach:

- the identity hash algorithm — already generic over its parts, the type name is just a string segment, which is exactly why the vocabulary can become data without any existing hash moving;
- the name tokenization rule — relaxing it per project would make removals unsweepable;
- the merkle tree shape — the profile says what leaves exist, never how the tree is built over them;
- the journal event format — append-only and permanent, not per-project.

A profile document attempting to declare any of these fails validation.

## Observability

The resolved profile has no dedicated command. It is observable through `spex render` — the declared types reach the node table — and the reproduce-current-behaviour criterion is enforced by golden tests, not by a surface a user must run.
