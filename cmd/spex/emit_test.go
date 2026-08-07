package main

// TODO(bead:spexmachina-y0wc.31): every test in this file drove
// `spex emit` end-to-end against emit.Builder.Build, gutted by
// spexmachina-y0wc.19's migration of MappingStore onto the journal (it
// depended on mapping.Store plus Labeler.LabelFor and
// Resolver.ResolveDeps/ResolveParent, all retired or gutted alongside it).
// Rewrite against the journal-era EmitCommand design per
// spec/emit/arch_emit_command.md.
