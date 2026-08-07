package ingest

// TODO(bead:spexmachina-y0wc.34): every test in this file drove
// RefreshHandler.Apply against mapping.Store, retired by
// spexmachina-y0wc.19's migration of MappingStore onto the journal.
// Rewrite against the journal-era refresh design per
// spec/ingest/arch_refresh.md.
