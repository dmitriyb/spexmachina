# Orphan Detection Implementation

## Approach

Set-based coverage analysis within each module.

## Algorithm

For each module:

### Orphan Requirements
1. `allReqs` = set of all requirement IDs in the module
2. `implReqs` = set of all requirement IDs referenced by any component's `implements` array
3. `orphanReqs` = `allReqs - implReqs`
4. For each orphan, emit a warning with the requirement ID and title

### Orphan Components
1. `allComps` = set of all component IDs in the module
2. `describedComps` = set of all component IDs referenced by any impl_section's `describes` array
3. `orphanComps` = `allComps - describedComps`
4. For each orphan, emit a warning with the component ID and name

## Severity

Orphans are warnings, not errors. A spec in active development may have requirements awaiting component assignment. The validator flags them for visibility without blocking the pipeline.
