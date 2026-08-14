# ImpactClassifier

[[425146f32e96|Classifies each diff change by the impact level of the spec layer it touched]]. It reads the node metadata every change already carries — node type and owning module — and parses no path prefixes.

## Responsibilities

- Analyze each change's node metadata to determine the spec layer affected
- Classify into four impact levels
- Attach impact classification to each change for downstream consumption

## Impact Levels

| Level | Condition | Meaning |
|-------|-----------|---------|
| `impl_only` | Node type is `test_section` | Implementation detail changed, architecture stable |
| `contract` | Node type is `data_flow` or `api` | A surface shared beyond one component changed — for a data_flow, every component in its `uses` array must be updated in lockstep |
| `arch_impl` | Node type is `component` | Architecture changed, dependent modules may be affected |
| `structural` | Node type is `meta` or `requirement` | Spec structure changed — new/removed nodes, changed edges, changed requirements |

The `contract` level sits between `impl_only` and `arch_impl`: contract changes are not purely local to one component (so `impl_only` is wrong), but they also do not rewire graph topology (so `arch_impl` is wrong). `data_flow` and `api` land there together because they are the same kind of surface seen from two sides — a data_flow is a shape agreed between components inside the module, an api is a surface the module declares to its callers — and neither belongs to a single component. Downstream node matchers skip `structural` changes but forward `contract` changes so a dedicated data_flow task bead is produced.

## Interface

One call, taking the diff's changes together with a map from module identity hash to module name. It returns one classified change per change it was handed, in the order it was handed them, and has no failure mode.

Each returned change carries everything the diff change carried plus its impact level. One field does not survive untouched: the owning module arrives as an identity hash and leaves as the module's name, so a report says `merkle` where the diff said a hash. A hash the name map does not cover is passed through as the hash, and a project-level change keeps its empty module.

The module association already travels with each change from [[cb262b280963|DiffEngine]], so nothing here parses a path to work out which module a change belongs to.

## Rules

Classification reads the node metadata the DiffEngine attached to each change — its `node_type` and its `module` — and never a path:

1. If a change's `node_type` is `test_section` → `impl_only`
2. If it is `data_flow` or `api` → `contract`
3. If it is `component` → `arch_impl`
4. If it is `meta` (module.json or project.json) → `structural`
5. If it is `requirement` → `structural`

A `node_type` that none of those five rules names gets no level at all and is reported as `unknown` —
on the change's own line in both output formats, and in the JSON summary's per-impact counts.
TreeBuilder never produces such a type, because it stamps every leaf it creates with one of the node
types the rules already cover. It is reachable anyway: a removed leaf takes its node type from the
snapshot side rather than from the current tree, and node types load out of `spec/.snapshot.json`
verbatim, so a foreign or hand-edited snapshot can carry a type no spec directory would produce.

## Call site

ImpactClassifier is invoked from `spex diff` only. The diff command builds
the classified-changes list once per invocation and writes it under the
top-level `changes` array of the diff JSON output. The downstream consumer
(`spex plan`) reads the classification through that JSON;
ImpactClassifier itself has no separate CLI surface and is not called
during snapshot persistence (`spex ingest`'s SnapshotSaver only rebuilds
the tree, not the classification).

Classification is strictly per change. Each change gets its level from its own
node type and from nothing else, and the five rules above are the whole of the
mapping: a module whose changes land on several levels is given no combined
level here, no change is widened to a second node, and a structural change in
one module is not propagated here to the modules that depend on it. Any
aggregating or propagating belongs downstream, to a consumer reading the
classified list.
