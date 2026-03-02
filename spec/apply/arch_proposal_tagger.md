# ProposalTagger

Tags all affected beads with the proposal that triggered the changes.

## Responsibilities

- Accept the proposal reference (filename or path)
- Tag all beads affected by the apply operation (created, closed, updated)
- Uses the `bd update --metadata` command to add the proposal reference

## Interface

```go
func TagWithProposal(ctx context.Context, beadIDs []string, proposalRef string) error
```

## bd Command Construction

For each affected bead:
```
bd update <bead_id> --metadata spec_proposal=<proposal_ref>
```

The proposal reference is the proposal filename (e.g., `2026-02-23-spex-machina.md`).

## Audit Trail

The proposal tag creates a queryable audit trail: given a proposal, find all beads it affected. Given a bead, find which proposal triggered its creation or modification.
