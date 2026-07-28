# HashIDCommand

[[6c378662424f|`spex hash-id`]] — computes and prints the identity hash for a spec node. It exists so an id is derived rather than invented, which is the whole of [[0c395440d59f|Identity hash computation command]].

## Usage

```
spex hash-id --module <module> --type <type> --name <name>
```

Prints a single 12-character lowercase hex string to stdout and exits 0. Nothing else reaches stdout, so the output can be substituted straight into a spec file. The command reads no spec directory and opens no file — it is a pure function of its three flags. It is a child of [[b6758cdfabc4|RootCommand]] and inherits the root's persistent flags, but consults none of them: `--spec-dir` has no effect here.

## Flags

| Flag | Required | Description |
|---|---|---|
| `--module` | for module-scoped nodes | Module name (e.g., `impact`). Omit for project-level nodes. |
| `--type` | yes | Node type: `requirement`, `component`, `impl_section`, `data_flow`, `test_section`, `api`, `module` |
| `--name` | yes | Node name or title (the human-readable identifier) |

## Identity String Construction

The command maps `--type` onto the identity string the schema defines for that node type, then hashes it. The table below is the whole mapping; there is no fallback branch behind it, so the set of types that produce a hash is exactly the set of rows here.

| --type | --module required? | Identity string |
|---|---|---|
| `requirement` + `--module` | yes | `<module>/requirement/<name>` |
| `requirement` (no `--module`) | no | `project/requirement/<name>` |
| `component` | yes | `<module>/component/<name>` |
| `impl_section` | yes | `<module>/impl_section/<name>` |
| `data_flow` | yes | `<module>/data_flow/<name>` |
| `test_section` | yes | `<module>/test_section/<name>` |
| `api` | yes | `<module>/api/<name>` |
| `module` | no | `module/<name>` |

For `--type api` the `--name` is the exact external surface string a caller types — `spex map get`, not `spex map get <record-id>` and not a Go signature. An api's id cannot be authored any other way: the validator recomputes `IdentityHash(<module>, "api", <name>)` for every declared api and rejects a mismatch, so hand-writing the hex is a guaranteed error rather than a shortcut.

## Examples

```
$ spex hash-id --module impact --type component --name NodeMatcher
a1b2c3d4e5f6

$ spex hash-id --type module --name schema
0f1e2d3c4b5a

$ spex hash-id --type requirement --name "Validate spec structure"
9988776655ee

$ spex hash-id --module validator --type requirement --name "ID uniqueness"
ddeeff001122
```

(Hex values above are illustrative, not real hashes.)

## Error Handling

- Missing `--type` or `--name`: exit 1 with usage error.
- `--module` omitted for a type that requires it (component, impl_section, etc.): exit 1 with error indicating `--module` is required for that type.
- `--module` supplied for `--type module`, the one type that ignores it: the flag is silently ignored (the identity string does not include it).
- Unknown `--type` value: exit 1 listing valid types.

## Design Rationale

The command exists so spec authors (both humans and LLMs) can compute identity hashes without writing Go code or running shell pipelines. The `/spec` skill calls it during authoring to populate `id` fields in `project.json` and `module.json`. It is also useful for debugging — given a hash in the diff output, reversing it is impossible, but given a node's module/type/name the hash is trivially recomputable.
