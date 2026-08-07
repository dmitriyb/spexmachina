package emit

// TODO(bead:spexmachina-y0wc.27): every test in this file drove Labeler
// against a stubStore double for mapping.Store, retired by
// spexmachina-y0wc.19's migration of MappingStore onto the journal.
// Rewrite against the journal-era idempotency-labelling design per
// spec/emit/test_changeset_builder.md.
