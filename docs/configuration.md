# The spec format

A spec directory is a JSON skeleton with markdown content leaves. The JSON is
machine-readable and carries structure and identity; the markdown is
human-readable and carries prose. The merkle tree hashes both.

The authoritative definitions are the JSON Schemas in `schema/` —
`project.schema.json`, `module.schema.json`, `journal-line.schema.json` and
`drift.schema.json`. `spex validate` enforces them. This document explains
what the fields mean and why they are shaped that way.

## Layout

```
spec/
├── project.json          required — the root
├── proposals/            why each change was made
└── <module>/
    ├── module.json       one per declared module
    └── *.md              content leaves referenced by `content` fields
.spex/
├── snapshot.json         generated — the merkle baseline, written by ingest
└── history.jsonl         generated — the task journal, appended by ingest
```

Only `project.json` and the module directories it declares are authored by
hand. The two files under `.spex/` are created by `spex init`, written by
`spex ingest`, committed to git, and never edited directly — see [`architecture.md`](architecture.md).

## Identity hashes

Every node carries an `id`: 12 lowercase hex characters, the first 6 bytes of
`SHA256` over the node's identity string. The identity string is
`<type>/<name>` for project-scoped nodes and `<type>/<module>/<name>` for
module-scoped ones.

You do not invent these. Compute them:

```sh
spex hash-id --type requirement --name "Declared stack"
spex hash-id --type component --module merkle --name "Hasher"
```

`spex validate` recomputes every ID from its name and rejects any that does
not match, so a hand-edited or stale hash fails the gate rather than silently
pointing somewhere wrong.

**Renaming a node changes its ID.** There is no rename operation — the old
identity is removed and a new one is added. This is intentional: the name is
part of the node's identity, so changing it carries task consequences.

## `project.json`

Required: `name`, `modules`.

| Field | Type | Purpose |
|---|---|---|
| `name` | string | Project name |
| `description` | string | Project description |
| `version` | string | Project version string |
| `requirements` | array | Project-level requirements |
| `modules` | array | Module declarations |
| `sections` | array | Project-level sections with typed envelopes |

### Requirements

Required: `id`, `type`, `name` — and `priority`, which the validator demands
even though the schema calls it optional.

| Field | Type | Purpose |
|---|---|---|
| `id` | identity hash | `SHA256("requirement/<name>")`, first 12 hex chars |
| `type` | `functional` \| `non_functional` | Requirement kind |
| `name` | string | Short name — what the ID derives from |
| `description` | string | The requirement itself, in full |
| `priority` | integer 0–4 | Required on a project requirement: the schema calls it optional, but `spex validate` rejects one without it (`project requirement <id> missing priority`) |
| `depends_on` | array of identity hashes | `depends_on` edges to other requirements |
| `derivation` | `"pending"` | Optional; declares a requirement not yet derived into any module — see below |

**Bootstrapping.** `validate` requires every project requirement to be derived
into at least one module requirement. A requirement written before its module
exists declares `"derivation": "pending"`: the coverage check then reports it
as a disclosure note instead of an error, and the gate stays green. Remove the
field once a module requirement carries its `preq_id`. Module requirements
have no such escape — the requirement-to-component link admits no exemption.

### Modules

Required: `id`, `name`, `path`.

| Field | Type | Purpose |
|---|---|---|
| `id` | identity hash | `SHA256("module/<name>")` |
| `name` | string | Module name |
| `path` | string | Relative path to the directory holding its `module.json` |
| `description` | string | Module description |
| `requires_module` | array of identity hashes | `requires_module` edges |

### Sections

Required: `id`, `name`, `type`. A section is a typed envelope with freeform
content preserved as raw JSON — renderers iterate sections generically and
reach the freeform fields without knowing the coupled module's schema. `type`
is the envelope discriminator (for example `coupled`).

## `module.json`

Required: `name`.

| Field | Type | Purpose |
|---|---|---|
| `name` | string | Module name — must match the declaration in `project.json` |
| `description` | string | Module description |
| `requirements` | array | Module-level requirements |
| `components` | array | Architecture components |
| `data_flows` | array | Data flows between components |
| `test_sections` | array | Integration and acceptance test descriptions |
| `apis` | array | External entry points this module provides |

### Module requirements

Required: `id`, `type`, `name`, `preq_id`.

Same shape as a project requirement, plus **`preq_id`** — the identity hash of
the project requirement this one derives from. That field is what keeps the
requirement tree connected: a module requirement never floats free. It must
not carry `priority` or `derivation`; the module schema rejects both.

### Components

Required: `id`, `name`, `content`.

| Field | Type | Purpose |
|---|---|---|
| `id` | identity hash | `SHA256("component/<module>/<name>")` |
| `name` | string | Component name |
| `description` | string | Short description |
| `content` | path | Relative path to its markdown leaf (`described_in` edge) |
| `implements` | array of identity hashes | Requirements this component implements |
| `uses` | array of identity hashes | Other components it depends on |

### Data flows

Required: `id`, `name`, `content`. Adds `uses` — the identity hashes of the
components involved in the flow.

### Test sections

Required: `id`, `name`, `content`. Adds `describes` — the identity hashes of
the components this section's tests cover.

### APIs

Required: `id`, `name`.

| Field | Type | Purpose |
|---|---|---|
| `id` | identity hash | `SHA256("api/<module>/<name>")` |
| `name` | string | The exact external surface string as callers write it — `spex diff`, `GET /v1/things` |
| `description` | string | What the entry point does |
| `provided_by` | array of identity hashes | Components in this module providing it |
| `group` | string | Freeform grouping label for renderers (`cli`, `http`, …). `spex` never branches on it |

An `api` node has no `content` field. It names a surface; the component that
provides it carries the prose.

## Content leaves

Any node with a `content` field points at a markdown file relative to the
module directory. That file is a leaf of the merkle tree, hashed by streaming
its bytes — so any byte change to it is a spec change.

Content files are ordinary markdown. Cross-references between spec nodes use
the `[[<identity-hash>|<display name>]]` form, which `spex validate` resolves;
an unresolvable link fails the gate.

## What validate enforces

Run `spex validate` before anything else. It checks:

- **Schema conformance** — every JSON file against its schema
- **Content resolution** — every `content` path exists on disk
- **ID derivation** — every `id` matches the hash of its own name
- **ID uniqueness** — within each array
- **DAG acyclicity** — no cycles across any edge type
- **Requirement coverage** — every requirement is implemented by a component
- **Test coverage** — every component is described by a test section
- **Link resolution** — every cross-reference resolves to a live node
- **Name consistency** — declared names agree across files
- **Coupled sections** — typed section envelopes are well formed

Each check lives in its own file under `validator/`, and each reports with the
path of the offending field.

One related check deliberately lives elsewhere: the *removed-name* check runs
in `spex diff`, not here, because it needs the classified changes to know
which name just disappeared. It reports as a completeness error and exits 2.
See [`commands.md`](commands.md).

## Generated files

`.spex/snapshot.json` is the merkle baseline: the tree as of the last ingest.
It moves only when `spex ingest` writes it, and moving it is a deliberate act
— see the drift and baseline discussion in [`skills.md`](skills.md).

`.spex/history.jsonl` is the task journal: append-only, one JSON object per
line, schema in `schema/journal-line.schema.json`. Fold it forward for the current
node-to-task mapping; read it whole for the biography of a node that has since
been removed. `spex map` queries it.
