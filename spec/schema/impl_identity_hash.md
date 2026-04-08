# Identity Hash Algorithm

How the canonical spec node ID is computed from a node's position in the spec graph.

## Algorithm

```go
func IdentityHash(parts ...string) string {
    identity := strings.Join(parts, "/")
    sum := sha256.Sum256([]byte(identity))
    return hex.EncodeToString(sum[:6])
}
```

The function joins its arguments with `/`, computes SHA-256 of the resulting string, and returns the first 6 bytes as a 12-character lowercase hex string.

## Identity Strings

The identity string is the human-readable description of where a node lives in the spec graph. It is built from fields that already exist on the node — name, title, module, type — so anyone with the spec source can recompute any ID by hand.

| Node type | Parts passed to IdentityHash | Resulting identity string |
|---|---|---|
| Project requirement | `"project", "requirement", title` | `project/requirement/<title>` |
| Module | `"module", name` | `module/<name>` |
| Module requirement | `module, "requirement", title` | `<module>/requirement/<title>` |
| Component | `module, "component", name` | `<module>/component/<name>` |
| Impl section | `module, "impl_section", name` | `<module>/impl_section/<name>` |
| Data flow | `module, "data_flow", name` | `<module>/data_flow/<name>` |
| Test section | `module, "test_section", name` | `<module>/test_section/<name>` |
| Milestone | `"milestone", title` | `milestone/<title>` |
| Test scenario | `"test_plan", "scenario", name` | `test_plan/scenario/<name>` |

## Why 12 hex characters

Twelve hex characters = 48 bits of hash space ≈ 2.8 × 10¹⁴ possible values. By the birthday bound, collisions become likely around 2²⁴ ≈ 16 million nodes. Spec graphs are hundreds of nodes, not millions, so collision probability is negligible. Twelve characters fit comfortably in JSON, in command-line output, and in human-readable diffs while staying long enough to be effectively unique.

## Why these parts

The identity string captures three properties:

1. **Where the node lives** (`module/...` or `project/...`) — disambiguates same-named nodes in different modules.
2. **What kind of node it is** (`component`, `requirement`, `impl_section`, ...) — disambiguates a component from a requirement of the same name within one module.
3. **Its human identifier** (`name` or `title`) — the field a spec author writes by hand.

The identity string deliberately excludes positional information (array index, sibling order) and content (description, body text). Reordering nodes within an array does not change any ID. Editing a description does not change any ID. Renaming a node does change its ID — that is intentional, see "Renaming is destructive" in the proposal.

## Why parallel branches do not collide

Two branches that add different nodes to the same module assign different `name`/`title` values, so their identity strings differ, so their hashes differ. Two branches that add the *same* node (same name in the same module) produce the same hash, which is correct — they are describing the same logical thing and should merge cleanly. There is no integer counter to coordinate, so no race condition exists.

## Determinism

The algorithm is pure:

- SHA-256 is deterministic.
- `strings.Join` is deterministic.
- `hex.EncodeToString` produces lowercase output.
- The truncation length is fixed at 6 bytes.

Same inputs always produce the same output, on any platform, in any process. This is what makes the identity hash safe to use as a stable cross-reference key in JSON files committed to git.

## Sharing

`IdentityHash` lives in the `schema` Go package. Every other module — validator, merkle, impact, apply, map — imports `schema.IdentityHash` rather than reimplementing the algorithm. If the truncation length, hash function, or part separator ever needs to change, only this one function changes and every caller picks up the new behavior.
