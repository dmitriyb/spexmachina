# NodeRenamer

The component behind `spex node rename`, and the answer to the one edit the identity contract makes expensive by hand. A node's id is the hash of its scope, type and name, so a new name is a new id, and every place the old id was written — reference fields in JSON, typed links in leaves, the content file named after the node — has to move with it. [[dfd7be4b1dd0|Rename a node as one transaction]] is the contract: all of it, or none of it.

## The transaction

Given a node's id and a new name, the renamer:

1. derives the new id from the node's scope, type and the new name — what `spex hash-id` would print;
2. rewrites the node's entry in place: the new name, the new id, and for a content-bearing type the new conventional content path;
3. rewrites every reference field entry that names the old id, in every `project.json` and `module.json` in the tree — `implements`, `uses`, `describes`, `provided_by`, `preq_id`, `depends_on`, `requires_module`, and any reference field a profile declares;
4. repoints every typed link naming the old id, in every content leaf, at the new id, leaving the display text alone — display text is prose, and the renamer does not judge whether it still fits;
5. moves the content file to the new path, contents unchanged;
6. prints the retired name, as data in the write report, so the vocabulary sweep receives it.

The after-state goes through [[b9e7b96f6aa7|ObligationReporter]] before any of it lands: the new name is checked for declarability, the new id for collision, the tree for integrity, and a refusal writes nothing. On success the report also carries the obligations the rename incurred — every `module.json` it rewrote moved a `meta` leaf.

## What the pipeline sees

A rename reaches `spex diff` exactly as the identity contract says it should: one node removed under the old id and one added under the new, with nothing dangling — no `id` error for a stale reference, no `link` error for a stale link, and `spex validate` green. There is no lineage edge between the two; the journal is the lineage. For a name-declarable node the removal side is also a removal for the vocabulary sweep, and the retired name the renamer printed is what the operator sweeps out of the prose: a `surviving_name` error stands for every mention the sweep finds until the prose is rewritten. Which mentions the sweep finds is the sweep's own rule, stated where `spex diff` is; the renamer promises only that it repointed every typed link and moved the file.

## What it refuses

A new name equal to the old is a no-op that says so. A new name that collides with an existing node of the same type and scope is refused with the existing node's id as the fix. A new name the tokenizer would not reproduce is refused with the form it would reproduce — the same declarability rule the validator applies, because a name the removal sweep could never rebuild is a rename nobody could later verify. A module id is refused too: a module's name is its path and its `module.json` name at once, and renaming one is a `project.json` edit plus a directory move made by hand, which the fix names.
