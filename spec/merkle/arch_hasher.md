# Hasher

Computes cryptographic hashes for merkle tree nodes. Every digest in the tree is
taken here: [[6c556c0db942|one hash per content file and per JSON file in the
spec]] for the leaves, and one hash per interior node above them.
[[26358c05face|The same bytes must always produce the same digest, and the
interior digest must not depend on the order children were discovered]] — that
requirement is what fixes the composition below, not a convenience.

## Responsibilities

- Compute SHA-256 hash of a file's contents (leaf hash)
- Compute SHA-256 hash of a byte string handed over directly (leaf hash for a node that has no content file)
- Compute SHA-256 hash of a sorted list of child hashes (interior hash)
- Return hashes as hex-encoded strings

## Interface

Three operations, none of which knows anything about the spec graph.

**Leaf hash from a file** — given a path, the SHA-256 digest of that file's
bytes, rendered as 64 lowercase hex characters. The file is read as bytes: no
line-ending normalisation, no trailing-whitespace trimming, no parsing. A file
that cannot be opened or read is an error naming the path, never a substitute
hash, so the failure reaches the caller instead of silently entering the tree.

**Leaf hash from bytes in hand** — the same 64-hex digest over a byte string
handed over directly, with no path involved and no failure mode. This is what
gives a leaf that has no content file its digest: a requirement's or an api's
JSON fields are serialised and the resulting bytes hashed here.

**Interior hash** — given the content hashes of a node's children, the SHA-256
digest of those hashes sorted and joined:

```
interior_hash(child_hashes):
    sorted = sort_ascending(child_hashes)   # byte order over the hex strings
    return sha256_hex(join(sorted, ""))     # no separator, no length prefix
```

The inputs are the children's hex hash strings, not their keys and not their
file bytes, and they are joined with nothing between them. The sort is what
makes the result independent of discovery order: the same children handed over
in any order produce the same interior hash.

## Design Rationale

### SHA-256

SHA-256 is used for its collision resistance and stdlib availability (`crypto/sha256`). The hash output is 64 hex characters, compact enough for snapshot files and git diffs.

### Sorted children

Interior node hashes are computed from lexicographically sorted child hashes. This ensures determinism — the hash is independent of the order its children arrive in. Two spec directories with identical content always produce identical trees.

### Hex encoding

Hashes are stored as hex strings, not raw bytes. This makes snapshot files human-readable and diff-friendly in git.

## Call sites

Hasher has no direct CLI surface. It is composed by `TreeBuilder`, and a tree
gets built wherever a command needs the current spec's hashes. That is not
only the snapshot pipeline:

- `spex diff` — builds the current tree to compare against the loaded snapshot.
- `spex ingest` SnapshotSaver — rebuilds the tree from the current spec to
  persist a fresh snapshot alongside the journal appends, on complete
  receipts.
- `spex ingest --mode refresh` — RefreshHandler builds its own tree, both to
  gate the run against the previous snapshot and to read off it the content
  hashes it writes into the journal's change events.
- `spex validate` — the link check builds a tree to collect the leaf keys an
  inline spec link has to resolve against.
- `spex plan` — the spec graph it loads carries a tree built the same way.

There is no standalone `spex hash` command. A separate CLI step that built
and persisted the tree on demand would either write a snapshot matching
current content (stalling the next `spex diff`) or desync the snapshot from
the journal (breaking the snapshot+journal atomicity invariant). Both
fail modes are avoided by keeping snapshot reads inside `spex diff` and
`spex ingest --mode refresh`, and every snapshot write inside `spex ingest`.
