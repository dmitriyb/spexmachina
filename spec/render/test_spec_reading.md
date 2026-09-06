# Spec Reading Tests

## Setup

S9 states its fixture in its Given: a `spec/profile.json` declaring an `endpoint` type and a module carrying two endpoints with content files. Reading goes through `ReadSpec` over that spec directory with the profile resolved first, and the assertion is on the graph the reader returns.

## Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.

## Edge Cases

### S9: Declared arrays are read through the resolved profile

**Given** a `spec/profile.json` declaring an `endpoint` type (module-scoped, plural key `endpoints`, content-bearing), and a module.json carrying two endpoints with content files.

**When** `ReadSpec(specDir)` is called.

**Then:** The returned graph holds both endpoint nodes with their ids, names and loaded content, exactly as component nodes are held — SpecReader iterates the arrays the resolved profile declares rather than a fixed five, so a declared type's nodes reach every downstream renderer. Under the default profile the read set is exactly today's arrays.
