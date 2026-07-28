# ContentResolver

Validates that all `content` paths in module.json files resolve to existing markdown files, which is what [[5a1ce39e1c9d|content path resolution]] requires of the spec.

## Responsibilities

- Walk all `content` fields in components, data_flows, and test_sections
- Resolve each path relative to its module directory
- Report missing files as validation errors

A test leaf is walked on the same terms as any other: a `test_sections` entry names its `test_*.md` file in a `content` field and is resolved against the same module directory, which is all [[530dc49d7135|test content path resolution]] asks for. There is nowhere else for a test path to hide — the project file declares no test nodes of its own, so every one lives in some module.json.

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when the spec loads and every declared path resolves. If the spec cannot be loaded, the load failures are returned under this checker's own name, located at `project.json` or at the `module.json` that failed rather than at any node, and no path is examined. Modules are visited in the order `project.json` lists them, and within a module the references are ordered by node type and then by node name, so the same spec always produces the same entries in the same sequence. The checker only asks whether each path exists; it never opens a content file, so nothing inside a leaf can make this check fail.

## Behavior

1. For each module in `project.json`, read its `module.json`
2. For each component, data_flow, and test_section with a `content` field:
   - Form the full path from the spec directory, the module's `path`, and the `content` value
   - Check if the file exists
3. Report each missing file with the module name and node that references it

Each entry raised against a declared path is located by module, node type and node name — `<module>/module.json:/<type>s/<node name>/content` — so a report points at the declaration to fix, and carries the unresolved path in its message. The load failures `## Interface` describes are the exception: they are located at the file that failed to load. A path the checker cannot examine at all is reported with a message that distinguishes it from one that is simply not there.

## Edge Cases

- A `content` value that is empty is passed over rather than reported. It is not a hole in the check: the module schema requires a non-empty `content` on every component, data_flow and test_section, so an empty one is already reported against the schema
- Content path should not contain `..` or absolute paths — flag these as errors. A path is rejected on its shape before its existence is looked at, so a traversal is refused at the declaration rather than followed
