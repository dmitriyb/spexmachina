# CoupledSectionChecker

Validates the `sections` array in project.json generically. This checker is not specific to any particular section type (e.g., delivery) — it handles all section types based on the `type` field. That generality is the point of [[d99ef6b9b776|validating sections and coupled modules]] in one place: a new coupled section needs no change here, only a module of the same name and a `section.schema.json` inside it.

## Responsibilities

- Validate envelope fields on every section entry (id, name, type present and correctly typed)
- Check section ID uniqueness within the sections array
- Check section name uniqueness (no two sections can share a name)
- For sections with `type: "coupled"`:
  - Verify a module with matching `name` exists in the project's modules array
  - Verify `section.schema.json` exists at `spec/<module-path>/section.schema.json`
  - Load and compile the section schema
  - Strip envelope fields (id, name, type) from the section entry, then validate the remaining content against the module's section schema
- For sections with other types: validate envelope only (no module coupling enforced)

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when the spec loads and every section is well formed. If the spec cannot be loaded, the load failures are returned under this checker's own name, located at `project.json` or at the `module.json` that failed rather than at any section, and no section is examined. A project with no `sections` array, or an empty one, is clean rather than incomplete: nothing to validate is not a violation.

Sections are visited in the order they are declared, and each one is judged on its own. A section missing any envelope field is reported and then set aside: its uniqueness and coupling checks are skipped, and the next section is still examined. A duplicate id or name, by contrast, is reported without stopping that section's coupling checks. Within one coupled section the coupling chain does stop at its first failure: a section whose module is absent is not then reported for a missing schema file as well.

The checker never mutates the spec and never writes output — aggregation, sorting and formatting belong to ErrorReporter.

## Integration with Validation Pipeline

CoupledSectionChecker plugs into the existing validation pipeline alongside SchemaChecker, DAGChecker, etc. ValidateCommand wires it in and its errors flow through ErrorReporter.

It runs after [[651d5315eebf|SchemaChecker]] and reads section envelopes on the assumption that the project file is structurally valid per the project schema. That assumption is not a delegation: the envelope check above looks for `id`, `name` and `type` again on its own, so a section missing one of them is reported twice in a single run — once under `schema` and once under `coupled_section`, both against the same path. What only this checker derives is the other half, which the project schema cannot describe: the section's freeform content, checked against the `section.schema.json` the coupled module supplies. That schema is read from the module's own directory at validation time rather than shipped inside the binary, because it is authored by the module rather than by spex core.

## Content Extraction

Envelope and content are separated before the module's schema ever sees the section. A `section.schema.json` therefore describes only the fields its module invented, and must neither require nor forbid `id`, `name` and `type` — those three are the checker's business and are removed first.

This is what keeps the checker generic. It never has to know what a section holds: whatever remains after the envelope comes off is handed to the module's schema, and every judgement about that content belongs to the module that declared it.

## Error Messages

Every entry raised against a section is located by that section's position in the array — `project.json:/sections/<index>` — because a section whose envelope is incomplete may have no id or name left to be located by. The load failures `## Interface` describes are the exception: they are located at the file that failed to load. Beyond that location, an entry raised against a section carries:

- The section name, on the content messages, on the duplicate-name message, and on the coupling messages that report an absent module, a missing `section.schema.json`, a schema that would not compile, or a section body that would not parse. Three coupling messages carry no name: when the module's `section.schema.json` cannot be read, cannot be parsed as JSON, or cannot be loaded into the compiler, the message carries that file's path and the underlying error alone. The envelope messages carry the section's position in the array instead, and the duplicate-id message carries the duplicated id together with the index of the earlier section already holding it
- Which check failed: envelope, coupling, or content schema
- Actionable guidance naming the thing to supply — the module the section expects, or the `section.schema.json` that module is expected to hold. When some module's name differs from the section's only by case, the message names it as the likely intended match
- For a content violation, the path to the failing field inside the section when the violation sits at a field, so the report points at the field rather than at the section as a whole; a violation at the section root — a missing required property, for instance — carries no field path and the message names the section alone. And — uniquely among this checker's entries — the `schema_path` of the section-schema rule that rejected it
