package merkle

// EmptyTree returns the canonical empty merkle baseline: a project root
// with no children and a hash equal to SHA-256 of the empty input. It is
// the baseline that SnapshotStore.Load returns when spec/.snapshot.json is
// absent, so the first spex diff invocation reports every current leaf as
// "added" and the standard impact → emit → adapter → ingest cycle creates
// the first beads. See spec/merkle/flow_hash_computation.md for the full
// bootstrap flow.
func EmptyTree() *Node {
	return &Node{
		Key:  "project",
		Hash: HashChildren(nil),
		Type: "project",
	}
}
