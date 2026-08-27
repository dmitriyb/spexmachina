# IDValidator

Validates identity-hash uniqueness, cross-reference integrity, mandatory `preq_id` on module requirements, and `priority` presence on project requirements.

## Responsibilities

### ID Uniqueness
- [[707094f8868b|ID uniqueness]] is scoped to one array: every identity hash must be unique within the array that contains it, and nowhere wider. The checked arrays are the ones the resolved profile declares; under the default profile that is `requirements`, `modules` and `sections` in project.json, and `requirements`, `components`, `data_flows`, `test_sections` and `apis` in each module.json
- Uniqueness is checked by tallying each hash in a per-array set of strings — any hash counted more than once is reported with its array location and the offending hash
- Collisions across distinct logical nodes are mathematically improbable in the 48-bit hash space, but the validator still checks them so hand-edited or hand-merged files cannot smuggle a stale ID into a new node

### API Name Uniqueness

One uniqueness rule in the spec is not scoped to a single array in a single file. An api's `name` is the exact external surface string callers type, so two modules declaring the same name are not two nodes that happen to collide — they are two claims on one surface. API names are therefore checked across every module.json in the project, and a duplicate is reported once, naming every module that declared it.

This is the only cross-file uniqueness check in this component. Every other one compares a single array against itself, so it needs one file at a time; this one needs every module.json in the project loaded together, and reports against the first module that declared the name.

### API Name Recoverability

A declared name of any type the resolved profile marks name-declarable — the same per-type role flag the removal-name sweep iterates; under the default profile, apis and components — is rejected unless tokenizing it the way the removal-time corpus scan tokenizes prose reproduces it exactly, in at least one and at most six whitespace-separated words. The rule is not a style preference: it is the corpus scan's reachability condition applied at the point of declaration. Every phrase that scan builds is a join of corpus tokens with single spaces, so a name that is not itself such a join is a name no candidate phrase can equal — the node would be unsweepable from the moment it was declared, and nothing would say so.

`spex validate [--json]` is the shape this rejects: the brackets are stripped by the tokenizer, so the declared name and the phrase the scan rebuilds differ. `spex validate --json` is declarable; so is `spex map get`. The api name is the surface string alone — never a signature, never an argument placeholder.

### Cross-Reference Integrity
[[4b399b1c568f|Cross-reference integrity]] holds when every edge in the spec names a node that exists. The edge set checked is the resolved profile's declaration — which reference fields a type may carry and what node type each points at — so a profile-declared edge is resolved by the same set-membership machinery, and an id whose type the profile does not declare has no array to resolve against. All references are identity hash strings, validated by string set membership against the appropriate per-array set. Under the default profile the declared edges are:

- `implements`: component → requirement identity hashes within the same module
- `uses` (component): component → component identity hashes within the same module
- `uses` (data_flow): data_flow → component identity hashes within the same module
- `depends_on`: requirement → requirement identity hashes within the same scope
- `requires_module`: module → module identity hashes in project.json
- `preq_id`: module requirement → project requirement identity hash (must exist)
- `describes` (test_section): test_section → component identity hashes within the same module
- `provided_by`: api → component identity hashes within the same module

`provided_by` is module-local like every other edge in this list. An api belongs to the module owning its entry point, and a component in another module that participates in the surface is reached through the entry point's `uses` edges — never by pointing `provided_by` across a module boundary.

There is no integer parsing, no path decomposition (`module/N/component/M`), and no comparison across types — every check is a single `set[hash]` lookup.

### Mandatory preq_id
- Every module requirement must have a non-empty `preq_id` field whose value is the identity hash of an existing project requirement
- Missing or empty `preq_id` is reported as an error
- A `preq_id` that does not match any project requirement is reported as a dangling reference

### Priority Presence
- [[9445f0fe054d|Validate priority]] reaches project requirements only: each one must carry a `priority` field holding an integer 0-4. A module requirement has no priority of its own
- Missing or out-of-range priority is reported as an error against the requirement that lacks it

## Interface

Given the path to a spec directory, the checker loads project.json and every module.json and returns a flat list of validation entries — empty when the spec is clean. If the spec cannot be loaded, the load failures are returned and no further checks run. The checker never mutates the spec and never writes output: aggregation, sorting and formatting belong to ErrorReporter.

Uniqueness and name shape run before cross-reference resolution, and the two phases do not mix in a single run. When the first phase reports anything at all, the checker returns those entries alone and skips reference resolution — a reference cannot be unambiguously resolved while two nodes share a hash, so resolving anyway would produce misleading errors.

Modules are visited in sorted name order, so the same spec always produces the same entries in the same sequence.

Every ID and cross-reference field is a string end to end — the loaded spec carries identity hashes as text, so the checker compares them directly with no conversion, parsing or decomposition.

Id derivation runs over the node types the resolved profile declares: each module-scoped node's declared id is recomputed from its identity string, whose middle part is the declared type name, so a profile-declared node's id is derived and checked exactly as a built-in one's. Under the default profile the derived set is today's five module-scoped types.

## Error Format

Each error raised against a declaration is located by that declaration — the file, the array inside it and, where one node is at fault, that node's identity hash — and its message names the reference field that failed together with the dangling target hash. The load failures `## Interface` describes are the exception: they are located at `project.json` or at the `module.json` that failed, with no array or node beneath it. Naming the field is what keeps `implements` and `uses` on the same component apart in a single report. A missing field is reported the same way, naming the field in place of a target. Identity hashes are short (12 characters) so error messages remain readable even when several appear in one report.
