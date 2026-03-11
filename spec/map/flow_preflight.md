# Preflight Checking Flow

## Data Flow

```
/implement or /review skill
     │
     │  reads bead, extracts label
     ▼
┌─────────────────┐
│ Parse bead label │── "spex:42" → record_id = 42
└──────┬──────────┘
       │
       ▼
┌─────────────────────┐
│ spex map get 42      │── returns mapping record JSON
│ (MapCommand)         │   {spec_node_id, module, component,
│                      │    content_file, spec_hash}
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ spex check <bead-id> │── returns preflight result JSON
│ (MapCommand +        │   {status, blockers}
│  PreflightChecker)   │
└──────┬──────────────┘
       │
       ├── status: "ready"  → skill proceeds with implementation
       ├── status: "blocked" → skill reports blockers to user
       └── status: "stale"  → skill warns user, may re-read spec
```

## Skill Integration

### Before (label-parsing approach)

```
bead labels: spec_module:impact, spec_component:ActionClassifier
skill: parse labels → concatenate "spec/impact/arch_action_classifier.md"
       (fragile: depends on naming convention, case manipulation)
```

### After (mapping approach)

```
bead label: spex:42
skill: spex map get 42 → {"content_file": "spec/impact/arch_action_classifier.md", ...}
       (robust: content_file is an exact path stored at mapping time)
```

## Benefits

- **No string manipulation**: Skills get the exact content path from the mapping record
- **No naming convention coupling**: Renaming a content file only requires updating the mapping record, not changing skill logic
- **Dependency awareness**: `spex check` gives skills dependency readiness information that was previously unavailable
- **Single label format**: Bead labels contain only `spex:<id>`, not multiple spec-related keys
