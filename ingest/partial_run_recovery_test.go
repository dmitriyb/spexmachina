package ingest

// TODO(bead:spexmachina-y0wc.36): every test in this file drove
// Reconciler.Apply against mapping.Store, retired by spexmachina-y0wc.19's
// migration of MappingStore onto the journal. Rewrite against the
// journal-era Reconciler design per spec/ingest/test_partial_run_recovery.md.
