# Bead Action Tests

Integration and acceptance tests for BeadCreator (component 1), BeadCloser (component 2), and BeadUpdater (component 3). These tests verify that the apply module correctly translates impact report actions into bead CLI commands and handles the full spectrum of success, failure, and idempotency cases.

## Setup

### Test Fixture: Fake BeadCLI

All bead action tests use a fake implementation of `BeadCLI` that records every call and returns configurable responses. This avoids shelling out to real `br`/`bd` binaries during tests.

```go
type fakeBeadCLI struct {
    createCalls   []CreateOpts
    closeCalls    []closeCall
    updateCalls   []updateCall
    findResults   map[string]string // labels key -> existing bead ID
    createResults []string          // bead IDs returned in order
    closeErrors   map[string]error  // bead ID -> error to return
    updateErrors  map[string]error  // bead ID -> error to return
}
```

The fake tracks call order across Create, Close, and Update to verify execution sequencing.

### Test Fixture: Impact Report Actions

Standard action sets used across scenarios:

- **Single component create**: one create action for `validator/ContentResolver` with `spec_hash:abc123`
- **Multi-node create**: three create actions spanning two modules (`validator/ContentResolver`, `validator/DagChecker`, `merkle/Hasher`)
- **Single close**: one close action for bead `spexmachina-42` with reason `"Spec node removed: validator/LegacyChecker"`
- **Single review**: one review action for bead `spexmachina-77` in module `merkle`, node `Hasher`, new hash `def456`
- **Mixed batch**: two creates, one close, three reviews — exercises all three components in a single apply

## Scenarios

### S1: BeadCreator creates bead with correct labels for a component node

Given a create action with `module=validator`, `node=ContentResolver`, `node_type=component`, `spec_hash=abc123`.

When `CreateBeads` is called with this action:

Then the fake receives exactly one `Create` call with:
- Title: `"validator: ContentResolver"`
- Type: `"task"`
- Labels containing `spec_module:validator`, `spec_component:ContentResolver`, `spec_hash:abc123`

And the returned bead ID list contains the single ID from the fake's response.

### S2: BeadCreator creates bead with correct labels for an impl_section node

Given a create action with `module=merkle`, `node=HashComputation`, `node_type=impl_section`, `spec_hash=fff000`.

When `CreateBeads` is called:

Then labels contain `spec_module:merkle`, `spec_impl_section:HashComputation`, `spec_hash:fff000` (not `spec_component`).

### S3: BeadCreator skips creation when matching bead already exists (idempotency)

Given a create action for `validator/ContentResolver` and the fake's `FindExisting` returns bead ID `spexmachina-99` for labels `["spec_module:validator", "spec_component:ContentResolver"]`.

When `CreateBeads` is called:

Then `Create` is never called on the fake. The returned bead ID list contains `spexmachina-99`. This verifies the idempotency guarantee: applying the same impact report twice does not duplicate beads.

### S4: BeadCreator processes multiple creates sequentially and accumulates IDs

Given three create actions across two modules.

When `CreateBeads` is called:

Then the fake receives exactly three `Create` calls in the order the actions appear in the input slice. The returned bead ID list has three entries matching the fake's configured responses. Sequential ordering prevents race conditions in the bead store.

### S5: BeadCreator propagates creation errors and stops the batch

Given two create actions where the fake returns an error on the second create.

When `CreateBeads` is called:

Then the first create succeeds, the second returns an error, and `CreateBeads` returns that error. The returned bead ID list contains only the first bead ID. No third action is attempted (short-circuit on error, consistent with the flow spec: if any step fails, subsequent steps do not run).

### S6: BeadCloser closes bead with correct reason string

Given a close action with `bead_id=spexmachina-42`, `module=validator`, `node=LegacyChecker`.

When `CloseBeads` is called:

Then the fake receives one `Close` call with ID `spexmachina-42` and reason `"Spec node removed: validator/LegacyChecker"`.

### S7: BeadCloser treats individual close errors as warnings and continues

Given three close actions where the second one fails (e.g., bead already closed).

When `CloseBeads` is called:

Then all three `Close` calls are made on the fake. The returned error is a summary error aggregating the warning from the second close. The first and third closes are not affected by the second's failure.

### S8: BeadCloser returns nil when all closes succeed

Given two close actions that both succeed.

When `CloseBeads` is called:

Then the returned error is nil. Both `Close` calls were made.

### S9: BeadUpdater updates spec_hash label on reviewed bead

Given a review action with `bead_id=spexmachina-77`, new `spec_hash=def456`.

When `UpdateBeads` is called:

Then the fake receives one `Update` call with ID `spexmachina-77` and metadata `{"spec_hash": "def456"}`. No other metadata keys are modified.

### S10: BeadUpdater treats individual update errors as warnings and continues

Given three review actions where the middle one fails.

When `UpdateBeads` is called:

Then all three `Update` calls are made. The returned error is a summary error aggregating the warning. The other two updates are not affected.

### S11: BeadUpdater handles empty review list as no-op

Given an empty reviews slice.

When `UpdateBeads` is called:

Then no `Update` calls are made and the returned error is nil. This covers the common case where an impact report has creates but no reviews.

## Edge Cases

### E1: Create action with empty spec_hash

Given a create action where `spec_hash` is an empty string (possible if the merkle tree has not been computed yet).

When `CreateBeads` is called:

Then the label `spec_hash:` is still included (empty value, not omitted). The bead is created; the empty hash signals "not yet hashed" rather than "no hash." The BeadUpdater will populate it on the next diff cycle.

### E2: Close action with bead_id that no longer exists

Given a close action for bead `spexmachina-gone` where the fake returns `"bead not found"` error.

When `CloseBeads` is called:

Then the error is logged as a warning (not fatal). `CloseBeads` returns a summary error containing the warning but does not abort. This handles the race condition where a bead is manually closed between `spex impact` and `spex apply`.

### E3: Create action where FindExisting errors

Given a create action where `FindExisting` returns an error (e.g., bead CLI timeout).

When `CreateBeads` is called:

Then the error propagates immediately. `CreateBeads` does not fall through to `Create` — a failed existence check is not the same as "does not exist." This prevents duplicate bead creation when the CLI is temporarily unavailable.

### E4: Update action with bead already at target hash

Given a review action for `spexmachina-77` with `spec_hash=def456` and the bead already has label `spec_hash:def456`.

When `UpdateBeads` is called:

Then the `Update` call is still made (the CLI `--add-label` is idempotent by design — it overwrites the existing value). No special handling is needed; the operation is naturally idempotent.

### E5: Mixed batch with zero-length sublists

Given an impact report with `creates=[]`, `closes=[one action]`, `reviews=[]`.

When the full apply flow processes this:

Then `CreateBeads` is called with an empty slice and returns immediately (no bead IDs). `CloseBeads` processes the single close. `UpdateBeads` is called with an empty slice and returns immediately. The empty sublists do not cause nil pointer dereferences or skip subsequent steps.

### E6: Large batch ordering verification

Given 50 create actions with distinct modules and nodes.

When `CreateBeads` is called:

Then the fake records exactly 50 `Create` calls in the same order as the input slice. The returned bead ID list has exactly 50 entries. This validates that sequential processing does not silently drop or reorder actions.
