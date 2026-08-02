# ModuleSchema

The module.json JSON Schema (`schema/module.schema.json`) defines the structure of each module within a spec — [[eed2cf85d5c3|what one module file may declare]], and which of it a module cannot leave out.

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
      "title": "Detect broken references",
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

`name` is the only required property at the module level; every array is optional, so a module that declares nothing but its name is valid. Within the arrays the required sets are fixed: `id`, `preq_id`, `type` and `title` on a requirement; `id`, `name` and `content` on a component, a data_flow and a test_section alike; `id` and `name` on an api. `additionalProperties` is false at every level, so a misspelled field is an error rather than a silently ignored one.

[[80debf6eb776|The `test_sections` array]] carries an id, a name, a path to a `test_*.md` leaf and a `describes` list of component ids, so a test leaf is hashed into the merkle tree exactly like any other content leaf: by the bytes of the file its `content` names. Hashing alike is not eligibility alike. What a bead-map record may point at is fixed by the bead-map schema's own `node_type` enum rather than by this one, and `test_section` is named there. Being named in that enum is permission for a record, not a guarantee of one.

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
