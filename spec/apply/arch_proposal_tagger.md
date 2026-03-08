# ProposalTagger

Tags all affected beads with the proposal that triggered the changes.

## Responsibilities

- Accept the proposal reference (filename or path)
- Tag all beads affected by the apply operation (created, closed, updated)
- Uses the `<bin> update --add-label` command to add the proposal reference (where `<bin>` is `br` or `bd`)

## Interface

```go
func TagWithProposal(ctx context.Context, beadIDs []string, proposalRef string) error
```

## Bead Command Construction

For each affected bead:
```
<bin> update <bead_id> --add-label spec_proposal=<proposal_ref>
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`).

The proposal reference is the proposal filename (e.g., `2026-02-23-spex-machina.md`).

## Audit Trail

The proposal tag creates a queryable audit trail: given a proposal, find all beads it affected. Given a bead, find which proposal triggered its creation or modification.
