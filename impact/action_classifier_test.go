package impact

// TODO(bead:spexmachina-y0wc.24): every test in this file drove
// ClassifyActions through Match/Unmatched/Orphaned (NodeMatcher,
// mapping.Record), both retired by spexmachina-y0wc.19's migration of
// MappingStore onto the journal. Rewrite against the journal-era
// ActionClassifier design per spec/impact/test_classification_reporting.md.
// attachDepSpecNodeIDs/collectRequiresModule/testSectionProducesBead/
// resolveNodeName in action_classifier.go carry no mapping.Record
// dependency and are kept live for reuse, but are untested until this file
// is rewritten.
