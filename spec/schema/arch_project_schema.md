# ProjectSchema

The project.json JSON Schema (`schema/project.schema.json`) defines the top-level structure of a spex-machina spec — [[f471f2764ab8|the project file's shape]]: what may appear in it, what must, and in what form.

The effective document is composed, not fixed: the shipped schema is the frame — the envelope fields, the identity-hash pattern, `additionalProperties: false` — and the resolved profile supplies the array properties for the project-scoped node types. Under the default profile the composed document is identical to the shipped one, which a golden test asserts, so everything below describes both the shipped frame and the default composition at once; a project declaring an additional project-scoped type gets its array added to the composed document with the same envelope constraints.

## Structure

```json
{
  "name": "spexmachina",
  "description": "...",
  "version": "0.1.0",
  "requirements": [
    {
      "id": "<identity hash>",
      "type": "functional | non_functional",
      "title": "Validate spec structure",
      "description": "...",
      "priority": 2,
      "derivation": "pending",
      "depends_on": ["<identity hash>"]
    }
  ],
  "modules": [
    {
      "id": "<identity hash>",
      "name": "validator",
      "path": "validator",
      "description": "...",
      "requires_module": ["<identity hash>"]
    }
  ],
  "sections": [
    {
      "id": "<identity hash>",
      "name": "delivery",
      "type": "coupled",
      "...": "freeform content, validated elsewhere"
    }
  ]
}
```

`name` and `modules` are the schema's only top-level required properties, and `modules` must carry at least one entry. Inside the arrays the required sets are small: `id`, `type` and `title` on a requirement; `id`, `name` and `path` on a module; `id`, `name` and `type` on a section. `priority` is an integer 0–4; the schema leaves it optional — deliberately, so requirements can gain priorities in stages — while `spex validate` reports a project requirement that carries none. `derivation` is optional too, and `"pending"` is its only permitted value: the property declares a requirement not yet derived into any module, so the requirement can be named honestly before its decomposition exists, and any other value fails schema conformance. A requirement without the field is an ordinary requirement that must be derived — the state is opt-in, never a default. What declaring it buys is decided in the validator, not here; the schema only admits the declaration. `$defs` holds exactly four definitions: `identityHash`, `requirement`, `module` and `section`. There is no `milestones` array and no `test_plan` — both node types were retired from `schema/project.schema.json` along with their `$defs`.

All `id` and cross-reference fields carry [[237fd8ffb610|the same 12-character lowercase hex identity hash]], matched here by the pattern `^[a-f0-9]{12}$`. The pattern is all this schema can enforce: that a value has the shape of an identity hash, not that it is the right one for the node carrying it. Distinctness is out of its reach too — two entries of one array may repeat an id as far as this schema is concerned, and `spex validate` is what reports the duplicate. Derivation, the identity string each node type hashes from, and the check that a declared id equals it all live with `arch_schema_loader.md`.

## Edge Types

- `depends_on`: requirement → requirement (within project-level requirements)
- `requires_module`: module → module (inter-module dependency)
- `sections[].name` → `modules[].name`: coupled section references its implementing module by name match

## Sections and Coupled Modules

[[581178718888|The `sections` array is the project file's extension point]] — where a project-level concern this schema does not model is declared.

The section entry itself does NOT use `additionalProperties: false` — this is intentional. The envelope fields are validated by spex core, while the additional content fields are validated by the coupled module's `section.schema.json`. This two-layer validation allows each module to define its own content structure without requiring changes to the project schema.

When `type` is `"coupled"`, the validator enforces that a module with the same `name` exists and provides a `section.schema.json` file. See the CoupledSectionChecker in the validator module for enforcement details.

### section.schema.json Convention

The file is looked for under the coupled module's declared `path` — `spec/delivery/section.schema.json` when the `delivery` module's path is `delivery` — and a coupled section whose module ships no such file fails validation, with the path that was tried named in the error. It is an ordinary JSON Schema document, compiled when validation runs. What it is handed is not the whole entry: `id`, `name` and `type` are stripped off first, so [[20da31e277e5|the module's schema judges the freeform content and never sees the envelope]].

## Design Rationale

The small required set at the project level is what supports incremental spec authoring: a spec can begin as a name and a list of modules, and gain its requirements later.

`additionalProperties: false` ensures strict conformance at the project level — no extra top-level fields allowed. However, section entries within the `sections` array allow additional properties to support freeform content validated externally.

The requirement definition is duplicated in `module.schema.json` rather than shared through a cross-file `$ref`, and each copy carries a `$comment` naming the other. Each document stays self-contained, so no `$ref` has to resolve across files. The price is one definition kept in step by hand. The two are not identical, and the differences are the point: a project requirement may carry `priority` and `derivation`, a module requirement must carry `preq_id`. `derivation` belongs to the project copy alone because a module requirement derives by construction — its required `preq_id` is the derivation — so a pending state has no meaning there, and the module copy stays untouched.

Collections are JSON arrays rather than objects keyed by id. This preserves ordering, which matters for rendering and consistent output, and the `id` field within each item provides lookup by identifier.
