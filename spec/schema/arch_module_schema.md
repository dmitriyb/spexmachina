# ModuleSchema

The module.json JSON Schema (`schema/module.schema.json`) defines the structure of each module within a spec — [[eed2cf85d5c3|what one module file may declare]], and which of it a module cannot leave out.

The effective document is composed, not fixed: the shipped schema is the frame, and the resolved profile supplies one array property per module-scoped node type it declares, keyed by that type's plural key, each validated with the same envelope constraints. The default profile declares today's five arrays, and the composed document it yields is identical in content to the shipped one — the same JSON value, which a golden test asserts by structural comparison. The one deliberate change to the on-disk shape of an existing module.json is the requirement type's title-to-name rename — spec format version 1's break, migrated by one mechanical rename — while a project declaring an `endpoint` type gets an `endpoints` array validated exactly as `components` is.

Each type's declared fields reach the composed document too. An entry definition carries — besides the envelope of identity-hash `id`, non-empty `name`, optional `description` and, when the type requires content, a required `content` path — one property per field the type declares: enum-constrained for a text field carrying an enumeration, bounded for an integer field carrying bounds, an identity-hash value for a reference field (a scalar at cardinality one, an array otherwise), with a required text field composed non-empty. A field that is neither envelope nor declared is rejected by the entry-level `additionalProperties: false`, so declaring a field is the only way to open a property on a declared type — the composed schema is what makes [[5392cca550c4|the taxonomy declaration]] carriable at all. The five built-in types compose through this same path from the default profile's field declarations; no built-in definitions remain in the frame, so `preq_id` is not a frame quirk but the default profile's one cardinality-one reference field, and a profile may declare a new reference field on a built-in type exactly as on a custom one.

## Structure

```json
{
  "name": "validator",
  "description": "...",
  "requirements": [
    {
      "id": "<identity hash>",
      "preq_id": "<identity hash>",
      "type": "functional | non_functional",
      "name": "Detect broken references",
      "description": "...",
      "depends_on": ["<identity hash>"]
    }
  ],
  "components": [
    {
      "id": "<identity hash>",
      "name": "RefChecker",
      "description": "...",
      "content": "arch_ref_checker.md",
      "implements": ["<identity hash>"],
      "uses": ["<identity hash>"]
    }
  ],
  "data_flows": [
    { "id": "<identity hash>", "name": "...", "description": "...", "content": "flow_*.md", "uses": ["<identity hash>"] }
  ],
  "test_sections": [
    { "id": "<identity hash>", "name": "...", "content": "test_*.md", "describes": ["<identity hash>"] }
  ],
  "apis": [
    {
      "id": "<identity hash>",
      "name": "spex validate",
      "description": "...",
      "provided_by": ["<identity hash>"],
      "group": "cli"
    }
  ]
}
```

`name` is the only required property at the module level; every array is optional, so a module that declares nothing but its name is valid. Within the arrays the required sets follow the field declarations, and under the default profile they are: `id`, `name`, `preq_id` and `type` on a requirement — `name` being the field this contract renames from `title`, so every node type carries the same identity-bearing envelope; `id`, `name` and `content` on a component, a data_flow and a test_section alike; `id` and `name` on an api. `additionalProperties` is false at every level, so a misspelled field is an error rather than a silently ignored one.

[[80debf6eb776|The `test_sections` array]] carries an id, a name, a path to a `test_*.md` leaf and a `describes` list of component ids, so a test leaf is hashed into the merkle tree exactly like any other content leaf: by the bytes of the file its `content` names. Hashing alike is not eligibility alike. What a journal change event may describe is fixed by the journal-line schema's own `node_type` enum rather than by this one, and `test_section` is named there. Being named in that enum is permission for a task pairing, not a guarantee of one.

All `id`, `preq_id`, and cross-reference fields carry [[237fd8ffb610|the same 12-character lowercase hex identity hash]], matched here by the pattern `^[a-f0-9]{12}$`. The pattern is all this schema can enforce: that a value has the shape of an identity hash, not that it is the right one for the node carrying it. Derivation, the identity string each node type hashes from, and the check that a declared id equals it all live with `arch_schema_loader.md`.

## API Entries

The `apis` array names the external surface the module provides: a CLI subcommand, an HTTP route, a library entry point. An api is declared, not described. It has no `content` property at all — the schema gives it nowhere to put a markdown leaf, and its hash is taken from its own JSON fields, the way a requirement's is.

`name` is the surface string exactly as a caller writes it — `spex validate`, `GET /v1/specs/{id}` — never a signature. Api names are unique across the whole project rather than within one module, so two modules cannot both claim the same entry point; `spex validate` names both declarers when they try.

`provided_by` lists the components that realise the entry point, and it is module-local: the ids must resolve inside this module.json, so an api cannot claim a component in another module. `group` is a freeform label for renderers to bucket a project's surface by; spex never branches on its value, so a project may invent whatever grouping it finds useful.

## Edge Types

- `preq_id`: module requirement → project requirement (traceability)
- `depends_on`: requirement → requirement (within module requirements)
- `implements`: component → requirement (fulfillment)
- `uses`: component → component (dependency)
- `uses` (data_flow): data_flow → component (involvement)
- `describes` (test_section): test_section → component (test coverage)
- `provided_by`: api → component (which component realises the entry point)
- `content`: any node → markdown leaf (described_in edge)

## Design Rationale

The small required set at the module level is what enables incremental authoring. The `preq_id` field on requirements is required (not optional) — every module requirement must trace to a project requirement. This creates the traceability chain: project requirement → module requirement → component. The `test_sections` array adds a parallel verification chain, component → test_section, enabling test coverage analysis; the `apis` array adds a third, api → component, which names the component behind each entry point a caller actually touches.

Content paths are relative to the module directory, keeping file references local and relocatable.
