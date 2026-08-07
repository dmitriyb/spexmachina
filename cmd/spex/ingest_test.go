package main

// TODO(bead:spexmachina-y0wc.35): every test in this file drove
// `spex ingest` end-to-end against mapping.NewFileStore/ingest.Reconciler/
// ingest.RefreshHandler, all retired or gutted by spexmachina-y0wc.19's
// migration of MappingStore onto the journal. Rewrite against the
// journal-era IngestCommand design per spec/ingest/arch_ingest_command.md.
