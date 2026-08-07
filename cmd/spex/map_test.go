package main

// TODO(bead:spexmachina-y0wc.21): every test in this file drove
// `spex map get/list/context` end-to-end against
// mapping.NewFileStore/mapping.ResolveContext, both retired by
// spexmachina-y0wc.19's migration of MappingStore onto the journal.
// Rewrite against mapping.NewMappingStore per spec/map/test_map_command.md.
