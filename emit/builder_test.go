package emit

// TODO(bead:spexmachina-y0wc.30): every test in this file drove
// Builder.Build, which spexmachina-y0wc.19's migration of MappingStore onto
// the journal gutted (it depended on mapping.Store plus Labeler.LabelFor and
// Resolver.ResolveDeps/ResolveParent, all retired or gutted alongside it).
// Rewrite against the journal-era ChangesetBuilder design per
// spec/emit/test_changeset_builder.md.
