# Hash command implementation

## Structure

`cmd/spex/hash.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, resolve spec directory
2. Call `Hasher.HashLeaves(dir)` to get leaf hashes
3. Call `TreeBuilder.Build(leaves)` to construct the merkle tree
4. Call `SnapshotStore.Save(tree)` to write snapshot file
5. Output root hash and per-node hashes (JSON or human-readable)
