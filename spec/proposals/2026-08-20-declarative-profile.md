# Change Proposal: Declarative profile

## Context

spex's node taxonomy is compiled into the binary. "A valid spex project" therefore means one specific decomposition of software — requirement, component, data_flow, test_section, api, module — together with coverage rules that assume every component is described by a test section and every module requirement is implemented by a component. That is a good decomposition. It is not the only one: a team speccing an HTTP service thinks in endpoints and resources, a data team in datasets and transforms, a product organisation in flows and screens. For any of them, adopting spex means adopting this ontology first.

The taxonomy is not in one place. Node-type string literals appear in twenty-one non-test Go files, and the sites that matter are policy rather than plumbing:

- `schema/project.schema.json` and `schema/module.schema.json` enumerate the node arrays — `requirements`, `components`, `data_flows`, `test_sections`, `apis` — under `additionalProperties: false`, so an undeclared type is a schema error before any check runs.
- `merkle/tree_builder.go` builds a leaf per type and carries the per-type hashed field allowlist — the policy that decides what counts as a semantic change, enumerated in `arch_tree_builder.md`.
- `merkle/completeness_checker.go` branches on `requirement` and `meta` to implement the two completeness rules.
- `plan/types.go` defines the plan-relevant set as three constants, and `plan/action_classifier.go` branches on them for bead-type assignment and the two-or-more-`describes` rule.
- `validator/content_resolver.go` knows which three types carry content files; `validator/id_derivation_checker.go` names all five when deriving ids; the coverage checkers hard-code the two-link chain.
- `ingest/refresh.go` carries a per-type, per-direction absorbable allowlist.
- `cmd/spex/hashid.go` validates `--type` against a fixed switch.
- `render/json.go` iterates the five types to produce the node table the authoring skills depend on.

The trigger is the same one behind the rest of this series: adoption outside the repository that grew the tool. `2026-08-20-derivation-status` made a partially-decomposed spec expressible, and `2026-08-20-project-lifecycle` gave a project an identity on disk. Both lower the cost of starting. This one removes a ceiling: a project whose vocabulary differs cannot express itself at all, and no amount of skill authoring changes that, because the skills produce documents the schema rejects.

There is a precedent in the codebase for exactly this shape. `project.json` already carries a `sections` array whose entries are a generic envelope — id, name, type, plus raw JSON preserved verbatim — validated against a per-module `section.schema.json` supplied outside the shipped schema, under [[20da31e277e5|the section.schema.json convention]] and [[d99ef6b9b776|validate sections and coupled modules]]. Renderers iterate sections without knowing their shape. The mechanism for "the tool ships the frame, the project supplies the vocabulary" is already written and already tested; it has simply never been pointed at node types.

## Proposed change

### The profile

A new authored file, `spec/profile.json`, sitting beside `project.json` because that is where a person edits it — `2026-08-20-project-lifecycle` reserves `.spex/` for what the tool writes, and this is not that. A project that has no `spec/profile.json` gets the built-in default profile, which declares today's ontology.

The profile declares four things, and each corresponds to a policy currently spread across the code:

**Node types.** For each type: its name, the plural key naming its array in `module.json` or `project.json`, whether it is module-scoped or project-scoped, and whether it requires a content leaf.

**Legal edges.** Which reference fields a type may carry and what they may point at — today's `preq_id`, `implements`, `describes`, `uses`, `provided_by`, `depends_on`, `requires_module`. The DAG check enforces acyclicity over whatever the profile declares rather than over a fixed edge set.

**Graph rules.** The coverage chains, the plan-relevant set, the per-type hashed field allowlist, and the per-type absorbable directions that refresh honours.

**Nothing else.** The profile is a description of a vocabulary, not a place to put behaviour.

### Coverage rules become declared chains

Today's rules are three specific links: project requirement to module requirement via `preq_id`; module requirement to component via `implements`; component to test section via `describes`. Each is the same shape — *every node of type A must be the target of at least one edge of kind E from some node of type B* — so the declarative form is a list of such triples, and both existing checkers can be driven by it rather than replaced. `RequirementCoverageChecker` (`c7d0282b0e05`) keeps owning the chain it owns and reads the links from the profile; `TestCoverageChecker` (`ed7a40b68995`) does the same for its one link. The error messages stay in the shape [[168ae8fde8e2|requirement coverage validation]] and [[a88e6fb4463d|test coverage checking]] already specify, with the type names interpolated instead of literal.

### Schemas are generated, arrays are kept

`SchemaLoader` (`ee88263d6555`) composes the project and module schemas from the profile at load time instead of loading two fixed documents. The shipped schemas become the frame — envelope fields, the identity-hash pattern, `additionalProperties: false` — and the profile supplies the array properties and their per-type constraints.

This keeps the on-disk shape of every existing `module.json` unchanged under the default profile: `components` is still an array called `components`, because the default profile says so. A project declaring an `endpoint` type gets an `endpoints` array validated the same way. Nothing migrates.

Two consequences to record rather than discover. The `schema_path` field on validation entries — which [[608f8ca2e1b0|structured error output]] documents as set only by the schema check and the coupled-section check — becomes a path into a generated document, so the profile must be part of what a reader needs to interpret it. And schema composition happens once per run, before any check, so a malformed profile is a distinct and early failure rather than a cascade of confusing conformance errors.

### The fixed points

Some things are not the profile's to change, and the proposal states them so that a later reading does not treat the profile as unlimited:

- **The identity hash algorithm.** [[cdc9c58ba097|identity hash algorithm]] joins its parts with `/` and takes the first six bytes of SHA-256. It is already generic over its inputs — the node type is just a string part — which is precisely why the type vocabulary can become data without any existing hash moving. The algorithm itself does not change and is not declarable.
- **The name tokenization rule.** A removed node's name is recovered by hashing corpus phrases against its identity hash. A profile that could relax tokenization would make removals unsweepable, so the rule stays fixed for every type.
- **The merkle tree shape.** Project root, module interior nodes, leaves keyed by identity hash, and the synthetic `meta` leaf per module. The profile says what leaves exist, never how the tree is built over them.
- **The journal event format.** The task journal is append-only and permanent; its line shape is not per-project.

### The default profile reproduces current behaviour exactly

This is an acceptance criterion, not an aspiration, and it is demonstrable rather than asserted:

- Every identity hash in this repository's own spec is unchanged, because the hash inputs are unchanged. `spex validate` on the current spec with no profile file present emits a byte-identical report, and `spex diff` against the current snapshot reports no changes — the same two checks that gate every PR today.
- The generated schemas are compared against the shipped static documents as golden files. If composition from the default profile does not reproduce them, the test fails.
- The built-in default profile is itself the golden file for what the code used to hard-code: the five types, the three coverage links, the plan-relevant set, the field allowlists, and refresh's absorbable directions. A reviewer can read one document and see the whole policy that was previously spread across seven modules — which is the point of the change as much as the extensibility is.

### What owns it

The `schema` module. It already owns what a spec document may contain and the identity hash, and the profile is the declaration of both. A new component `ProfileLoader` reads `spec/profile.json` or falls back to the built-in default, validates it, and exposes the resolved profile to every consumer. Three new module requirements derive from [[d5a8407d38e1|validate spec structure]]: one for declaring the taxonomy, one for generating schemas from it, one for declaring the graph rules.

No new api. The resolved profile is observable through `spex render`, and the acceptance criterion above is checked by golden tests rather than by a command a user has to run.

## Impact expectation

This is the largest epic in the series. The size is not the profile file — it is that eight modules currently hold a copy of the taxonomy, and every one of them has to start reading it instead.

### New nodes — schema module

| Node | Type | Bead |
|---|---|---|
| Declare the node taxonomy | module requirement | no |
| Generate schemas from the profile | module requirement | no |
| Declare graph rules | module requirement | no |
| `ProfileLoader` | component | yes |

Coverage is satisfied without a new test section: `ProfileLoader` joins `describes` on **Schema loading tests** (`8719672c7580`), which today describes only `SchemaLoader` (`ee88263d6555`) and therefore gains a bead by reaching two components.

### Modified component leaves — one bead each

| Module | Component | Hash | Reads from the profile |
|---|---|---|---|
| schema | ProjectSchema | `79946d618829` | project-scoped types; the schema is composed, not fixed |
| schema | ModuleSchema | `78883b84c32d` | module-scoped types and their arrays |
| schema | SchemaLoader | `ee88263d6555` | composes both schemas at load |
| validator | SchemaChecker | `651d5315eebf` | validates against the generated schema |
| validator | IDValidator | `00beeeda5ddd` | derives ids over declared types |
| validator | ContentResolver | `5dcca0dab9bd` | which types carry content leaves |
| validator | DAGChecker | `c6c770a59d68` | declared edges, not a fixed edge set |
| validator | RequirementCoverageChecker | `c7d0282b0e05` | declared coverage chains |
| validator | TestCoverageChecker | `ed7a40b68995` | declared coverage chains |
| merkle | TreeBuilder | `dfe1467b7a4b` | per-type leaf construction and hashed field allowlists |
| merkle | CompletenessChecker | `de3309dfbd3c` | which types trigger the completeness rules |
| merkle | ImpactClassifier | `f1a672216ce9` | classification by declared type |
| merkle | DiffCommand | `c8b958ec310d` | the removed-name sweep iterates declared types |
| plan | ActionClassifier | `8aa1ab5ac102` | the plan-relevant set and bead-type assignment |
| plan | ChangesetBuilder | `4c1146bb7287` | bead type per declared type |
| ingest | RefreshHandler | `f9033352c13f` | per-type absorbable directions |
| cli | HashIDCommand | `a338e50fec70` | `--type` validated against declared types |
| render | SpecReader | `7d1150c19724` | reads declared arrays |
| render | JSONRenderer | `172d16ca0eac` | emits declared types in the node table |
| render | MarkdownRenderer | `b4b4eba6b551` | renders declared types |
| render | DOTRenderer | `45331bdc0bd0` | renders declared types |
| render | RenderCommand | `c56eefd05f42` | obliged by all three render requirements below |

Twenty-two modified plus one new: twenty-three component beads.

`Hasher` (`325f48728e04`) is deliberately absent — file, byte and child hashing are type-agnostic and stay exactly as they are.

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| schema | Define project schema | `f471f2764ab8` | ProjectSchema |
| schema | Define module schema | `eed2cf85d5c3` | ModuleSchema |
| schema | Embed schemas in binary | `b7c3bccd7c64` | SchemaLoader |
| schema | Identity hash ID constraints | `237fd8ffb610` | ProjectSchema, ModuleSchema |
| validator | JSON schema conformance | `8599f07272ad` | SchemaChecker |
| validator | Cross-reference integrity | `4b399b1c568f` | IDValidator |
| validator | Content path resolution | `5a1ce39e1c9d` | ContentResolver |
| validator | DAG acyclicity | `d0451520f7be` | DAGChecker |
| validator | Requirement coverage validation | `168ae8fde8e2` | RequirementCoverageChecker |
| validator | Test coverage checking | `a88e6fb4463d` | TestCoverageChecker |
| merkle | Identity-hash tree keying | `3ada6b800cc5` | TreeBuilder |
| merkle | Change completeness validation | `6f8284df92a2` | CompletenessChecker, DiffCommand |
| merkle | Classify impact | `425146f32e96` | ImpactClassifier, DiffCommand |
| plan | Classify actions | `de42d9efa750` | ActionClassifier |
| ingest | Refresh mode for impl_only drift | `e68653819f38` | RefreshHandler |
| cli | Identity hash computation command | `0c395440d59f` | HashIDCommand |
| render | Render JSON | `1078c088e0c6` | SpecReader, JSONRenderer, RenderCommand |
| render | Render markdown | `8828685278e9` | SpecReader, MarkdownRenderer, RenderCommand |
| render | Render DOT | `a596d8caefb1` | SpecReader, DOTRenderer, RenderCommand |

Every obligation is discharged by the component table above.

[[cdc9c58ba097|Identity hash algorithm]] is deliberately **not** amended. Leaving it untouched is what makes "no existing hash moves" a structural fact rather than a promise.

### Meta-leaf accounting

`schema`, `validator`, `merkle`, `plan`, `ingest`, `cli` and `render` all have `module.json` changes. In every one of them at least one requirement in the same module also changes, so the meta rule is suppressed throughout and no module owes leaves from components not already listed.

### Modified test sections

| Module | Test section | Hash | Bead |
|---|---|---|---|
| schema | Schema loading tests | `8719672c7580` | yes — gains `ProfileLoader`, reaching two components |
| schema | Schema validation tests | `96f944302b78` | yes |
| validator | Conformance and content tests | `15019980fcea` | yes |
| validator | Graph structure tests | `78ac211428bb` | yes |
| validator | Name and coverage tests | `15d919047039` | yes |
| merkle | Hashing tests | `31b096a76fd4` | yes |
| merkle | Diff and classification tests | `95a279cbdcbc` | yes |
| merkle | Merkle command tests | `49a61e0d5737` | no |
| plan | Classification tests | `f3c4a2c35344` | yes |
| plan | Changeset builder tests | `338edabe0796` | yes |
| ingest | Refresh mode tests | `a483524c406c` | no |
| cli | Hash ID Command Tests | `379d1bf22453` | no |
| render | Renderer tests | `23b86e6f80c7` | yes |
| render | Spec reading tests | `5ba9651a675e` | no |

Nine of the fourteen produce beads.

The checks that can actually fail, and that carry the acceptance criterion: a golden test asserting the schemas composed from the default profile equal the shipped static documents; a hash test asserting every identity hash in this repository's spec is unchanged under the default profile; a validate test asserting a byte-identical report; and a profile test asserting a second, deliberately different profile — one type renamed, one coverage link dropped — produces a spec that validates under itself and fails under the default.

### Scope

Thirty-two beads under one epic: twenty-three component leaves — twenty-two modified, one new — and nine test sections. No node is renamed and nothing is removed, so there is no delete-plus-create and no link rewriting.

If the epic needs splitting, the seam is clean and worth naming: the schema generation and the profile itself (schema module, plus `SchemaChecker`) stand alone and change no behaviour; everything downstream then converts one module at a time, since each consumer reads the resolved profile independently. Splitting on any other line leaves the taxonomy declared in one place and hard-coded in another, which is worse than either end state.
